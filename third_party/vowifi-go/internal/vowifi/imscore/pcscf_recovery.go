package imscore

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const pcscfInitialRegistrationTimeout = 30 * time.Second

type pcscf503RecoveryDecision struct {
	recover          bool
	retryAfter       time.Duration
	unavailableUntil time.Time
	headerInvalid    bool
}

func decidePCSCF503Recovery(
	response *sipResponse,
	timerB time.Duration,
	now time.Time,
) pcscf503RecoveryDecision {
	if response == nil || response.StatusCode != 503 {
		return pcscf503RecoveryDecision{}
	}
	retryAfter, present, err := parseSIPRetryAfter(response.HeaderValues("Retry-After"))
	if err != nil || !present {
		return pcscf503RecoveryDecision{recover: true, headerInvalid: err != nil}
	}
	if timerB <= 0 {
		timerB = defaultSIPTransactionTimers().bf
	}
	if retryAfter <= timerB {
		return pcscf503RecoveryDecision{retryAfter: retryAfter}
	}
	return pcscf503RecoveryDecision{
		recover: true, retryAfter: retryAfter, unavailableUntil: now.Add(retryAfter),
	}
}

func parseSIPRetryAfter(values []string) (time.Duration, bool, error) {
	if len(values) == 0 {
		return 0, false, nil
	}
	value := strings.TrimSpace(values[0])
	if end := strings.IndexAny(value, " \t(;"); end >= 0 {
		value = value[:end]
	}
	seconds, err := strconv.ParseInt(value, 10, 32)
	if err != nil || seconds < 0 {
		return 0, true, fmt.Errorf("invalid SIP Retry-After %q", values[0])
	}
	return time.Duration(seconds) * time.Second, true, nil
}

func (s *Service) observeInitialInvite503(response *sipResponse) {
	decision := decidePCSCF503Recovery(response, s.inviteTimerB(), time.Now())
	if !decision.recover {
		return
	}
	if decision.headerInvalid {
		logging.Info("IMS 503 has invalid Retry-After; treating it as absent",
			"device", s.DeviceID(), "retry_after", response.Header("Retry-After"))
	}
	if !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return
	}
	s.mu.RLock()
	failedRegistrar := s.registrar
	s.mu.RUnlock()
	go s.recoverPCSCFAfter503(failedRegistrar, decision)
}

func (s *Service) inviteTimerB() time.Duration {
	if s != nil && s.transport != nil && s.transport.timers.bf > 0 {
		return s.transport.timers.bf
	}
	return defaultSIPTransactionTimers().bf
}

func (s *Service) recoverPCSCFAfter503(
	failedRegistrar string,
	decision pcscf503RecoveryDecision,
) {
	defer s.pcscfRecoveryPending.Store(false)
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	next, current := s.markRegistrarUnavailableAndAdvance(
		failedRegistrar, decision.unavailableUntil,
	)
	if !current {
		return
	}
	s.markPCSCFRegistrationUnbound(failedRegistrar)
	s.resetRegistrationTransportForRegistrarRetry()
	if next == "" {
		s.reportRegistrationRuntimeError(fmt.Errorf(
			"imscore: P-CSCF %s returned 503 and no alternate is available", failedRegistrar,
		))
		return
	}
	logging.Info("IMS P-CSCF recovery starting initial registration",
		"device", s.DeviceID(), "previous", failedRegistrar, "next", next,
		"retry_after_seconds", int(decision.retryAfter/time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), pcscfInitialRegistrationTimeout)
	defer cancel()
	if err := s.registerLocked(ctx); err != nil {
		s.reportRegistrationRuntimeError(fmt.Errorf("imscore: alternate P-CSCF registration failed: %w", err))
		return
	}
	logging.Info("IMS P-CSCF recovery completed", "device", s.DeviceID(), "registrar", next)
}

func (s *Service) markPCSCFRegistrationUnbound(registrar string) {
	reason := fmt.Sprintf("P-CSCF %s returned 503 Service Unavailable", registrar)
	s.mu.Lock()
	s.regState = regFailed
	s.signalingReady = false
	s.signalingFailureReason = reason
	s.registrationRefreshAt = time.Time{}
	s.lastSIPText = "503 Service Unavailable"
	s.mu.Unlock()
	s.lastSIPCode.Store(503)
	s.reRegisterPending.Store(true)
	s.transitionRegStatus(registrationRejectedTemporary)
	s.notifySMSReadiness()
}
