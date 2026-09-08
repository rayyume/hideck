package imscore

import (
	"math"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

// The 30-minute preference is an empirical Vodafone UK policy, not a server
// Retry-After. Eligibility is controlled independently by recovery backoff.
func (s *Service) markVodafoneRegistrarFailure(registrar, reason string, err error) registrarPenaltyEntry {
	return s.recordVodafoneRegistrarFailure(registrar, registrarFailureOptions{reason: reason, err: err})
}

type registrarFailureOptions struct {
	reason         string
	err            error
	minimumRetryAt time.Time
}

func (s *Service) recordVodafoneRegistrarFailure(registrar string, options registrarFailureOptions) registrarPenaltyEntry {
	now := time.Now()
	retryAfter := options.minimumRetryAt
	if delay, present := registerRetryAfterFromError(options.err); present {
		retryAfter = laterRegistrarDeadline(retryAfter, now.Add(delay))
	}
	entry := s.registrarPenalties.recordDeprioritizedFailure(registrar, registrarRecoveryInput{
		now: now, reason: options.reason, minimumRetryAt: retryAfter,
		nextRetry: func(failures uint32) time.Time {
			return s.failedRegisterUnavailableUntil(options.err, now, failures)
		},
	})
	logging.Info("IMS P-CSCF recovery preference and retry scheduled",
		"device", s.DeviceID(), "pcscf", registrar, "reason", options.reason,
		"deprioritized_until", entry.deprioritizedUntil,
		"retry_not_before", entry.retryNotBefore, "failures", entry.consecutiveFailures)
	return entry
}

type registrarRecoveryInput struct {
	now            time.Time
	reason         string
	minimumRetryAt time.Time
	nextRetry      func(uint32) time.Time
}

func (store *RegistrarPenaltyStore) recordDeprioritizedFailure(registrar string, input registrarRecoveryInput) registrarPenaltyEntry {
	registrar = strings.TrimSpace(registrar)
	if store == nil || registrar == "" {
		return registrarPenaltyEntry{}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	wasRecovering := store.recovering
	if !store.recovering || store.recoveryAttempts == nil {
		store.recoveryAttempts = make(map[string]bool)
	}
	store.recovering = true
	store.updateRecoveryModeLocked(input.reason, wasRecovering)
	store.recoveryAttempts[registrar] = true
	store.generation++
	if store.entries == nil {
		store.entries = make(map[string]registrarPenaltyEntry)
	}
	entry := store.entries[registrar]
	entry.failureGeneration = store.generation
	// Reports during an existing cooldown are not additional recovery attempts.
	// Only a later server Retry-After may extend this retry deadline.
	if !input.now.Before(entry.retryNotBefore) {
		if entry.consecutiveFailures < math.MaxUint32 {
			entry.consecutiveFailures++
		}
		entry.retryNotBefore = input.nextRetry(entry.consecutiveFailures)
	}
	entry.retryNotBefore = laterRegistrarDeadline(entry.retryNotBefore, input.minimumRetryAt)
	entry.deprioritizedUntil = laterRegistrarDeadline(entry.deprioritizedUntil, input.now.Add(vodafoneUKPCSCFDeprioritizedPeriod))
	entry.reason = input.reason
	store.entries[registrar] = entry
	if store.downlinkRound != nil {
		store.downlinkRound.attempted[registrar] = true
	}
	return entry
}

func (store *RegistrarPenaltyStore) updateRecoveryModeLocked(reason string, wasRecovering bool) {
	switch reason {
	case vodafoneUKMTReportFailure:
		store.recoveryMode = registrarRecoveryModeMTReportRedelivery
	case "initial_registration_failed":
		if !wasRecovering || store.recoveryMode == registrarRecoveryModeNone {
			store.recoveryMode = registrarRecoveryModeDownlinkValidation
		}
	default:
		store.recoveryMode = registrarRecoveryModeDownlinkValidation
	}
}

func laterRegistrarDeadline(current, next time.Time) time.Time {
	if next.After(current) {
		return next
	}
	return current
}
