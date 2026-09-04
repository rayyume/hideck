package imscore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const defaultSIPPort = 5060

func registerTransportDeadline(ctx context.Context, timeout time.Duration) time.Time {
	deadline := time.Now().Add(timeout)
	if ctx == nil {
		return deadline
	}
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		return contextDeadline
	}
	return deadline
}

func (s *Service) ensureRegistrationTransport(ctx context.Context) error {
	if s.transport == nil {
		return errors.New("imscore: no SIP transport")
	}
	if reconnect, client, server := s.protectedReconnectParameters(); reconnect {
		return s.dialProtectedRegistrationTCP(ctx, client, server)
	}
	if s.transport.hasSendFn() {
		candidates := registerTransportCandidates(s.cfg.Transport)
		s.mu.Lock()
		if s.registrationIO == nil && s.registrationTCP == nil {
			s.externalTransport = true
			s.registrationTransport = candidates[0]
		}
		s.mu.Unlock()
		return nil
	}
	if s.cfg.IMSNetwork == nil {
		return errors.New("imscore: no IMS network")
	}
	serverListener, clientReservation, err := s.reserveProtectedTCPPorts()
	if err != nil {
		return err
	}
	if err := s.openInitialRegistrationTransport(ctx, serverListener, clientReservation); err != nil {
		closeRegistrationReservations(serverListener, clientReservation)
		return err
	}
	return nil
}

func (s *Service) reserveProtectedTCPPorts() (net.Listener, net.Listener, error) {
	if effectiveSecAgreeMode(s.cfg, resolveActiveIMSRegisterTemplate(s.cfg)) == "disabled" {
		return nil, nil, nil
	}
	server, err := s.cfg.IMSNetwork.ListenTCP(&net.TCPAddr{IP: s.cfg.LocalIP})
	if err != nil {
		return nil, nil, fmt.Errorf("imscore: reserve protected server port: %w", err)
	}
	configuredServer, err := applyIPSec3GPPPortSListenerTCPMSSWithError(server)
	if err != nil {
		_ = server.Close()
		return nil, nil, err
	}
	server = configuredServer
	client, err := s.cfg.IMSNetwork.ListenTCP(&net.TCPAddr{IP: s.cfg.LocalIP})
	if err != nil {
		_ = server.Close()
		return nil, nil, fmt.Errorf("imscore: reserve protected client port: %w", err)
	}
	return server, client, nil
}

func tcpPort(address net.Addr) int {
	if tcpAddress, ok := address.(*net.TCPAddr); ok {
		return tcpAddress.Port
	}
	return 0
}

func cloneUDPAddr(address *net.UDPAddr) *net.UDPAddr {
	if address == nil {
		return nil
	}
	return &net.UDPAddr{IP: append(net.IP(nil), address.IP...), Port: address.Port, Zone: address.Zone}
}

func (s *Service) currentRegistrationRemote() *net.UDPAddr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneUDPAddr(s.registrationRemote)
}

func (s *Service) setProtectedRegistrarPort(port uint16) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.registrationRemote == nil {
		return errors.New("imscore: registrar address is unavailable")
	}
	s.registrationRemote.Port = int(port)
	return nil
}

func (s *Service) connectProtectedRegistrationTCP(ctx context.Context, client, server securityMechanism) error {
	s.mu.Lock()
	reservation := s.clientPortReserve
	s.clientPortReserve = nil
	s.mu.Unlock()
	if reservation == nil {
		return errors.New("imscore: protected client port was not reserved")
	}
	if err := reservation.Close(); err != nil {
		return fmt.Errorf("imscore: release protected client port: %w", err)
	}
	return s.dialProtectedRegistrationTCP(ctx, client, server)
}

func (s *Service) dialProtectedRegistrationTCP(ctx context.Context, client, server securityMechanism) error {
	registrationRemote := s.currentRegistrationRemote()
	if registrationRemote == nil || registrationRemote.IP == nil {
		return errors.New("imscore: registrar IP unavailable for protected TCP")
	}
	local := &net.TCPAddr{IP: s.cfg.LocalIP, Port: int(client.PortC)}
	remote := &net.TCPAddr{IP: registrationRemote.IP, Port: int(server.PortS)}
	conn, attempts, err := dialSecureChannelWithFallback(func(network string) (net.Conn, error) {
		if !ipsec3gppTCPNetwork(network) {
			return nil, fmt.Errorf("imscore: unsupported secure signaling network %q", network)
		}
		return s.cfg.IMSNetwork.DialTCPContext(ctx, local, remote)
	})
	logSecureChannelAttemptResult(attempts, err)
	if err != nil {
		return err
	}
	if err := setIPSec3GPPTCPMSS(conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf("imscore: set protected REGISTER TCP MSS: %w", err)
	}
	logging.Info("IMS protected REGISTER TCP connected",
		"local_port", local.Port, "remote_port", remote.Port)
	logSecureChannelEstablished(conn)
	s.activateProtectedRegistrationTCP(conn)
	return nil
}

func (s *Service) protectedReconnectParameters() (bool, securityMechanism, securityMechanism) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.externalTransport || (s.registrationTCP != nil && s.registrationTCPProtected) || s.regSession == nil || s.regSession.security == nil || s.regSession.security.server == nil {
		return false, securityMechanism{}, securityMechanism{}
	}
	return true, s.regSession.security.client, *s.regSession.security.server
}

func (s *Service) protectedTransportState() (external, connected bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.externalTransport, s.registrationTCP != nil && s.registrationTCPProtected
}

func (s *Service) activateProtectedRegistrationTCP(conn net.Conn) {
	s.configureRegistrationTCPKeepalive(conn)
	conn = s.newInboundCountingConn(conn)
	s.mu.Lock()
	previous := s.registrationTCP
	s.registrationTCP = conn
	if previous != nil && previous != conn {
		s.registrationPreviousTCP = previous
	}
	s.registrationTCPProtected = true
	s.registrationTransport = "tcp"
	s.transport.SetSendFn(func(request string) error {
		return s.writeSIPStream(conn, request)
	})
	s.mu.Unlock()
	s.networkDone.Add(1)
	go s.readRegistrationStream(conn)
}

func (s *Service) finalizeRegistrationTransportSwitch() {
	s.mu.Lock()
	previous := s.registrationPreviousTCP
	s.registrationPreviousTCP = nil
	s.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
}

func (s *Service) readRegistrationResponses(conn net.PacketConn) {
	defer s.networkDone.Done()
	s.receiverStarted()
	defer s.receiverStopped()
	buffer := make([]byte, 64*1024)
	for {
		n, remote, err := conn.ReadFrom(buffer)
		if err != nil {
			s.handleRegistrationPacketReadError(conn, err)
			return
		}
		if isSTUNMessage(buffer[:n]) {
			s.handleInboundSTUN(append([]byte(nil), buffer[:n]...))
			continue
		}
		err = s.dispatchInboundSIP(string(buffer[:n]), func(response string) error {
			if _, writeErr := conn.WriteTo([]byte(response), remote); writeErr != nil {
				return fmt.Errorf("imscore: write SIP datagram: %w", writeErr)
			}
			return nil
		})
		if err != nil {
			logging.WarnRate("ims-udp-inbound", "IMS UDP inbound handling failed", "err", err)
		}
	}
}

func (s *Service) handleRegistrationPacketReadError(conn net.PacketConn, readErr error) {
	if s.stopped() {
		return
	}
	err := fmt.Errorf("imscore: registration SIP packet socket closed: %w", readErr)
	if !s.markRegistrationPacketSignalingDead(conn, err) {
		return
	}
	logging.WarnRate("ims-registration-packet", "IMS registration SIP packet socket closed", "err", err)
}

func (s *Service) acceptProtectedSIP(listener net.Listener) {
	defer s.networkDone.Done()
	s.receiverStarted()
	defer s.receiverStopped()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		logging.Info("IPSec portS accepted server push connection",
			"device", s.DeviceID(), "remote", conn.RemoteAddr(), "local", conn.LocalAddr())
		s.inboundTCPAccept.Add(1)
		s.configureRegistrationTCPKeepalive(conn)
		conn = s.newPortSInboundConn(conn)
		if !s.trackProtectedConnection(conn) {
			_ = conn.Close()
			return
		}
		s.recordPortSOpened(conn, time.Now())
		s.resetPortSReadClock()
		s.networkDone.Add(1)
		go s.serveProtectedSIPConnection(conn)
	}
}

func (s *Service) serveProtectedSIPConnection(conn net.Conn) {
	defer s.networkDone.Done()
	defer conn.Close()
	err := s.readRegistrationStreamSync(conn)
	s.recordPortSClosed(conn, err, time.Now())
	s.untrackProtectedConnection(conn)
	if err != nil && !s.stopped() {
		logging.WarnRate("ims-protected-server-stream-"+s.DeviceID(),
			"IMS protected server push connection closed", "err", err)
		s.handleProtectedServerPushClosed()
	}
}

// handleProtectedServerPushClosed waits for the P-CSCF to reopen port-s and
// falls back to a REGISTER refresh if it does not. A failed refresh is not
// evidence of an on-demand flow: only a later peer connection without a
// successful refresh proves that behavior for the active P-CSCF.
func (s *Service) handleProtectedServerPushClosed() {
	if s == nil || s.stopped() || s.pcscfRecoveryPending.Load() {
		return
	}
	s.protectedConnMu.Lock()
	remaining := len(s.protectedConns)
	if remaining > 0 {
		s.protectedConnMu.Unlock()
		return
	}
	s.portSReconnectWaiting.Store(true)
	s.protectedConnMu.Unlock()
	if s.portSOnDemandObserved.Load() {
		logging.Info("IMS protected server push closed; wait for proven on-demand port-s reconnect",
			"device", s.DeviceID(), "since_last_read", s.portSSinceLastRead(),
			"reason", "peer previously reopened port-s without a successful REGISTER")
		return
	}
	now := time.Now()
	if retryAt, waiting := s.portSRecoveryDeadline(now); waiting {
		logging.Info("IMS protected server push closed during recovery backoff",
			"device", s.DeviceID(), "since_last_read", s.portSSinceLastRead(),
			"retry_in", retryAt.Sub(now))
		s.schedulePortSReconnectWatchAt(retryAt)
		return
	}
	wakeAt := now.Add(s.portSReconnectWait())
	logging.Info("IMS protected server push closed; wait for port-s reconnect",
		"device", s.DeviceID(), "grace", s.portSReconnectWait(),
		"since_last_read", s.portSSinceLastRead())
	s.schedulePortSReconnectWatchAt(wakeAt)
}

func (s *Service) portSReconnectWait() time.Duration {
	if s != nil && s.portSReconnectGrace > 0 {
		return s.portSReconnectGrace
	}
	if s.usesVodafoneUKPeerResetGrace() {
		return vodafoneUKPortSResetReconnectGrace
	}
	return defaultPortSReconnectGrace
}

func (s *Service) schedulePortSReconnectWatchAt(wakeAt time.Time) {
	if s == nil || s.stopped() {
		return
	}
	delay := time.Until(wakeAt)
	if delay < 0 {
		delay = 0
	}
	registrar := s.currentPortSRecoveryRegistrar()
	s.portSWatchMu.Lock()
	defer s.portSWatchMu.Unlock()
	s.portSWatchGeneration++
	generation := s.portSWatchGeneration
	if s.portSWatchTimer != nil {
		s.portSWatchTimer.Stop()
	}
	s.portSWatchTimer = time.AfterFunc(delay, func() {
		s.portSReconnectWatchFired(generation, registrar)
	})
}

func (s *Service) cancelPortSReconnectWatch() {
	if s == nil {
		return
	}
	s.portSWatchMu.Lock()
	defer s.portSWatchMu.Unlock()
	s.portSWatchGeneration++
	if s.portSWatchTimer != nil {
		s.portSWatchTimer.Stop()
		s.portSWatchTimer = nil
	}
}

func (s *Service) portSReconnectWatchFired(generation uint64, registrar string) {
	if !s.consumePortSReconnectWatch(generation, registrar) {
		return
	}
	if s == nil || s.stopped() || s.RegState() != regRegistered {
		return
	}
	s.protectedConnMu.Lock()
	remaining := len(s.protectedConns)
	if remaining > 0 || s.portSOnDemandObserved.Load() {
		s.protectedConnMu.Unlock()
		return
	}
	s.protectedConnMu.Unlock()
	if s.portSRecoveryAwaitingFlow.Swap(false) {
		s.backoffMissingPortSAfterRegister()
		return
	}
	if retryAt, waiting := s.portSRecoveryDeadline(time.Now()); waiting {
		s.schedulePortSReconnectWatchAt(retryAt)
		return
	}
	if !s.portSRecoveryPending.CompareAndSwap(false, true) {
		return
	}
	s.markPortSResetRecoveryAttempt(registrar)
	s.triggerRegisterImmediate("port-s flow failed")
}

func (s *Service) consumePortSReconnectWatch(generation uint64, registrar string) bool {
	if s == nil {
		return false
	}
	currentRegistrar := s.currentPortSRecoveryRegistrar()
	s.portSWatchMu.Lock()
	defer s.portSWatchMu.Unlock()
	if generation != s.portSWatchGeneration || registrar != currentRegistrar {
		return false
	}
	s.portSWatchTimer = nil
	return true
}

func (s *Service) completePortSRecovery(err error, bindingPreserved bool) {
	if s == nil {
		return
	}
	pending := s.portSRecoveryPending.Swap(false)
	if err == nil {
		if pending {
			s.markPortSResetRecoverySucceeded(s.currentPortSRecoveryRegistrar())
		}
		if s.portSPushReady.Load() {
			s.portSRecoveryAwaitingFlow.Store(false)
			s.portSReconnectWaiting.Store(false)
			s.resetPortSRecoveryBackoff()
			return
		}
		if pending || s.portSReconnectWaiting.Load() {
			s.clearPortSRecoveryDeadline()
			s.portSRecoveryAwaitingFlow.Store(true)
			s.portSReconnectWaiting.Store(true)
			s.schedulePortSReconnectWatchAt(time.Now().Add(s.portSReconnectWait()))
		}
		return
	}
	if !pending {
		if s.portSPushReady.Load() {
			s.portSReconnectWaiting.Store(false)
		}
		return
	}
	if !bindingPreserved {
		s.portSRecoveryAwaitingFlow.Store(false)
		s.portSReconnectWaiting.Store(false)
		s.resetPortSRecoveryBackoff()
		return
	}
	backoff := s.recordPortSRecoveryFailure(err, time.Now())
	if s.portSPushReady.Load() {
		// The failed REGISTER itself may have prompted this connection.
		// It does not prove that the peer reconnects on demand.
		s.portSReconnectWaiting.Store(false)
	} else {
		s.schedulePortSReconnectWatchAt(backoff.retryAt)
	}
	logging.WarnRate("ims-ports-recovery-backoff-"+s.DeviceID(), 30*time.Second,
		"IMS port-s REGISTER recovery failed; keep current binding and back off",
		"device", s.DeviceID(), "err", err,
		"failures", backoff.failures, "retry_in", backoff.delay,
		"retry_after", backoff.retryAfter, "retry_after_present", backoff.retryAfterSet)
}

func (s *Service) recordOnDemandPortSReconnect() {
	if s == nil {
		return
	}
	if s.portSRecoveryAwaitingFlow.Swap(false) {
		s.portSReconnectWaiting.Store(false)
		s.resetPortSRecoveryBackoff()
		logging.Info("IMS port-s reopened after successful REGISTER", "device", s.DeviceID())
		return
	}
	if s.portSRecoveryPending.Load() || s.registrationInProgress() {
		return
	}
	if !s.portSReconnectWaiting.CompareAndSwap(true, false) || s.portSOnDemandObserved.Swap(true) {
		return
	}
	s.clearPortSResetRecovery(s.currentPortSRecoveryRegistrar())
	s.resetPortSRecoveryBackoff()
	logging.Info("IMS on-demand port-s reconnect observed",
		"device", s.DeviceID(),
		"reason", "peer reopened port-s without a successful REGISTER")
}

func (s *Service) registrationInProgress() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.regState == regRegistering || s.regState == regReregister
}

func (s *Service) resetPortSRecoveryKnowledge() {
	if s == nil {
		return
	}
	s.resetPortSRecoveryBackoff()
	s.portSReconnectWaiting.Store(false)
	s.portSRecoveryPending.Store(false)
	s.portSRecoveryAwaitingFlow.Store(false)
	s.portSOnDemandObserved.Store(false)
	s.clearPortSResetRecovery("")
}

func (s *Service) trackProtectedConnection(conn net.Conn) bool {
	s.protectedConnMu.Lock()
	select {
	case <-s.stop:
		s.protectedConnMu.Unlock()
		return false
	default:
		s.protectedConns[conn] = struct{}{}
		changed := !s.portSPushReady.Swap(true)
		s.protectedConnMu.Unlock()
		s.signalDownlinkValidation()
		s.cancelPortSReconnectWatch()
		s.recordOnDemandPortSReconnect()
		if changed {
			s.notifySMSReadiness()
		}
		return true
	}
}

func (s *Service) untrackProtectedConnection(conn net.Conn) {
	s.protectedConnMu.Lock()
	delete(s.protectedConns, conn)
	empty := len(s.protectedConns) == 0
	changed := empty && s.portSPushReady.Swap(false)
	s.protectedConnMu.Unlock()
	if changed {
		s.notifySMSReadiness()
	}
}

func (s *Service) detachProtectedConnections() []net.Conn {
	if s == nil {
		return nil
	}
	s.protectedConnMu.Lock()
	connections := make([]net.Conn, 0, len(s.protectedConns))
	for conn := range s.protectedConns {
		connections = append(connections, conn)
	}
	s.protectedConns = make(map[net.Conn]struct{})
	changed := s.portSPushReady.Swap(false)
	s.protectedConnMu.Unlock()
	if changed {
		s.notifySMSReadiness()
	}
	return connections
}

func (s *Service) readRegistrationStream(conn net.Conn) {
	defer s.networkDone.Done()
	s.receiverStarted()
	defer s.receiverStopped()
	readErr := s.readRegistrationStreamSync(conn)
	s.clearClosedRegistrationTCP(conn, readErr)
}

func (s *Service) clearClosedRegistrationTCP(conn net.Conn, readErr error) {
	if readErr == nil {
		readErr = io.EOF
	}
	err := fmt.Errorf("imscore: registration SIP stream closed: %w", readErr)
	logging.WarnRate("ims-registration-stream", "IMS registration SIP stream closed", "err", err)
	s.failTCPKeepalivePong(errTCPCRLFStreamClosed)
	_ = conn.Close()
	stopped := s.stopped()
	s.mu.Lock()
	current := s.registrationTCP == conn
	if current {
		s.registrationTCP = nil
		s.registrationTCPProtected = false
		if !stopped {
			s.regState = regFailed
			s.transport.SetSendFn(func(string) error {
				return errors.New("imscore: registered SIP transport is not connected")
			})
		}
	}
	s.mu.Unlock()
	if !current || stopped {
		return
	}
	s.transport.terminateClientTransactions(transactionTransportError(readErr))
	s.transitionRegStatus(registrationRejectedTemporary)
	s.notifySMSReadiness()
	s.reportRegistrationRuntimeError(err)
}

func (s *Service) readRegistrationStreamSync(conn net.Conn) error {
	decoder := newSIPStreamDecoder(conn)
	decoder.onPong = s.signalTCPKeepalivePong
	defer decoder.Close()
	for {
		message, err := decoder.ReadMessage()
		if err != nil {
			return err
		}
		if err := s.dispatchInboundSIPMessageWithPeer(message, message.String(), func(response string) error {
			return s.writeSIPStream(conn, response)
		}, conn); err != nil {
			logging.WarnRate("ims-tcp-inbound", "IMS TCP inbound handling failed", "err", err)
		}
	}
}

type tcpKeepaliveConn interface {
	SetKeepAlive(bool) error
	SetKeepAlivePeriod(time.Duration) error
}

func configureTCPKeepalive(conn net.Conn, idle time.Duration) {
	for {
		counted, ok := conn.(*inboundCountingConn)
		if !ok || counted == nil {
			break
		}
		conn = counted.conn
	}
	keepalive, ok := conn.(tcpKeepaliveConn)
	if !ok {
		return
	}
	if idle <= 0 {
		idle = imsTCPSocketKeepaliveIdle
	}
	_ = keepalive.SetKeepAlive(true)
	_ = keepalive.SetKeepAlivePeriod(idle)
}
