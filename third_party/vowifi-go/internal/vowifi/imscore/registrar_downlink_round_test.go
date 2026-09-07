package imscore

import (
	"context"
	"errors"
	"testing"
	"time"
)

func singleCandidateReplacement(t *testing.T) *Service {
	t.Helper()
	s := newRecoveryCompletionTestService(t)
	s.cfg.Registrar = "pcscf-a.example:5060"
	s.registrarCandidates = []string{s.cfg.Registrar}
	startProtectedReplacementForTest(t, s)
	return s
}

func replacementUsingStore(t *testing.T, store *RegistrarPenaltyStore) *Service {
	t.Helper()
	cfg := registerTransportTestConfig("udp", "pcscf-a.example:5060;pcscf-b.example:5060")
	cfg.CarrierPresetID = vodafoneUKCarrierPresetID
	cfg.RegistrarPenalties = store
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.StopCurrent)
	s.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	return s
}

func TestDownlinkRoundSurvivesReplacementServices(t *testing.T) {
	first := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, first)
	first.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, first))
	if len(first.RegistrationErrors()) != 1 {
		t.Fatal("untried alternate did not trigger replacement")
	}
	second := replacementUsingStore(t, first.registrarPenalties)
	// Even if B has lower historical preference, A has already been tried.
	first.registrarPenalties.noteUnverifiedDownlink("pcscf-b.example:5060", time.Now().Add(time.Minute), time.Time{})
	selected, err := second.selectRegistrarCandidate(context.Background(), "tcp")
	if err != nil || selected != "pcscf-b.example:5060" {
		t.Fatalf("revisited an attempted node: %s, %v", selected, err)
	}
	startProtectedReplacementForTest(t, second)
	second.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, second))
	if second.RegState() != regRegistered || len(second.RegistrationErrors()) != 0 {
		t.Fatal("completed round tore down its last successful registration")
	}
	third := replacementUsingStore(t, first.registrarPenalties)
	_, err = third.selectRegistrarCandidate(context.Background(), "tcp")
	var waiting *allRegistrarCandidatesUnavailableError
	if !errors.As(err, &waiting) || !waiting.RetryAt().After(time.Now()) {
		t.Fatalf("new runtime bypassed the shared round deadline: %v", err)
	}
	if _, nextErr := third.selectRegistrarCandidate(context.Background(), "udp"); nextErr == nil || nextErr.Error() != err.Error() {
		t.Fatalf("UDP fallback changed the recovery round: %v / %v", err, nextErr)
	}
}

func TestDownlinkRoundRetriesAfterCooldownAndCanBeCanceled(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	store := s.registrarPenalties
	store.mu.Lock()
	store.downlinkRound.retryAt = time.Now().Add(-time.Second)
	store.mu.Unlock()
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	if len(s.RegistrationErrors()) != 1 {
		t.Fatal("round exhaustion permanently disabled automatic recovery")
	}
	replacement := replacementUsingStore(t, store)
	replacement.cfg.Registrar = s.cfg.Registrar
	if selected, err := replacement.selectRegistrarCandidate(context.Background(), "tcp"); err != nil || selected != s.cfg.Registrar {
		t.Fatalf("new round did not allow rediscovery/re-registration: %s, %v", selected, err)
	}
	startProtectedReplacementForTest(t, replacement)
	replacement.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, replacement))
	store.mu.Lock()
	round, deadline := store.downlinkRound.number, store.downlinkRound.retryAt
	store.mu.Unlock()
	if round != 2 || time.Until(deadline) < 89*time.Second || time.Until(deadline) > 91*time.Second {
		t.Fatalf("passive validation inflated registration backoff: round=%d retry=%s", round, deadline)
	}
	replacement.trackProtectedConnection(newRecoveryCompletionPortS(t))
	if store.recoveryInProgress() || replacement.replacementDownlinkWatch != nil {
		t.Fatal("proven replacement did not end the incident")
	}
}

func TestDownlinkRoundRetryDoesNotShortenRetryAfter(t *testing.T) {
	s := singleCandidateReplacement(t)
	before := time.Now()
	s.retainReplacementRegisterRetryAfter(registerResponseErrorWithRetryAfter(t, "3600"))
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	s.mu.RLock()
	deadline := s.replacementDownlinkWatch.deadline
	s.mu.RUnlock()
	if deadline.Before(before.Add(time.Hour)) || s.RegState() != regRegistered {
		t.Fatalf("Retry-After lost or binding torn down: %s", deadline)
	}
	// An old timer firing again cannot reschedule, consume another round or tear down.
	s.replacementDownlinkWatchFired(s.replacementDownlinkWatch)
	if s.RegState() != regRegistered || len(s.RegistrationErrors()) != 0 {
		t.Fatal("stale timer bypassed the shared retry deadline")
	}
}

func TestMTReport488StillReplacesDeferredDownlink(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	s.triggerMTReportPCSCFRecovery(&rpReportRejectError{Status: 488, Registrar: s.cfg.Registrar})
	select {
	case <-s.RegistrationErrors():
	case <-time.After(time.Second):
		t.Fatal("488 recovery was blocked by the idle downlink round")
	}
	s.registrarPenalties.mu.Lock()
	pending := s.registrarPenalties.downlinkRound != nil
	s.registrarPenalties.mu.Unlock()
	if pending {
		t.Fatal("488 carried the previous validation round into replacement")
	}
}

func TestConfirmedResetStillReplacesDeferredDownlink(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	s.pcscfRecoveryPending.Store(true)
	s.recoverPCSCFAfterPortSReset(s.cfg.Registrar, time.Now())
	if s.RegState() == regRegistered || len(s.RegistrationErrors()) != 1 {
		t.Fatal("confirmed RST was blocked by the pending validation round")
	}
}

func TestStale488DoesNotRestartDeferredDownlinkRound(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	s.mu.RLock()
	watch, deadline := s.replacementDownlinkWatch, s.replacementDownlinkWatch.deadline
	s.mu.RUnlock()
	s.pcscfRecoveryPending.Store(true)
	s.requestFreshRuntimeAfterMTReportReject("old.example:5060", 488, time.Now().Add(time.Minute))
	s.mu.RLock()
	unchanged := s.replacementDownlinkWatch == watch && watch.deadline.Equal(deadline)
	s.mu.RUnlock()
	if !unchanged || len(s.RegistrationErrors()) != 0 {
		t.Fatal("stale path rejection bypassed or reset the current recovery round")
	}
}

func TestOtherCarriersDoNotUseVodafoneDownlinkRounds(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	for _, carrier := range []string{"2degrees_nz", "ctexcel", ""} {
		replacement := replacementUsingStore(t, s.registrarPenalties)
		replacement.cfg.CarrierPresetID = carrier
		if _, err := replacement.selectRegistrarCandidate(context.Background(), "tcp"); err != nil {
			t.Fatalf("Vodafone round leaked into %q: %v", carrier, err)
		}
	}
}

func TestReplacementSingleCandidateKeepsRegisteredTransport(t *testing.T) {
	s := singleCandidateReplacement(t)
	peer := s.registrationTCP
	watch := expireReplacementWatchForTest(t, s)
	s.replacementDownlinkWatchFired(watch)
	if s.RegState() != regRegistered || s.registrationTCP != peer || len(s.RegistrationErrors()) != 0 {
		t.Fatal("missing downlink tore down a successful registration with no alternate")
	}
	s.mu.RLock()
	pending := s.replacementDownlinkWatch
	s.mu.RUnlock()
	if pending == nil || !pending.deadline.After(time.Now()) {
		t.Fatal("retained registration has no scheduled recovery")
	}
	if s.SMSReadiness().Ready {
		t.Fatal("unverified downlink was reported ready")
	}
}

func TestReplacementValidationDoesNotInflateRegistrationFailures(t *testing.T) {
	s := singleCandidateReplacement(t)
	before := s.registrarPenalties.states(time.Now())[s.cfg.Registrar].consecutiveFailures
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	entry := s.registrarPenalties.states(time.Now())[s.cfg.Registrar]
	if entry.consecutiveFailures != before {
		t.Fatalf("REGISTER succeeded but failure count increased: %d -> %d", before, entry.consecutiveFailures)
	}
}

func TestReplacementLateDownlinkCancelsRoundRetry(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	peer := s.registrationTCP
	s.recordCurrentDownlinkRequest(peer, s.captureDownlinkCheckpoint())
	s.mu.RLock()
	pending := s.replacementDownlinkWatch
	s.mu.RUnlock()
	if pending != nil || s.registrarPenalties.recoveryInProgress() || len(s.RegistrationErrors()) != 0 {
		t.Fatal("late current-path downlink did not cancel deferred recovery")
	}
}
