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
		err := fmt.Errorf(
			"imscore: P-CSCF %s reset port-s and no alternate P-CSCF is available; fresh runtime required",
			failedRegistrar,
		)
		s.markPCSCFRegistrationUnboundForPortSReset(failedRegistrar)
		logging.WarnRate("ims-ports-reset-no-alternate-"+s.DeviceID(), 30*time.Second,
			"IMS port-s reset recovery has no alternate P-CSCF; rebuilding runtime",
			"device", s.DeviceID(), "policy", vodafoneUKPortSResetRecoveryPolicy,
			"pcscf", failedRegistrar,
			"reset_at", observedAt, "deprioritized_until", unavailableUntil)
		s.reportRegistrationRuntimeError(err)
		return
	}
	s.markPCSCFRegistrationUnboundForPortSReset(failedRegistrar)
	if err := s.resetRegistrationForPCSCFSwitch(); err != nil {
		s.reportRegistrationRuntimeError(err)
		return
	}
	logging.WarnRate("ims-ports-reset-switch-"+s.DeviceID(), 30*time.Second,
		"IMS port-s reset recovery switching P-CSCF",
		"device", s.DeviceID(), "policy", vodafoneUKPortSResetRecoveryPolicy,
		"previous", failedRegistrar, "next", next,
		"reset_at", observedAt, "deprioritized_until", unavailableUntil)
	ctx, cancel := context.WithTimeout(context.Background(), pcscfInitialRegistrationTimeout)
	defer cancel()
	if err := s.registerLocked(ctx); err != nil {
		s.reportRegistrationRuntimeError(fmt.Errorf("imscore: alternate P-CSCF registration after port-s reset failed: %w", err))
		return
	}
	logging.Info("IMS port-s reset recovery completed",
		"device", s.DeviceID(), "policy", vodafoneUKPortSResetRecoveryPolicy,
		"registrar", next)
}
