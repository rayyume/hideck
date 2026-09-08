package imscore

import (
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

// Protected by Service.mu. Keep the watch after a busy callback so the current
// recovery owner can hand it back without changing the original deadline.
type replacementDownlinkWatch struct {
	path           downlinkCheckpoint
	baseline       downlinkCheckpoint
	deadline       time.Time
	timer          *time.Timer
	retryNotBefore time.Time
	unverified     bool
	sharedAttempt  uint64
}

func (s *Service) startReplacementDownlinkWatch(baseline downlinkCheckpoint) {
	s.watchReplacementDownlink(baseline, s.portSFailoverValidationWait())
}

func (s *Service) watchReplacementDownlink(baseline downlinkCheckpoint, delay time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopped() || s.regState != regRegistered || !usesVodafoneUKPortSResetRecovery(s.cfg) ||
		!s.registrarPenalties.recoveryInProgress() || !s.protectedSMSPushRequiredLocked() ||
		s.downlinkEvidenceSinceLocked(baseline) != "" {
		return
	}
	path := s.downlinkCheckpointLocked()
	if watch := s.replacementDownlinkWatch; watch != nil && watch.path.samePath(path) {
		return
	}
	s.cancelReplacementDownlinkWatchLocked()
	watch := &replacementDownlinkWatch{
		path: path, baseline: baseline, deadline: time.Now().Add(delay),
	}
	watch.sharedAttempt = s.registrarPenalties.noteDownlinkAttempt(path.registrar)
	s.replacementDownlinkWatch = watch
	s.armReplacementDownlinkWatchLocked(watch)
	logging.Info("IMS replacement registered; awaiting downlink validation",
		"device", s.DeviceID(), "pcscf", path.registrar, "validation_deadline", watch.deadline)
}

func (s *Service) armReplacementDownlinkWatchLocked(watch *replacementDownlinkWatch) {
	watch.timer = time.AfterFunc(time.Until(watch.deadline), func() {
		s.replacementDownlinkWatchFired(watch)
	})
}

func (s *Service) cancelReplacementDownlinkWatchLocked() {
	if watch := s.replacementDownlinkWatch; watch != nil && watch.timer != nil {
		watch.timer.Stop()
	}
	s.replacementDownlinkWatch = nil
}

// A dead current transport must be repaired, not held by a passive validation
// cadence for a connection that no longer exists. Keep actual node deadlines.
func (s *Service) abandonReplacementDownlinkWaitLocked() {
	watch := s.replacementDownlinkWatch
	if watch == nil || !watch.path.samePath(s.downlinkCheckpointLocked()) {
		return
	}
	s.registrarPenalties.abandonDownlinkAttempt(watch.path.registrar, watch.sharedAttempt)
	s.cancelReplacementDownlinkWatchLocked()
}

func (s *Service) replacementDownlinkWatchFired(watch *replacementDownlinkWatch) {
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	plan, claimed := s.claimReplacementDownlinkRecovery(watch)
	if !claimed {
		return
	}
	defer s.finishPCSCFRecovery()
	s.logDownlinkDiagnostics("replacement_validation_expired")
	if !plan.retryAt.IsZero() {
		logging.Info("IMS downlink recovery round exhausted; keeping registration pending validation",
			"device", s.DeviceID(), "pcscf", watch.path.registrar,
			"round", plan.round, "retry_at", plan.retryAt)
		return
	}
	logging.Info("IMS downlink recovery continuing candidate round",
		"device", s.DeviceID(), "pcscf", watch.path.registrar, "next", plan.next, "round", plan.round)
	penalty := s.registrarPenalties.states(time.Now())[watch.path.registrar]
	cause := portSFailoverCause{
		reason: "downlink_validation_timeout", observedAt: time.Now(), deprioritizedUntil: penalty.deprioritizedUntil,
	}
	if plan.reuseTunnel {
		s.recoverPortSOnAlternate(watch.path.registrar, plan.next, cause)
		return
	}
	s.requestFreshRuntimeAfterPortSFailure(watch.path.registrar, cause,
		"replacement registration did not establish a downlink; rediscovery or transport repair required")
}

func (s *Service) retainReplacementRegisterRetryAfter(err error) {
	delay, present := registerRetryAfterFromError(err)
	if !present {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	watch := s.replacementDownlinkWatch
	if watch != nil && watch.path.samePath(s.downlinkCheckpointLocked()) {
		watch.retryNotBefore = laterRegistrarDeadline(watch.retryNotBefore, time.Now().Add(delay))
		s.registrarPenalties.mark(watch.path.registrar, watch.retryNotBefore)
	}
}

func (s *Service) claimReplacementDownlinkRecovery(watch *replacementDownlinkWatch) (downlinkRoundPlan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replacementDownlinkWatch != watch || time.Now().Before(watch.deadline) {
		return downlinkRoundPlan{}, false
	}
	watch.timer = nil
	if s.stopped() || s.regState != regRegistered || !watch.path.samePath(s.downlinkCheckpointLocked()) ||
		s.downlinkEvidenceSinceLocked(watch.baseline) != "" {
		s.cancelReplacementDownlinkWatchLocked()
		return downlinkRoundPlan{}, false
	}
	if !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return downlinkRoundPlan{}, false
	}
	if !watch.unverified {
		s.registrarPenalties.noteUnverifiedDownlink(watch.path.registrar, time.Now(), watch.retryNotBefore)
		watch.unverified = true
	}
	plan := downlinkRoundPlan{next: watch.path.registrar}
	if s.registeredSIPTransportReadyLocked() {
		plan = s.planDownlinkRound(s.registrarCandidates, watch.path.registrar)
	}
	if !plan.retryAt.IsZero() {
		watch.deadline = plan.retryAt
		// finishPCSCFRecovery rearms this same watch without resetting the round.
		return plan, true
	}
	plan.reuseTunnel = s.selectDownlinkAlternateLocked(plan.next)
	// This is the commit point. A peer accepted afterwards belongs to the
	// rejected path and cannot clear its failure before runtime teardown.
	s.regState = regFailed
	s.replacementDownlinkWatch = nil
	return plan, true
}

func (s *Service) selectDownlinkAlternateLocked(next string) bool {
	if next == "" || next == s.registrar || !s.registeredSIPTransportReadyLocked() {
		return false
	}
	for index, candidate := range s.registrarCandidates {
		if candidate == next {
			s.registrar, s.registrarIndex = candidate, index
			return true
		}
	}
	return false
}
