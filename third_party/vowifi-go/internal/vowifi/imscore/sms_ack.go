package imscore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/smscodec"
	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

type rpReportRequest struct {
	Inbound       string
	Body          []byte
	RPMR          byte
	Fingerprint   string
	RemoteURI     string
	ContentType   string
	OmitBinaryCTE bool
	OmitInReplyTo bool
}

type rpReportRejectError struct {
	Status int
}

func (e *rpReportRejectError) Error() string {
	if e == nil {
		return "imscore: RP report rejected"
	}
	return fmt.Sprintf("imscore: RP report rejected with SIP status %d %s",
		e.Status, SIPStatusText(e.Status))
}

var errRPReportAborted = errors.New("imscore: RP report aborted because IMS is stopping")

const (
	rpReportInitialDelay = 0
	rpReportRetryDelay   = time.Second
	rpReportMaxAttempts  = 4
)

type mtAckAudit struct {
	traceID, target, destination, transport, callID, fingerprint string
	rpMR                                                         int
	at                                                           time.Time
}

func (s *Service) recordMTAckAudit(audit mtAckAudit, err error) {
	if s == nil {
		return
	}
	s.lastMTAckMu.Lock()
	s.lastMTAckTraceID = strings.TrimSpace(audit.traceID)
	s.lastMTAckTarget = strings.TrimSpace(audit.target)
	s.lastMTAckDestination = strings.TrimSpace(audit.destination)
	s.lastMTAckTransport = strings.TrimSpace(audit.transport)
	s.lastMTAckCallID = strings.TrimSpace(audit.callID)
	s.lastMTAckRPMR = audit.rpMR
	s.lastMTAckFingerprint = strings.TrimSpace(audit.fingerprint)
	s.lastMTAckAt = audit.at
	s.lastMTAckErr = ""
	if err != nil {
		s.lastMTAckErr = err.Error()
	}
	s.lastMTAckMu.Unlock()
}

func (s *Service) sendRPReport(report rpReportRequest) error {
	request, err := s.buildRPAckMESSAGE(report.Inbound, report.Body, report.RemoteURI, report.ContentType, report.OmitBinaryCTE, report.OmitInReplyTo)
	if err != nil {
		s.mtAckSendErr.Add(1)
		return err
	}
	modeCtx, resolveErr := s.resolveOutboundModeContext("mt-rp-ack", request)
	if resolveErr != nil {
		s.mtAckSendErr.Add(1)
		return resolveErr
	}
	traceID := common.NewTraceID()
	audit := mtAckAudit{
		traceID: traceID, target: request.Recipient.String(), destination: destinationFromContext(modeCtx),
		transport: modeCtx.Transport, callID: request.CallID().Value(), rpMR: int(report.RPMR),
		fingerprint: report.Fingerprint, at: time.Now(),
	}
	logging.RunDebug("IMS RP report send",
		"trace_id", traceID, "target", audit.target, "destination", audit.destination,
		"transport", audit.transport, "call_id", audit.callID, "rp_mr", audit.rpMR)
	ctx, cancel := context.WithTimeout(common.WithTraceID(context.Background(), traceID), inboundSMSAckTimeout)
	defer cancel()
	result, dispatchErr := s.dispatchOutboundMESSAGEWithCallbacks(outboundDispatchOptions{
		Context: ctx, Flow: "mt-rp-ack", Request: request,
		Timeout: inboundSMSAckTimeout,
	})
	err = rpReportTransactionError(result.SIPCode, dispatchErr)
	s.logRPReportProtocolTrace(request, modeCtx, report, result.SIPCode, err)
	if err != nil {
		s.mtAckSendErr.Add(1)
		s.recordMTAckAudit(audit, err)
		return err
	}
	s.mtAckSendOK.Add(1)
	s.recordMTAckAudit(audit, nil)
	return nil
}

func rpReportTransactionError(status int, dispatchErr error) error {
	if dispatchErr != nil {
		return dispatchErr
	}
	if status < 1 {
		return errors.New("imscore: RP report transaction completed without a final response")
	}
	if status < 200 || status >= 300 {
		return &rpReportRejectError{Status: status}
	}
	return nil
}

func rpReportRejectStatus(err error) int {
	var rejected *rpReportRejectError
	if errors.As(err, &rejected) && rejected != nil {
		return rejected.Status
	}
	return 0
}

func (s *Service) sendRPReportWithRetry(report rpReportRequest) {
	err := s.sendRPReportWithRetryPolicy(
		report, rpReportInitialDelay, rpReportRetryDelay,
	)
	if err != nil {
		deviceID := ""
		if s != nil && s.cfg != nil {
			deviceID = s.cfg.DeviceID
		}
		logging.WarnRate("smsip-rp-report-retry-exhausted:"+deviceID, 30*time.Second,
			"IMS RP report delivery failed after retries",
			"device", deviceID, "attempts", rpReportMaxAttempts,
			"rp_mr", int(report.RPMR), "error", err)
	}
}

func (s *Service) sendRPReportWithRetryPolicy(
	report rpReportRequest,
	initialDelay time.Duration,
	retryDelay time.Duration,
) error {
	if !s.waitSMSRetryDelay(initialDelay) {
		return errRPReportAborted
	}
	current := report
	if targets := report.ackTargets(); len(targets) > 0 {
		current.RemoteURI = targets[0]
	}
	delay := retryDelay
	attempts := 0
	var lastErr error
	for _, variant := range rpAckRequestVariants(current) {
		for {
			if attempts > 0 && delay > 0 && !s.waitSMSRetryDelay(delay) {
				if lastErr != nil {
					return lastErr
				}
				return errRPReportAborted
			}
			attempts++
			lastErr = s.sendRPReport(variant)
			if lastErr == nil {
				return nil
			}
			if rpReportFallbackStatus(lastErr) {
				// Next packing must go out on the same IP-SM-GW
				// transaction. Waiting here turned a 488 into 481.
				delay = 0
				break
			}
			if !rpReportRetryable(lastErr) || attempts >= rpReportMaxAttempts {
				return lastErr
			}
			if delay <= 0 {
				delay = retryDelay
			} else {
				delay *= 2
			}
		}
	}
	return lastErr
}

func rpReportRetryable(err error) bool {
	if err == nil {
		return false
	}
	status := rpReportRejectStatus(err)
	if status == 0 {
		return true
	}
	return status == 408 || status >= 500
}

func rpReportFallbackStatus(err error) bool {
	switch rpReportRejectStatus(err) {
	case 406, 415, 481, 488:
		return true
	default:
		return false
	}
}

func rpAckRequestVariants(report rpReportRequest) []rpReportRequest {
	var variants []rpReportRequest
	for _, uri := range rpAckURICandidates(report) {
		withURI := report
		withURI.RemoteURI = uri
		// 24.341 5.3.2.4: In-Reply-To is shall on the first attempt.
		// Live Vodafone host-only IP-SM-GW 481s an omitted In-Reply-To.
		variants = append(variants, withURI)
		if extra := rpAckDeliverReportVariant(withURI); extra != nil {
			variants = append(variants, *extra)
		}
		if !withURI.OmitInReplyTo {
			withoutIRT := withURI
			withoutIRT.OmitInReplyTo = true
			variants = append(variants, withoutIRT)
		}
		if !withURI.OmitBinaryCTE {
			withoutCTE := withURI
			withoutCTE.OmitBinaryCTE = true
			variants = append(variants, withoutCTE)
		}
	}
	if len(variants) == 0 {
		return []rpReportRequest{report}
	}
	return variants
}

func rpAckDeliverReportVariant(report rpReportRequest) *rpReportRequest {
	if len(report.Body) != 2 || report.Body[0] != 0x02 {
		return nil
	}
	next := report
	next.Body = smscodec.BuildRPAckWithDeliverReport(report.RPMR)
	if len(next.Body) <= len(report.Body) {
		return nil
	}
	return &next
}

func rpAckURICandidates(report rpReportRequest) []string {
	primary := strings.TrimSpace(report.RemoteURI)
	if primary == "" {
		targets := resolveRpAckTargets(
			rawSIPHeaderValue(report.Inbound, "P-Asserted-Identity"),
			rawSIPHeaderValue(report.Inbound, "From"),
			rawSIPHeaderValue(report.Inbound, "Contact"),
		)
		if len(targets) == 0 {
			return nil
		}
		primary = targets[0]
	}
	out := []string{primary}
	if sipURIUser(primary) != "" {
		return out
	}
	// RFC 3261 488 is location-specific. Host-only PAI can 488 while the
	// same-host From/Contact still names the IP-SM-GW (24.341 5.3.2.4).
	seen := map[string]struct{}{primary: {}}
	host := sipURIHost(primary)
	for _, extra := range []string{
		firstSIPHeaderURI(rawSIPHeaderValue(report.Inbound, "From")),
		firstSIPHeaderURI(rawSIPHeaderValue(report.Inbound, "Contact")),
	} {
		extra = strings.TrimSpace(extra)
		if extra == "" || strings.ContainsAny(extra, "\r\n") || sipURIUser(extra) == "" {
			continue
		}
		if host == "" || sipURIHost(extra) != host {
			continue
		}
		if _, ok := seen[extra]; ok {
			continue
		}
		seen[extra] = struct{}{}
		out = append(out, extra)
	}
	return out
}

func sipURIUser(value string) string {
	var uri sip.Uri
	if err := sip.ParseUri(strings.TrimSpace(value), &uri); err != nil {
		return ""
	}
	return strings.TrimSpace(uri.User)
}

func sipURIHost(value string) string {
	var uri sip.Uri
	if err := sip.ParseUri(strings.TrimSpace(value), &uri); err != nil {
		return ""
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(uri.Host), "[]"))
}

func (report rpReportRequest) ackTargets() []string {
	if strings.TrimSpace(report.RemoteURI) != "" {
		return []string{strings.TrimSpace(report.RemoteURI)}
	}
	return resolveRpAckTargets(
		rawSIPHeaderValue(report.Inbound, "P-Asserted-Identity"),
		rawSIPHeaderValue(report.Inbound, "From"),
		rawSIPHeaderValue(report.Inbound, "Contact"),
	)
}

func (s *Service) waitSMSRetryDelay(delay time.Duration) bool {
	if delay <= 0 {
		select {
		case <-s.stop:
			return false
		default:
			return true
		}
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-s.stop:
		return false
	}
}

func resolveRpAckTarget(assertedIdentity, from string) (string, error) {
	targets := resolveRpAckTargets(assertedIdentity, from, "")
	if len(targets) == 0 {
		return "", errors.New("IMS RP-ACK target is unavailable")
	}
	return targets[0], nil
}

func resolveRpAckTargets(assertedIdentity, from, contact string) []string {
	for _, value := range []string{assertedIdentity, contact, from} {
		target := firstSIPHeaderURI(value)
		if target == "" || strings.ContainsAny(target, "\r\n") {
			continue
		}
		return []string{target}
	}
	return nil
}

func routeFromRemoteEndpoint(host string, port int) string {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if net.ParseIP(host) == nil || port < 1 {
		return ""
	}
	return fmt.Sprintf("<sip:%s;lr>", net.JoinHostPort(host, fmt.Sprint(port)))
}
