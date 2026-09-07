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
}

func (s *Service) startReplacementDownlinkWatch(baseline downlinkCheckpoint) {
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
		path: path, baseline: baseline, deadline: time.Now().Add(s.portSFailoverValidationWait()),
	}
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

func (s *Service) replacementDownlinkWatchFired(watch *replacementDownlinkWatch) {
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	if !s.claimReplacementDownlinkFailure(watch) {
		return
	}
	defer s.finishPCSCFRecovery()
	penalty := s.recordVodafoneRegistrarFailure(watch.path.registrar, registrarFailureOptions{
		reason: "downlink_validation_timeout", minimumRetryAt: watch.retryNotBefore,
	})
	s.requestFreshRuntimeAfterPortSFailure(watch.path.registrar, portSFailoverCause{
		reason: "downlink_validation_timeout", observedAt: watch.deadline, deprioritizedUntil: penalty.deprioritizedUntil,
	}, "replacement registration did not establish a downlink")
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
	}
}

func (s *Service) claimReplacementDownlinkFailure(watch *replacementDownlinkWatch) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.replacementDownlinkWatch != watch {
		return false
	}
	watch.timer = nil
	if s.stopped() || s.regState != regRegistered || !watch.path.samePath(s.downlinkCheckpointLocked()) ||
		s.downlinkEvidenceSinceLocked(watch.baseline) != "" {
		s.cancelReplacementDownlinkWatchLocked()
		return false
	}
	if !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return false
	}
	// This is the commit point. A peer accepted afterwards belongs to the
	// rejected path and cannot clear its failure before runtime teardown.
	s.regState = regFailed
	s.replacementDownlinkWatch = nil
	return true
}
