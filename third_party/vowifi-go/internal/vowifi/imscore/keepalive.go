package imscore

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	enginesim "github.com/iniwex5/vowifi-go/engine/sim"
	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
	"github.com/iniwex5/vowifi-go/internal/vowifi/imsheaders"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

const (
	imsKeepaliveInterval           = 60 * time.Second
	imsKeepaliveTransactionTimeout = 15 * time.Second
	imsTCPCRLFWriteTimeout         = 2 * time.Second
	imsTCPCRLFPongTimeout          = 10 * time.Second
	imsSTUNRetransmitRTO           = 500 * time.Millisecond
	imsSTUNRetransmitCount         = 7
	imsUDPSTUNMinInterval          = 24 * time.Second
	imsUDPSTUNMaxInterval          = 29 * time.Second
	imsKeepaliveFailureLimit       = 3
	imsMaintenancePollInterval     = 5 * time.Second
	imsMaintenanceMinimumDelay     = 100 * time.Millisecond
	imsRegistrationRefreshAdvance  = 60 * time.Second
	imsKeepaliveFlow               = "options_keepalive"
	imsKeepaliveSupported          = "path, 100rel, replaces, outbound, gruu"
	imsProtectedKeepaliveSupported = "path, sec-agree, 100rel, replaces, outbound, gruu"
)

var (
	errOPTIONSKeepalive    = errors.New("imscore: SIP OPTIONS keepalive")
	errTCPCRLFPongTimeout  = errors.New("imscore: TCP CRLF keepalive pong timeout")
	errTCPCRLFStreamClosed = errors.New("imscore: TCP CRLF keepalive stream closed")
)

type imsMaintenanceAction uint8

const (
	imsMaintenanceIdle imsMaintenanceAction = iota
	imsMaintenanceRefresh
	imsMaintenanceSubscribe
	imsMaintenanceSubscribeMWI
	imsMaintenanceKeepalive
)

func (s *Service) startIMSKeepalive() {
	if s == nil {
		return
	}
	s.UpdateLastPingAt()
	s.keepaliveOnce.Do(func() {
		s.networkDone.Add(1)
		go s.keepaliveLoop()
	})
	s.signalIMSMaintenance()
}

func (s *Service) keepaliveLoop() {
	defer s.networkDone.Done()
	for {
		if !s.waitForIMSMaintenance(s.computeNextWakeTime(time.Now())) {
			return
		}
		switch s.nextIMSMaintenanceAction(time.Now()) {
		case imsMaintenanceRefresh:
			s.refreshRegistration()
		case imsMaintenanceSubscribe:
			s.refreshRegistrationSubscription()
		case imsMaintenanceSubscribeMWI:
			s.refreshMWISubscription()
		case imsMaintenanceKeepalive:
			s.handleIMSKeepaliveTick()
		}
	}
}

func (s *Service) waitForIMSMaintenance(wakeAt time.Time) bool {
	delay := time.Until(wakeAt)
	if delay < imsMaintenanceMinimumDelay {
		delay = imsMaintenanceMinimumDelay
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.stop:
		return false
	case <-s.maintenanceWake:
		return true
	case <-timer.C:
		return true
	}
}

func (s *Service) computeNextWakeTime(now time.Time) time.Time {
	s.mu.RLock()
	registered := s.regState == regRegistered
	subscriptionEligible := s.subscriptionEligibleLocked()
	mwiEligible := s.mwiSubscriptionEligibleLocked()
	refreshAt := s.registrationRefreshAt
	subscribeAt := s.subscriptionRefreshAt
	mwiAt := s.mwiSubscriptionRefreshAt
	lastTrafficAt := s.lastPingAt
	interval := s.keepaliveIntervalLocked()
	s.mu.RUnlock()

	next := now.Add(imsMaintenancePollInterval)
	if !registered {
		return next
	}
	if !refreshAt.IsZero() && refreshAt.Before(next) {
		next = refreshAt
	}
	if subscriptionEligible && !subscribeAt.IsZero() && subscribeAt.Before(next) {
		next = subscribeAt
	}
	if mwiEligible && !mwiAt.IsZero() && mwiAt.Before(next) {
		next = mwiAt
	}
	keepaliveAt := lastTrafficAt.Add(interval)
	if lastTrafficAt.IsZero() {
		keepaliveAt = now
	}
	if keepaliveAt.Before(next) {
		next = keepaliveAt
	}
	return next
}

func (s *Service) nextIMSMaintenanceAction(now time.Time) imsMaintenanceAction {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.regState != regRegistered {
		return imsMaintenanceIdle
	}
	if s.registrationRefreshAt.IsZero() || !now.Before(s.registrationRefreshAt) {
		return imsMaintenanceRefresh
	}
	if s.subscriptionEligibleLocked() &&
		(s.subscriptionRefreshAt.IsZero() || !now.Before(s.subscriptionRefreshAt)) {
		return imsMaintenanceSubscribe
	}
	if s.mwiSubscriptionEligibleLocked() &&
		!s.mwiSubscriptionRefreshAt.IsZero() && !now.Before(s.mwiSubscriptionRefreshAt) {
		return imsMaintenanceSubscribeMWI
	}
	if s.lastPingAt.IsZero() || !now.Before(s.lastPingAt.Add(s.keepaliveIntervalLocked())) {
		return imsMaintenanceKeepalive
	}
	return imsMaintenanceIdle
}

func (s *Service) handleIMSKeepaliveTick() {
	if !s.IsRegistered() {
		return
	}
	_, _ = s.sendPing()
}

func (s *Service) sendPing() (bool, error) {
	if !s.pingSending.CompareAndSwap(false, true) {
		return false, nil
	}
	defer s.pingSending.Store(false)
	err := s.sendIMSKeepalive()
	s.recordIMSKeepaliveResult(err, time.Now())
	return true, err
}

func (s *Service) recordIMSKeepaliveResult(err error, completedAt time.Time) {
	if errors.Is(err, errTCPCRLFPongTimeout) && !s.expectsCRLFPong() {
		logging.RunDebug("IMS TCP CRLF pong not required",
			"device", s.DeviceID(), "err", err)
		err = nil
	}
	s.mu.Lock()
	s.lastPingAt = completedAt
	limit := s.keepaliveFailureLimit
	if err == nil {
		s.pingFailCount.Store(0)
		s.lastPingOK.Store(true)
		s.mu.Unlock()
		s.keepaliveSuccessOnce.Do(func() {
			logging.Info("IMS SIP keepalive established", "device", s.DeviceID())
		})
		return
	}
	s.lastPingOK.Store(false)
	failures := int(s.pingFailCount.Add(1))
	s.mu.Unlock()
	logging.WarnRate("ims-keepalive", "IMS SIP keepalive failed",
		"device", s.DeviceID(), "attempt", failures, "err", err)
	if failures == limit && s.keepaliveFailureRequiresRefresh(err) {
		s.triggerRegisterImmediate(fmt.Sprintf("SIP keepalive failed %d times: %v", failures, err))
	}
}

func (s *Service) keepaliveFailureRequiresRefresh(err error) bool {
	if err == nil || errors.Is(err, errOPTIONSKeepalive) {
		return false
	}
	if errors.Is(err, errSTUNMappedChanged) {
		return true
	}
	if errors.Is(err, errTCPCRLFPongTimeout) && !s.expectsCRLFPong() {
		return false
	}
	return true
}

func (s *Service) expectsCRLFPong() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sipOutboundKeepalive
}

func (s *Service) signalIMSMaintenance() {
	select {
	case s.maintenanceWake <- struct{}{}:
	default:
	}
}

func (s *Service) refreshRegistrationSubscription() {
	ctx, cancel := context.WithTimeout(context.Background(), registrationSubscriptionTimeout)
	defer cancel()
	if err := s.sendSubscribeReg(ctx); err != nil {
		s.reportSubscriptionRuntimeError(err)
	}
}

func logRegisterRetryAttemptFailure(device, phase string, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, enginesim.ErrAPDUBusy) {
		logging.Info("IMS REGISTER retry deferred because SIM is busy",
			"device", device, "phase", strings.TrimSpace(phase), "err", err)
		return
	}
	logging.WarnRate("ims-register-retry-"+device, 30*time.Second,
		"IMS REGISTER retry failed",
		"device", device, "phase", strings.TrimSpace(phase), "err", err)
}

func registrationRefreshDelay(expires time.Duration) time.Duration {
	if expires > imsRegistrationRefreshAdvance {
		return expires - imsRegistrationRefreshAdvance
	}
	if expires > 0 && expires/2 > 0 {
		return expires / 2
	}
	return time.Second
}

func (s *Service) sendIMSKeepalive() error {
	if conn := s.liveRegistrationTCP(); conn != nil {
		return s.sendTCPCRLFKeepalive(conn)
	}
	return s.sendSTUNKeepalive()
}

func (s *Service) keepaliveIntervalLocked() time.Duration {
	if !s.udpSTUNKeepaliveLocked() {
		if s.keepaliveInterval > 0 {
			return s.keepaliveInterval
		}
		return imsKeepaliveInterval
	}
	if s.stunKeepaliveInterval > 0 {
		return s.stunKeepaliveInterval
	}
	if s.flowTimer > 0 {
		return stunIntervalFromFlowTimer(s.flowTimer)
	}
	return stunUDPKeepaliveInterval()
}

func (s *Service) udpSTUNKeepaliveLocked() bool {
	if s.registrationTCP != nil {
		return false
	}
	if s.registrationIO != nil || strings.EqualFold(s.registrationTransport, "udp") {
		return true
	}
	return s.cfg != nil && strings.EqualFold(s.cfg.Transport, "udp")
}

func (s *Service) liveRegistrationTCP() net.Conn {
	if s == nil || s.stopped() {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registrationTCP
}

type stunKeepaliveResult struct {
	mapped *net.UDPAddr
	err    error
}

func (s *Service) sendSTUNKeepalive() error {
	conn, remote := s.udpKeepaliveEndpoint()
	if conn == nil || remote == nil {
		return fmt.Errorf("%w: UDP registration flow is unavailable", errSTUNKeepalive)
	}
	var txID [12]byte
	if _, err := io.ReadFull(rand.Reader, txID[:]); err != nil {
		return fmt.Errorf("%w: %w", errSTUNKeepalive, err)
	}
	request := buildSTUNBindingRequest(txID)
	wait := make(chan stunKeepaliveResult, 1)
	s.mu.Lock()
	s.stunKeepaliveWait = wait
	s.stunKeepaliveTxID = txID
	s.mu.Unlock()
	defer s.clearSTUNKeepaliveWait(wait)

	rto := s.stunRetransmitRTO()
	attempts := 0
	timer := time.NewTimer(rto)
	defer timer.Stop()
	for {
		if err := s.writeSTUNPacket(conn, request, remote); err != nil {
			return fmt.Errorf("%w: %w", errSTUNKeepalive, err)
		}
		attempts++
		select {
		case result := <-wait:
			return s.finishSTUNKeepalive(result)
		case <-timer.C:
			if attempts >= s.stunRetransmitCount() {
				return errSTUNKeepaliveTimeout
			}
			rto *= 2
			timer.Reset(rto)
		case <-s.stop:
			return errors.New("imscore: STUN keepalive aborted")
		}
	}
}

func (s *Service) udpKeepaliveEndpoint() (net.PacketConn, *net.UDPAddr) {
	if s == nil || s.stopped() {
		return nil, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.registrationIO, cloneUDPAddr(s.registrationRemote)
}

func (s *Service) writeSTUNPacket(conn net.PacketConn, pkt []byte, remote net.Addr) error {
	s.sipWriteMu.Lock()
	defer s.sipWriteMu.Unlock()
	if err := conn.SetWriteDeadline(time.Now().Add(imsTCPCRLFWriteTimeout)); err != nil {
		return err
	}
	defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
	_, err := conn.WriteTo(pkt, remote)
	return err
}

func (s *Service) finishSTUNKeepalive(result stunKeepaliveResult) error {
	if result.err != nil {
		return result.err
	}
	if result.mapped == nil {
		return fmt.Errorf("%w: %w", errSTUNKeepalive, errSTUNMappedAddress)
	}
	s.mu.Lock()
	previous := s.stunMappedAddr
	s.stunMappedAddr = cloneUDPAddr(result.mapped)
	s.mu.Unlock()
	if previous != nil && !sameUDPAddr(previous, result.mapped) {
		if s.migrateOnMappedAddressChange(previous, result.mapped) {
			return nil
		}
		return errSTUNMappedChanged
	}
	return nil
}

func (s *Service) migrateOnMappedAddressChange(oldAddr, newAddr *net.UDPAddr) bool {
	if s == nil || s.cfg == nil || s.cfg.OnLocalAddressChange == nil || oldAddr == nil || newAddr == nil {
		return false
	}
	if err := s.cfg.OnLocalAddressChange(oldAddr.IP, newAddr.IP); err != nil {
		logging.WarnRate("ims-mobike", "IMS mapped-address MOBIKE failed", "device", s.DeviceID(), "err", err)
		return false
	}
	logging.Info("IMS mapped-address migrated with MOBIKE", "device", s.DeviceID())
	return true
}

func (s *Service) handleInboundSTUN(pkt []byte) {
	txID, ok := stunTransactionID(pkt)
	if !ok {
		return
	}
	s.mu.Lock()
	wait := s.stunKeepaliveWait
	expected := s.stunKeepaliveTxID
	s.mu.Unlock()
	if wait == nil || txID != expected {
		return
	}
	msg, err := parseSTUNMessage(pkt)
	if err != nil {
		s.completeSTUNKeepalive(wait, stunKeepaliveResult{err: fmt.Errorf("%w: %w", errSTUNKeepalive, err)})
		return
	}
	var result stunKeepaliveResult
	switch msg.Type {
	case stunBindingSuccess:
		mapped, mappedErr := stunMappedAddress(msg)
		if mappedErr != nil {
			result.err = fmt.Errorf("%w: %w", errSTUNKeepalive, mappedErr)
			break
		}
		result.mapped = mapped
	case stunBindingError:
		result.err = fmt.Errorf("%w: error response", errSTUNKeepalive)
	default:
		return
	}
	s.completeSTUNKeepalive(wait, result)
}

func (s *Service) completeSTUNKeepalive(wait chan stunKeepaliveResult, result stunKeepaliveResult) {
	if wait == nil {
		return
	}
	select {
	case wait <- result:
	default:
	}
}

func (s *Service) clearSTUNKeepaliveWait(wait chan stunKeepaliveResult) {
	if s == nil || wait == nil {
		return
	}
	s.mu.Lock()
	if s.stunKeepaliveWait == wait {
		s.stunKeepaliveWait = nil
		s.stunKeepaliveTxID = [12]byte{}
	}
	s.mu.Unlock()
}

func (s *Service) stunRetransmitRTO() time.Duration {
	if s != nil && s.stunRTO > 0 {
		return s.stunRTO
	}
	return imsSTUNRetransmitRTO
}

func (s *Service) stunRetransmitCount() int {
	if s != nil && s.stunRc > 0 {
		return s.stunRc
	}
	return imsSTUNRetransmitCount
}

func stunUDPKeepaliveInterval() time.Duration {
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 27 * time.Second
	}
	return time.Duration(24+int(b[0]%6)) * time.Second
}

func stunIntervalFromFlowTimer(flowTimer time.Duration) time.Duration {
	if flowTimer <= 0 {
		return stunUDPKeepaliveInterval()
	}
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return flowTimer * 9 / 10
	}
	percent := 80 + int(b[0]%21)
	return flowTimer * time.Duration(percent) / 100
}

func (s *Service) sendTCPCRLFKeepalive(conn net.Conn) error {
	if conn == nil {
		return errors.New("imscore: SIP TCP keepalive has no registration stream")
	}
	wait := make(chan error, 1)
	s.mu.Lock()
	s.tcpKeepalivePong = wait
	s.mu.Unlock()
	defer s.clearTCPKeepalivePong(wait)

	s.sipWriteMu.Lock()
	writeErr := func() error {
		if err := conn.SetWriteDeadline(time.Now().Add(imsTCPCRLFWriteTimeout)); err != nil {
			return err
		}
		defer func() { _ = conn.SetWriteDeadline(time.Time{}) }()
		_, err := io.WriteString(conn, "\r\n\r\n")
		return err
	}()
	s.sipWriteMu.Unlock()
	if writeErr != nil {
		return fmt.Errorf("TCP CRLF keepalive: %w", writeErr)
	}
	if !s.expectsCRLFPong() {
		s.clearTCPKeepalivePong(wait)
		return nil
	}

	timer := time.NewTimer(s.tcpCRLFPongTimeout())
	defer timer.Stop()
	select {
	case err := <-wait:
		if err != nil {
			return fmt.Errorf("TCP CRLF keepalive: %w", err)
		}
		return nil
	case <-timer.C:
		return errTCPCRLFPongTimeout
	case <-s.stop:
		return errors.New("imscore: TCP CRLF keepalive aborted")
	}
}

func (s *Service) tcpCRLFPongTimeout() time.Duration {
	if s != nil && s.tcpCRLFPongWait > 0 {
		return s.tcpCRLFPongWait
	}
	return imsTCPCRLFPongTimeout
}

func (s *Service) signalTCPKeepalivePong() {
	s.completeTCPKeepalivePong(nil)
}

func (s *Service) failTCPKeepalivePong(err error) {
	if err == nil {
		err = errTCPCRLFStreamClosed
	}
	s.completeTCPKeepalivePong(err)
}

func (s *Service) completeTCPKeepalivePong(err error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	wait := s.tcpKeepalivePong
	s.tcpKeepalivePong = nil
	s.mu.Unlock()
	if wait == nil {
		return
	}
	select {
	case wait <- err:
	default:
	}
}

func (s *Service) clearTCPKeepalivePong(wait chan error) {
	if s == nil || wait == nil {
		return
	}
	s.mu.Lock()
	if s.tcpKeepalivePong == wait {
		s.tcpKeepalivePong = nil
	}
	s.mu.Unlock()
}

func (s *Service) sendOPTIONSKeepalive() error {
	request, err := s.buildIMSKeepaliveOPTIONS()
	if err != nil {
		return err
	}
	timeout := s.keepaliveTimeout
	if timeout <= 0 {
		timeout = imsKeepaliveTransactionTimeout
	}
	response, _, err := s.dispatchOutboundRequest(
		context.Background(), imsKeepaliveFlow, request, timeout, true,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", errOPTIONSKeepalive, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: rejected with status %d (%s)", errOPTIONSKeepalive, response.StatusCode, response.Reason)
	}
	return nil
}

func (s *Service) buildIMSKeepaliveOPTIONS() (*sip.Request, error) {
	profile, err := s.reserveRegisteredSIPProfile()
	if err != nil {
		return nil, fmt.Errorf("imscore: keepalive registered profile: %w", err)
	}
	recipient, err := parseKeepaliveURI("sip:" + profile.RemoteAddress)
	if err != nil {
		return nil, fmt.Errorf("imscore: keepalive P-CSCF endpoint: %w", err)
	}
	aor, err := parseKeepaliveURI(profile.LocalURI)
	if err != nil {
		return nil, err
	}
	securityMode := "disabled"
	supported := imsKeepaliveSupported
	if strings.TrimSpace(profile.SecurityVerify) != "" {
		securityMode = securityModeIPSec
		supported = imsProtectedKeepaliveSupported
	}
	return sipkit.BuildIMSRequest(sip.OPTIONS, recipient, sipkit.IMSRequestOptions{
		Destination: profile.RemoteAddress,
		Transport:   profile.Transport, Branch: "z9hG4bK." + common.RandomHex(20),
		FromURI: aor, FromTag: profile.FromTag, ToURI: aor,
		CallID: common.RandomHex(20), CSeq: uint32(profile.InitialCSeq),
		Kind: sipkit.RequestKindOutOfDialog, SecurityMode: securityMode,
		AddRPort:          true,
		AddUserAgent:      strings.TrimSpace(profile.UserAgent) != "",
		AddSupported:      true,
		Supported:         supported,
		PreferredIdentity: imsheaders.PreferredIdentityHeaderValue(profile.LocalURI),
		Runtime: sipkit.IMSRuntimeSnapshot{
			ServiceRoute: profile.ServiceRoute, SecVerify: profile.SecurityVerify,
			PAccessNetworkInfo: profile.PANI, UserAgent: profile.UserAgent,
			LocalAddr: profile.LocalAddress, Transport: profile.Transport,
		},
	})
}

func parseKeepaliveURI(value string) (sip.Uri, error) {
	var uri sip.Uri
	if err := sip.ParseUri(strings.TrimSpace(value), &uri); err != nil {
		return sip.Uri{}, fmt.Errorf("imscore: keepalive AOR: %w", err)
	}
	return uri, nil
}
