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
	store.mark("a:5060", now.Add(time.Hour))
	input.now = first.retryAt.Add(time.Second)
	input.nextRetry = func(uint32) time.Time { return input.now.Add(time.Minute) }
	second := store.planDownlinkRound(input)
	if second.round != first.round || second.next != "" || second.retryAt.Before(now.Add(time.Hour)) {
		t.Fatalf("waiting on Retry-After consumed another recovery round: %+v", second)
	}
}

func TestDownlinkRoundNewDiscoveryCannotEraseCooldown(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	store.noteDownlinkAttempt("a:5060")
	now := time.Now()
	input := downlinkRoundInput{
		candidates: []string{"a:5060"}, now: now,
		nextRetry: func(uint32) time.Time { return now.Add(time.Minute) },
	}
	first := store.planDownlinkRound(input)
	input.candidates = []string{"new:5060", "a:5060"}
	input.now = now.Add(time.Second)
	if waiting := store.planDownlinkRound(input); !waiting.retryAt.Equal(first.retryAt) || waiting.next != "" {
		t.Fatalf("rediscovery bypassed the completed round's delay: %+v", waiting)
	}
	input.now = first.retryAt.Add(time.Second)
	if ready := store.planDownlinkRound(input); ready.next != "new:5060" || ready.round != 2 {
		t.Fatalf("fresh discovery was not available in the next round: %+v", ready)
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
