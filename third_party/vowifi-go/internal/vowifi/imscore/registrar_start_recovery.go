package imscore

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type registrarRecoveryRetryError struct {
	err     error
	retryAt time.Time
}

func (err *registrarRecoveryRetryError) Error() string {
	return fmt.Sprintf("%v; next P-CSCF recovery attempt at %s", err.err, err.retryAt.Format(time.RFC3339))
}

func (err *registrarRecoveryRetryError) Unwrap() error      { return err.err }
func (err *registrarRecoveryRetryError) RetryAt() time.Time { return err.retryAt }

// A replacement service must not reset the recovery attempt count when its
// initial REGISTER fails. Ordinary startup and other carriers stay unchanged.
func (s *Service) scheduleInitialRecoveryFailure(err error) error {
	if err == nil || !usesVodafoneUKPortSResetRecovery(s.cfg) || errors.Is(err, context.Canceled) {
		return err
	}
	var scheduled interface{ RetryAt() time.Time }
	if errors.As(err, &scheduled) {
		return err
	}
	now := time.Now()
	if !s.registrarPenalties.recoveryInProgress() {
		return err
	}
	registrar := s.currentPortSRecoveryRegistrar()
	if registrar == "" {
		return err
	}
	s.markVodafoneRegistrarFailure(registrar, "initial_registration_failed", err)
	s.mu.RLock()
	candidates := append([]string(nil), s.registrarCandidates...)
	s.mu.RUnlock()
	states := s.registrarPenalties.states(now)
	retryAt := now
	if _, available := preferredRegistrarIndex(candidates, 0, states); !available {
		retryAt = earliestRegistrarAvailability(candidates, states)
	}
	return &registrarRecoveryRetryError{err: err, retryAt: retryAt}
}
