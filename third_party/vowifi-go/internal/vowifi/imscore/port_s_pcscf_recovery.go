package imscore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const (
	vodafoneUKCarrierPresetID           = "vodafone_uk_23415"
	vodafoneUKPortSResetRecoveryPolicy  = "vodafone_uk_port_s_reset"
	vodafoneUKPortSResetReconnectGrace  = 5 * time.Second
	vodafoneUKMaturePortSResetThreshold = 2 * time.Minute
	vodafoneUKPCSCFDeprioritizedPeriod  = 30 * time.Minute
)

type portSResetRecoveryState struct {
	registrar           string
	observedAt          time.Time
	recoveryAttemptedAt time.Time
	recoverySucceeded   bool
	switchAfterRecovery bool
	failoverPending     bool
}

func (s *Service) armVodafoneUKResetRecoveryLocked(registrar string, openedAt, now time.Time) bool {
	if !usesVodafoneUKPortSResetRecovery(s.cfg) || openedAt.IsZero() {
		return false
	}
	lifetime := now.Sub(openedAt)
	if lifetime < 0 {
		return false
	}
	registrar = strings.TrimSpace(registrar)
	if registrar == "" {
		return false
	}
	if lifetime > vodafoneUKMaturePortSResetThreshold {
		// Vodafone UK and its sub-brands share this IMS network policy. A
		// mature push flow reset can indicate a stale MT delivery binding.
		// Let same-P-CSCF REGISTER recovery complete before switching once.
		s.portSSession.resetRecovery = portSResetRecoveryState{
			registrar: registrar, observedAt: now, switchAfterRecovery: true,
		}
		return false
	}
	state := &s.portSSession.resetRecovery
	if state.registrar == registrar && !state.recoveryAttemptedAt.IsZero() &&
		!openedAt.Before(state.recoveryAttemptedAt) {
		if !state.recoverySucceeded {
			return false
		}
		state.observedAt = now
		state.failoverPending = true
		return true
	}
	if state.registrar == registrar && !state.observedAt.IsZero() {
		return false
	}
	s.portSSession.resetRecovery = portSResetRecoveryState{
		registrar: registrar, observedAt: now,
	}
	return false
}

func usesVodafoneUKPortSResetRecovery(cfg *IMSConfig) bool {
	return cfg != nil && strings.TrimSpace(cfg.CarrierPresetID) == vodafoneUKCarrierPresetID
}

func (s *Service) usesVodafoneUKPeerResetGrace() bool {
	if s == nil || !usesVodafoneUKPortSResetRecovery(s.cfg) {
		return false
	}
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	return s.portSSession.lastCloseKind == portSClosePeerReset
}

func (s *Service) markPortSResetRecoveryAttempt(registrar string) {
	if s == nil {
		return
	}
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	state := &s.portSSession.resetRecovery
	if state.registrar == strings.TrimSpace(registrar) && !state.observedAt.IsZero() {
		state.recoveryAttemptedAt = time.Now()
		state.recoverySucceeded = false
	}
}

func (s *Service) markPortSResetRecoverySucceeded(registrar string) {
	if s == nil {
		return
	}
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	state := &s.portSSession.resetRecovery
	if state.registrar == strings.TrimSpace(registrar) && !state.recoveryAttemptedAt.IsZero() {
		state.recoverySucceeded = true
		state.failoverPending = state.switchAfterRecovery
	}
}

func (s *Service) clearPortSResetRecovery(registrar string) {
	if s == nil {
		return
	}
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	if registrar == "" || s.portSSession.resetRecovery.registrar == strings.TrimSpace(registrar) {
		s.portSSession.resetRecovery = portSResetRecoveryState{}
	}
}

func (s *Service) pendingPortSResetFailover() (string, time.Time, bool) {
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	state := s.portSSession.resetRecovery
	if !state.failoverPending || state.registrar == "" {
		return "", time.Time{}, false
	}
	s.portSSession.resetRecovery = portSResetRecoveryState{}
	return state.registrar, state.observedAt, true
}

func (s *Service) startPendingPortSResetFailover() {
	if s == nil || s.stopped() || !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return
	}
	failed, observedAt, ok := s.pendingPortSResetFailover()
	if !ok {
		s.pcscfRecoveryPending.Store(false)
		return
	}
	go s.recoverPCSCFAfterPortSReset(failed, observedAt)
}

func (s *Service) recoverPCSCFAfterPortSReset(failedRegistrar string, observedAt time.Time) {
	defer s.pcscfRecoveryPending.Store(false)
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	unavailableUntil := time.Now().Add(vodafoneUKPCSCFDeprioritizedPeriod)
	next, current := s.markRegistrarUnavailableAndAdvance(failedRegistrar, unavailableUntil)
	if !current {
		return
	}
	if next == "" {
		s.requestFreshRuntimeAfterPortSReset(
			failedRegistrar, observedAt, unavailableUntil,
			"no alternate P-CSCF is available",
		)
		return
	}
	s.recoverPortSResetOnAlternate(failedRegistrar, next, observedAt, unavailableUntil)
}

func (s *Service) recoverPortSResetOnAlternate(
	failedRegistrar, next string,
	observedAt, failedUnavailableUntil time.Time,
) {
	s.markPCSCFRegistrationUnboundForPortSReset(failedRegistrar)
	if err := s.resetRegistrationForPCSCFSwitch(); err != nil {
		s.reportRegistrationRuntimeError(err)
		return
	}
	logging.WarnRate("ims-ports-reset-switch-"+s.DeviceID(), 30*time.Second,
		"IMS port-s reset recovery switching P-CSCF",
		"device", s.DeviceID(), "policy", vodafoneUKPortSResetRecoveryPolicy,
		"previous", failedRegistrar, "next", next,
		"reset_at", observedAt, "deprioritized_until", failedUnavailableUntil)
	baseline := s.inboundSIPHandledRequest.Load()
	ctx, cancel := context.WithTimeout(context.Background(), pcscfInitialRegistrationTimeout)
	err := s.registerLocked(ctx)
	cancel()
	if err != nil {
		s.rejectFailedPortSRegistrar(next, observedAt, err)
		return
	}
	logging.Info("IMS port-s reset recovery registered; awaiting downlink validation",
		"device", s.DeviceID(), "policy", vodafoneUKPortSResetRecoveryPolicy,
		"registrar", next, "timeout", s.portSFailoverValidationWait())
	validatedBy, ok := s.waitForPortSFailoverValidation(baseline)
	if !ok {
		s.rejectUnverifiedPortSRegistrar(next, observedAt, "downlink validation timed out")
		return
	}
	logging.Info("IMS port-s reset recovery completed",
		"device", s.DeviceID(), "policy", vodafoneUKPortSResetRecoveryPolicy,
		"registrar", next, "validated_by", validatedBy)
}

func (s *Service) waitForPortSFailoverValidation(baseline uint64) (string, bool) {
	if !s.protectedDownlinkValidationRequired() {
		return "not_required", true
	}
	timer := time.NewTimer(s.portSFailoverValidationWait())
	defer timer.Stop()
	for {
		if s.portSPushReady.Load() {
			return "port-s", true
		}
		if s.inboundSIPHandledRequest.Load() > baseline {
			return "inbound_sip_request", true
		}
		select {
		case <-s.downlinkValidationWake:
		case <-timer.C:
			return "", false
		case <-s.stop:
			return "", false
		}
	}
}

func (s *Service) protectedDownlinkValidationRequired() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.protectedSMSPushRequiredLocked()
}

func (s *Service) portSFailoverValidationWait() time.Duration {
	if s != nil && s.portSFailoverVerifyWait > 0 {
		return s.portSFailoverVerifyWait
	}
	return defaultPortSReconnectGrace
}

func (s *Service) signalDownlinkValidation() {
	if s == nil || s.downlinkValidationWake == nil {
		return
	}
	select {
	case s.downlinkValidationWake <- struct{}{}:
	default:
	}
}

func (s *Service) rejectUnverifiedPortSRegistrar(registrar string, observedAt time.Time, reason string) {
	unavailableUntil := time.Now().Add(vodafoneUKPCSCFDeprioritizedPeriod)
	s.registrarPenalties.mark(registrar, unavailableUntil)
	s.requestFreshRuntimeAfterPortSReset(registrar, observedAt, unavailableUntil, reason)
}

func (s *Service) rejectFailedPortSRegistrar(registrar string, observedAt time.Time, err error) {
	now := time.Now()
	unavailableUntil := s.failedRegisterUnavailableUntil(err, now)
	s.registrarPenalties.mark(registrar, unavailableUntil)
	reason := fmt.Sprintf("initial registration failed: %v", err)
	s.requestFreshRuntimeAfterPortSReset(registrar, observedAt, unavailableUntil, reason)
}

func (s *Service) failedRegisterUnavailableUntil(err error, now time.Time) time.Time {
	retryDelay := s.jitterPortSRecoveryDelay(rfc5626RecoveryUpperBound(1, true))
	if retryAfter, present := registerRetryAfterFromError(err); present && retryAfter > retryDelay {
		retryDelay = retryAfter
	}
	return now.Add(retryDelay)
}

func (s *Service) requestFreshRuntimeAfterPortSReset(
	registrar string,
	observedAt, unavailableUntil time.Time,
	reason string,
) {
	s.markPCSCFRegistrationUnboundForPortSReset(registrar)
	err := fmt.Errorf("imscore: P-CSCF %s port-s recovery failed (%s); fresh runtime required", registrar, reason)
	logging.WarnRate("ims-ports-reset-runtime-"+s.DeviceID(), 30*time.Second,
		"IMS port-s reset recovery requires a fresh runtime",
		"device", s.DeviceID(), "policy", vodafoneUKPortSResetRecoveryPolicy,
		"pcscf", registrar, "reason", reason,
		"reset_at", observedAt, "deprioritized_until", unavailableUntil)
	s.reportRegistrationRuntimeError(err)
}
