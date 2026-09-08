package imscore

import (
	"testing"
	"time"
)

func TestDownlinkRoundDoesNotCountWaitingAsAnotherAttempt(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	store.noteDownlinkAttempt("a:5060")
	now := time.Now()
	input := downlinkRoundInput{
		candidates: []string{"a:5060"}, current: "a:5060", now: now,
		nextRetry: func(uint32) time.Time { return now.Add(time.Minute) },
	}
	first := store.planDownlinkRound(input)
	if !first.rediscover {
		t.Fatal("completed set did not request P-CSCF rediscovery")
	}
	input.current = ""
	first = store.planDownlinkRound(input)
	store.mark("a:5060", now.Add(time.Hour))
	input.now = first.retryAt.Add(time.Second)
	input.nextRetry = func(uint32) time.Time { return input.now.Add(time.Minute) }
	second := store.planDownlinkRound(input)
	if second.round != first.round || second.next != "" || second.retryAt.Before(now.Add(time.Hour)) {
		t.Fatalf("waiting on Retry-After consumed another recovery round: %+v", second)
	}
}

func TestDownlinkRoundFreshDiscoveryUsesANewCandidateImmediately(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	store.noteDownlinkAttempt("a:5060")
	now := time.Now()
	input := downlinkRoundInput{
		candidates: []string{"a:5060"}, current: "a:5060", now: now,
		nextRetry: func(uint32) time.Time { return now.Add(time.Minute) },
	}
	first := store.planDownlinkRound(input)
	if !first.rediscover {
		t.Fatal("completed set did not request P-CSCF rediscovery")
	}
	input.candidates = []string{"new:5060", "a:5060"}
	input.current = ""
	input.now = now.Add(time.Second)
	if ready := store.planDownlinkRound(input); ready.next != "new:5060" || ready.round != 1 {
		t.Fatalf("freshly discovered P-CSCF was not selected: %+v", ready)
	}
}

func TestDownlinkRoundFreshDiscoveryOccursOncePerRound(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	now := time.Now()
	store.noteDownlinkAttempt("a:5060")
	if plan := store.planDownlinkRound(downlinkRoundInput{
		candidates: []string{"a:5060"}, current: "a:5060", now: now,
		nextRetry: func(uint32) time.Time { return now.Add(time.Minute) },
	}); !plan.rediscover {
		t.Fatalf("first exhausted set did not request discovery: %+v", plan)
	}

	fresh := downlinkRoundInput{
		candidates: []string{"c:5060", "d:5060"}, now: now.Add(time.Second),
		nextRetry: func(uint32) time.Time { return now.Add(time.Minute) },
	}
	if plan := store.planDownlinkRound(fresh); plan.next != "c:5060" {
		t.Fatalf("newly discovered candidate not selected: %+v", plan)
	}
	store.noteDownlinkAttempt("c:5060")
	fresh.current = "c:5060"
	if plan := store.planDownlinkRound(fresh); plan.next != "d:5060" {
		t.Fatalf("fresh alternate not selected: %+v", plan)
	}
	store.noteDownlinkAttempt("d:5060")
	fresh.current = "d:5060"
	if plan := store.planDownlinkRound(fresh); plan.rediscover || !plan.retryAt.After(now) {
		t.Fatalf("same round requested repeated discovery: %+v", plan)
	}
}

func TestDownlinkRoundBackoffEscalatesByRound(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper }
	s.registrarPenalties.mu.Lock()
	s.registrarPenalties.downlinkRound.number = 3
	s.registrarPenalties.downlinkRound.rediscoveryRequested = true
	s.registrarPenalties.mu.Unlock()

	now := time.Now()
	plan := s.planDownlinkRound([]string{s.cfg.Registrar}, s.cfg.Registrar)
	want := rfc5626RecoveryUpperBound(3, false)
	if delta := plan.retryAt.Sub(now); delta < want-time.Second || delta > want+time.Second {
		t.Fatalf("round backoff = %s, want %s", delta, want)
	}
}

func TestDownlinkRoundSameRediscoveredSetWaitsBeforeReuse(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	store.noteDownlinkAttempt("a:5060")
	now := time.Now()
	input := downlinkRoundInput{
		candidates: []string{"a:5060"}, current: "a:5060", now: now,
		nextRetry: func(uint32) time.Time { return now.Add(time.Minute) },
	}
	if plan := store.planDownlinkRound(input); !plan.rediscover {
		t.Fatalf("completed set did not request rediscovery: %+v", plan)
	}
	input.current = ""
	if plan := store.planDownlinkRound(input); plan.next != "" || !plan.retryAt.After(now) {
		t.Fatalf("unchanged P-CSCF set bypassed cooldown: %+v", plan)
	}
}

func TestDownlinkRoundDoesNotSeedUnrelatedHistoricalFailures(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	now := time.Now()
	recordTestRegistrarFailure(store, "old:5060", now.Add(-time.Minute))
	store.clearFailures(registrarRecoveryAttempt{registrar: "healthy:5060", generation: store.recoveryGeneration()})
	recordTestRegistrarFailure(store, "failed:5060", now)
	store.noteDownlinkAttempt("replacement:5060")
	plan := store.planDownlinkRound(downlinkRoundInput{
		candidates: []string{"failed:5060", "replacement:5060", "old:5060"},
		current:    "replacement:5060", now: now,
		nextRetry: func(uint32) time.Time { return now.Add(time.Minute) },
	})
	if plan.next != "old:5060" {
		t.Fatalf("a previous incident consumed an attempt in the current round: %+v", plan)
	}
}

func TestDownlinkRoundAbandonIsAttemptScopedOnSameRegistrar(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	old := store.noteDownlinkAttempt("a:5060")
	current := store.noteDownlinkAttempt("a:5060")
	deadline := time.Now().Add(time.Hour)
	store.mark("a:5060", deadline)
	store.abandonDownlinkAttempt("a:5060", old)
	if store.downlinkRound == nil {
		t.Fatal("old attempt cleared a newer path using the same registrar")
	}
	store.abandonDownlinkAttempt("a:5060", current)
	if store.downlinkRound != nil || !store.states(time.Now())["a:5060"].retryNotBefore.Equal(deadline) {
		t.Fatal("current attempt did not cancel only its passive wait")
	}
}

func TestDownlinkRoundAbandonCannotReviveCompletedRecovery(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	attempt := store.noteDownlinkAttempt("a:5060")
	store.clearFailures(registrarRecoveryAttempt{registrar: "a:5060"})
	store.abandonDownlinkAttempt("a:5060", attempt)
	if store.downlinkRound != nil || len(store.recoveryAttempts) != 0 {
		t.Fatal("late transport failure restored attempts from a completed incident")
	}
}
