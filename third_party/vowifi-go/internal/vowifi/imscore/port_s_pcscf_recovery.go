package imscore

import (
	"strings"
	"time"
)

const (
	vodafoneUKCarrierPresetID           = "vodafone_uk_23415"
	vodafoneUKPortSResetRecoveryPolicy  = "vodafone_uk_port_s_reset"
	vodafoneUKPortSResetReconnectGrace  = 5 * time.Second
	vodafoneUKPortSReconnectGrace       = 30 * time.Second
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
		s.finishPCSCFRecovery()
		return
	}
	go s.recoverPCSCFAfterPortSReset(failed, observedAt)
}

func (s *Service) recoverPCSCFAfterPortSReset(failedRegistrar string, observedAt time.Time) {
	defer s.finishPCSCFRecovery()
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	s.recoverPCSCFAfterPortSFailureLocked(failedRegistrar, portSFailoverCause{
		reason: "port_s_peer_reset", observedAt: observedAt,
	})
}
