package imscore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

type rpReportRequest struct {
	Inbound       string
	Body          []byte
	RPMR          byte
	Fingerprint   string
	Identity      string
	RemoteURI     string
	ContentType   string
	ServiceCenter string
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
	// One prompt covers every message the SMSC releases in the same flush,
	// so rate limit it rather than sending one per rejected report.
	smmaPromptMinInterval = time.Minute
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
	report = specRPAckReport(report)
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
	s.logRPReportWireTrace(request, result.Response)
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
	attempts, err := s.sendRPReportWithRetryPolicy(
		report, rpReportInitialDelay, rpReportRetryDelay,
	)
	if err == nil {
		s.clearRejectedMTReport(report.Identity)
		return
	}
	if err != nil {
		s.rememberRejectedMTReport(report.Identity)
		deviceID := ""
		if s != nil && s.cfg != nil {
			deviceID = s.cfg.DeviceID
		}
		logging.WarnRate("smsip-rp-report-retry-exhausted:"+deviceID, 30*time.Second,
			"IMS RP report delivery failed",
			"device", deviceID, "attempts", attempts,
			"rp_mr", int(report.RPMR), "error", err)
		s.promptSMSCRedeliveryAfterRejectedReport(err)
	}
}

// A report the gateway rejects leaves the SMSC believing the short message is
// still outstanding. Measured on 2026-09-03: one message was redelivered three
// times over 24 minutes on a rejection every time, on two different P-CSCF
// instances and with a report byte-identical in structure to ones the same pool
// accepts, and nothing else was delivered while it stayed at the head. RP-SMMA
// is the only receiver-initiated way to ask for the queue (TS 24.011 clears
// MCEF and alerts the service centre), so send one instead of waiting out a
// retransmission timer that had already backed off past 18 minutes.
func (s *Service) promptSMSCRedeliveryAfterRejectedReport(reportErr error) {
	if rpReportRejectStatus(reportErr) == 0 {
		return
	}
	if s == nil || s.stopped() || !s.reserveSMMAPrompt(time.Now()) {
		return
	}
	s.smsSendMu.Lock()
	err := s.sendRPSMMAWithRetryPolicy(rpReportInitialDelay, rpReportRetryDelay)
	s.smsSendMu.Unlock()
	if err != nil {
		logging.WarnRate("smsip-smma-prompt-"+s.DeviceID(), 30*time.Second,
			"IMS RP-SMMA redelivery prompt failed",
			"device", s.DeviceID(), "error", err)
		return
	}
	logging.Info("IMS RP-SMMA sent to release the SMSC queue", "device", s.DeviceID())
}

func (s *Service) reserveSMMAPrompt(now time.Time) bool {
	last := s.smmaPromptLastAt.Load()
	if last != 0 && now.Sub(time.Unix(0, last)) < smmaPromptMinInterval {
		return false
	}
	return s.smmaPromptLastAt.CompareAndSwap(last, now.UnixNano())
}

func (s *Service) sendRPReportWithRetryPolicy(
	report rpReportRequest,
	initialDelay time.Duration,
	retryDelay time.Duration,
) (int, error) {
	if !s.waitSMSRetryDelay(initialDelay) {
		return 0, errRPReportAborted
	}
	current := specRPAckReport(report)
	delay := retryDelay
	var lastErr error
	sent := 0
	for attempt := 1; attempt <= rpReportMaxAttempts; attempt++ {
		if attempt > 1 && delay > 0 && !s.waitSMSRetryDelay(delay) {
			if lastErr != nil {
				return sent, lastErr
			}
			return sent, errRPReportAborted
		}
		lastErr = s.sendRPReport(current)
		sent++
		if lastErr == nil {
			return sent, nil
		}
		if !rpReportRetryable(lastErr) {
			return sent, lastErr
		}
		if delay <= 0 {
			delay = retryDelay
		} else {
			delay *= 2
		}
	}
	return sent, lastErr
}

func rpReportRetryable(err error) bool {
	if err == nil {
		return false
	}
	status := rpReportRejectStatus(err)
	if status == 0 {
		return true
	}
	// 488 (24.341 5.3.3.4.1: In-Reply-To not correlated) is final: the
	// SMSC retransmits the short message and we report on that copy.
	return status == 408 || status >= 500
}

func specRPAckReport(report rpReportRequest) rpReportRequest {
	report.RemoteURI = specRPAckURI(report)
	report.OmitInReplyTo = false
	report.OmitBinaryCTE = false
	return report
}

// specRPAckURI is TS 24.341 5.3.2.4 a) / NOTE 1: Request-URI is the
// IP-SM-GW from P-Asserted-Identity of the delivered MESSAGE. From is
// only used when PAI is absent. Caller RemoteURI is last-resort only.
func specRPAckURI(report rpReportRequest) string {
	for _, header := range []string{"P-Asserted-Identity", "From"} {
		uri := firstSIPHeaderURI(rawSIPHeaderValue(report.Inbound, header))
		if uri != "" && !strings.ContainsAny(uri, "\r\n") {
			return uri
		}
	}
	if uri := strings.TrimSpace(report.RemoteURI); uri != "" && !strings.ContainsAny(uri, "\r\n") {
		return uri
	}
	return ""
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
	targets := resolveRpAckTargets(assertedIdentity, from)
	if len(targets) == 0 {
		return "", errors.New("IMS RP-ACK target is unavailable")
	}
	return targets[0], nil
}

func resolveRpAckTargets(assertedIdentity, from string) []string {
	for _, value := range []string{assertedIdentity, from} {
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
