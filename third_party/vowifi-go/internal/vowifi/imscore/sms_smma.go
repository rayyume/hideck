package imscore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/smscodec"
	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

// SetSMSMemoryFull records whether the SM-over-IP receiver can store MT SMS.
// Transitioning from full to available sends RP-SMMA as specified in TS 24.341 5.3.2.5.
func (s *Service) SetSMSMemoryFull(full bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	wasFull := s.smsMemoryFull
	denied := s.smsMemoryDenied
	s.smsMemoryFull = full
	s.mu.Unlock()
	if wasFull && !full && denied {
		if err := s.NotifySMSMemoryAvailable(); err != nil {
			logging.WarnRate("smsip-smma-"+s.DeviceID(), 30*time.Second,
				"IMS RP-SMMA failed after memory became available",
				"device", s.DeviceID(), "error", err)
		}
	}
}

// NotifySMSMemoryAvailable sends a SIP MESSAGE containing RP-SMMA.
func (s *Service) NotifySMSMemoryAvailable() error {
	if s == nil {
		return errors.New("imscore: nil service")
	}
	s.smsSendMu.Lock()
	defer s.smsSendMu.Unlock()
	return s.sendRPSMMAWithRetryPolicy(rpReportInitialDelay, rpReportRetryDelay)
}

func (s *Service) sendRPSMMAWithRetryPolicy(initialDelay, retryDelay time.Duration) error {
	if !s.waitSMSRetryDelay(initialDelay) {
		return errRPReportAborted
	}
	target, err := s.rpSMMATarget()
	if err != nil {
		return err
	}
	rpMR, err := s.allocateSMSRPMR()
	if err != nil {
		return fmt.Errorf("imscore: allocate RP-MR for RP-SMMA: %w", err)
	}
	delay := retryDelay
	var lastErr error
	for attempt := 0; attempt < rpReportMaxAttempts; attempt++ {
		if attempt > 0 && delay > 0 && !s.waitSMSRetryDelay(delay) {
			if lastErr != nil {
				return lastErr
			}
			return errRPReportAborted
		}
		lastErr = s.dispatchRPSMMA(target, rpMR)
		if lastErr == nil {
			s.clearSMSMemoryDenied()
			return nil
		}
		if delay <= 0 {
			delay = retryDelay
		} else {
			delay *= 2
		}
	}
	return lastErr
}

func (s *Service) dispatchRPSMMA(target string, rpMR byte) error {
	request, err := s.buildOutboundMESSAGEWithOptions(smsMESSAGEOptions{
		RemoteURI: target,
		Body:      smscodec.BuildRPSMMA(rpMR),
	})
	if err != nil {
		return err
	}
	traceID := common.NewTraceID()
	callID := outboundRequestCallID(request)
	s.rememberRPSMMA(callID)
	logging.Debug("IMS RP-SMMA send",
		"trace_id", traceID, "target", request.Recipient.String(),
		"rp_mr", int(rpMR), "call_id", callID)
	ctx, cancel := context.WithTimeout(common.WithTraceID(context.Background(), traceID), inboundSMSAckTimeout)
	defer cancel()
	result, dispatchErr := s.dispatchOutboundMESSAGEWithCallbacks(outboundDispatchOptions{
		Context: ctx, Flow: "mt-rp-smma", Request: request,
		Timeout: inboundSMSAckTimeout,
	})
	return rpReportTransactionError(result.SIPCode, dispatchErr)
}

// rememberRPSMMA records the Call-ID of an RP-SMMA we sent. The network
// acknowledges RP-SMMA with its own RP-ACK/RP-ERROR MESSAGE (24.341 5.3.2.5),
// and that report carries no MO part, so without this record it looks
// unsolicited and gets rejected with 488.
func (s *Service) rememberRPSMMA(callID string) {
	key := normalizeSMSCallID(callID)
	if s == nil || key == "" {
		return
	}
	now := time.Now()
	s.smmaSentMu.Lock()
	defer s.smmaSentMu.Unlock()
	for sent, at := range s.smmaSent {
		if now.Sub(at) > pendingSMSReportMatchWindow {
			delete(s.smmaSent, sent)
		}
	}
	s.smmaSent[key] = now
}

// matchRPSMMAReport reports whether inReplyTo refers to an RP-SMMA we sent.
// The record is kept so retransmissions of the same report still match.
func (s *Service) matchRPSMMAReport(inReplyTo string) bool {
	key := normalizeSMSCallID(inReplyTo)
	if s == nil || key == "" {
		return false
	}
	s.smmaSentMu.Lock()
	defer s.smmaSentMu.Unlock()
	at, ok := s.smmaSent[key]
	return ok && time.Since(at) <= pendingSMSReportMatchWindow
}

func (s *Service) rpSMMATarget() (string, error) {
	s.mu.RLock()
	gateway := strings.TrimSpace(s.smsMemoryDeniedGateway)
	s.mu.RUnlock()
	if gateway != "" && !strings.ContainsAny(gateway, "\r\n") {
		return gateway, nil
	}
	return s.resolveSendRoute("")
}

func (s *Service) rememberSMSMemoryDenied(raw string) {
	if s == nil {
		return
	}
	gateway := firstSIPHeaderURI(rawSIPHeaderValue(raw, "P-Asserted-Identity"))
	s.mu.Lock()
	s.smsMemoryDenied = true
	if gateway != "" && !strings.ContainsAny(gateway, "\r\n") {
		s.smsMemoryDeniedGateway = gateway
	}
	s.mu.Unlock()
}

func (s *Service) clearSMSMemoryDenied() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.smsMemoryDenied = false
	s.smsMemoryDeniedGateway = ""
	s.mu.Unlock()
}

func (s *Service) smsMemoryIsFull() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.smsMemoryFull
}
