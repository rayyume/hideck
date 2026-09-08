package imscore

import (
	"strings"
	"time"
)

// This is a Vodafone delivery-validation policy, not a REGISTER failure count.
// The owning penalty store outlives individual IMS services and tunnels.
type registrarDownlinkRound struct {
	number               uint32
	attempted            map[string]bool
	retryAt              time.Time
	rediscoveryRequested bool
}

type downlinkRoundInput struct {
	candidates []string
	current    string // A still-bound path to retain while waiting; empty at startup.
	now        time.Time
	nextRetry  func(uint32) time.Time
}

type downlinkRoundPlan struct {
	next        string
	retryAt     time.Time
	round       uint32
	reuseTunnel bool
	rediscover  bool
}

func (store *RegistrarPenaltyStore) noteDownlinkAttempt(registrar string) uint64 {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureDownlinkRoundLocked()
	return store.noteDownlinkAttemptLocked(registrar)
}

func (store *RegistrarPenaltyStore) noteMTReportFailureAttempt(registrar string, now time.Time) uint64 {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.ensureDownlinkRoundLocked()
	for candidate, entry := range store.entries {
		if entry.reason == vodafoneUKMTReportFailure && now.Before(entry.deprioritizedUntil) {
			store.downlinkRound.attempted[candidate] = true
		}
	}
	return store.noteDownlinkAttemptLocked(registrar)
}

func (store *RegistrarPenaltyStore) ensureDownlinkRoundLocked() {
	if store.downlinkRound == nil {
		store.downlinkRound = &registrarDownlinkRound{number: 1, attempted: make(map[string]bool)}
		// Do not revisit paths already rejected by an actual failure in this
		// incident merely because the first replacement obtained REGISTER 200.
		for candidate := range store.recoveryAttempts {
			store.downlinkRound.attempted[candidate] = true
		}
	}
}

func (store *RegistrarPenaltyStore) noteDownlinkAttemptLocked(registrar string) uint64 {
	store.downlinkRound.attempted[strings.TrimSpace(registrar)] = true
	store.downlinkAttempt++
	return store.downlinkAttempt
}

func (store *RegistrarPenaltyStore) resetDownlinkRound(registrar string) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.resetDownlinkRoundLocked(registrar)
}

// Overlapping IKE reauth shares this store. Only the newest watched path can
// abandon the shared wait; an old service still cancels its own local timer.
func (store *RegistrarPenaltyStore) abandonDownlinkAttempt(registrar string, attempt uint64) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.downlinkRound == nil || store.downlinkAttempt != attempt {
		return
	}
	store.resetDownlinkRoundLocked(registrar)
}

func (store *RegistrarPenaltyStore) resetDownlinkRoundLocked(registrar string) {
	store.downlinkRound = nil
	store.downlinkAttempt++
	store.recoveryAttempts = map[string]bool{registrar: true}
}

// An unverified delivery path only changes preference. It must not manufacture
// failed REGISTER attempts or prevent late evidence from validating this binding.
func (store *RegistrarPenaltyStore) noteUnverifiedDownlink(registrar string, now, retryAfter time.Time) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.entries == nil {
		store.entries = make(map[string]registrarPenaltyEntry)
	}
	entry := store.entries[registrar]
	entry.reason = "downlink_unverified"
	entry.deprioritizedUntil = laterRegistrarDeadline(entry.deprioritizedUntil, now.Add(vodafoneUKPCSCFDeprioritizedPeriod))
	entry.retryNotBefore = laterRegistrarDeadline(entry.retryNotBefore, retryAfter)
	store.entries[registrar] = entry
}

func (store *RegistrarPenaltyStore) planDownlinkRound(input downlinkRoundInput) downlinkRoundPlan {
	store.mu.Lock()
	defer store.mu.Unlock()
	round := store.downlinkRound
	if round == nil {
		return downlinkRoundPlan{}
	}
	if input.now.Before(round.retryAt) {
		return downlinkRoundPlan{retryAt: round.retryAt, round: round.number}
	}
	newRound := !round.retryAt.IsZero()
	states := store.statesLocked(input.now)
	candidates := append([]string(nil), input.candidates...)
	for i, candidate := range candidates {
		if (!newRound && round.attempted[strings.TrimSpace(candidate)]) || candidate == input.current {
			candidates[i] = ""
		}
	}
	if index, ok := preferredRegistrarIndex(candidates, 0, states); ok {
		round.beginNextIfDue(newRound)
		return downlinkRoundPlan{next: candidates[index], round: round.number}
	}
	// A successful REGISTER does not prove the reverse SMS path. Once every
	// candidate assigned to this tunnel has been tried, obtain a fresh P-CSCF
	// set instead of cycling the same addresses in another local round.
	if !newRound && input.current != "" && !round.rediscoveryRequested {
		round.rediscoveryRequested = true
		return downlinkRoundPlan{round: round.number, rediscover: true}
	}
	// A single-candidate recovery can rediscover/recreate its path once the
	// whole-round delay expires, never on every 30-second validation timeout.
	if newRound && input.current != "" && states[input.current].retryNotBefore.IsZero() {
		round.beginNextIfDue(newRound)
		return downlinkRoundPlan{next: input.current, round: round.number}
	}
	round.retryAt = input.nextRetry(round.number)
	if _, eligible := preferredRegistrarIndex(input.candidates, 0, states); !eligible {
		round.retryAt = laterRegistrarDeadline(round.retryAt, earliestRegistrarAvailability(input.candidates, states))
	}
	return downlinkRoundPlan{retryAt: round.retryAt, round: round.number}
}

func (round *registrarDownlinkRound) beginNextIfDue(due bool) {
	if !due {
		return
	}
	round.number++
	round.attempted = make(map[string]bool)
	round.retryAt = time.Time{}
	round.rediscoveryRequested = false
}

func (s *Service) planDownlinkRound(candidates []string, current string) downlinkRoundPlan {
	if !usesVodafoneUKPortSResetRecovery(s.cfg) {
		return downlinkRoundPlan{}
	}
	now := time.Now()
	return s.registrarPenalties.planDownlinkRound(downlinkRoundInput{
		candidates: candidates, current: current, now: now,
		nextRetry: func(round uint32) time.Time {
			// Every exhausted round failed to restore the protected downlink.
			// Escalate the shared retry window instead of rebuilding tunnels at
			// the initial cadence forever. Per-node Retry-After still extends it.
			upper := rfc5626RecoveryUpperBound(round, current == "")
			return now.Add(s.jitterPortSRecoveryDelay(upper))
		},
	})
}
