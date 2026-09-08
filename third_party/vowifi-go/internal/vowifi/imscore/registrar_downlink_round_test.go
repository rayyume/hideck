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
	plan, claimed := first.claimReplacementDownlinkRecovery(expireReplacementWatchForTest(t, first))
	if !claimed || !plan.reuseTunnel || plan.next != "pcscf-b.example:5060" {
		t.Fatal("untried alternate did not trigger IMS-only replacement")
	}
	// An independent tunnel interruption can still replace the service here.
	second := replacementUsingStore(t, first.registrarPenalties)
	// Even if B has lower historical preference, A has already been tried.
	first.registrarPenalties.noteUnverifiedDownlink("pcscf-b.example:5060", time.Now().Add(time.Minute), time.Time{})
	selected, err := second.selectRegistrarCandidate(context.Background(), "tcp")
	if err != nil || selected != "pcscf-b.example:5060" {
		t.Fatalf("revisited an attempted node: %s, %v", selected, err)
	}
	startProtectedReplacementForTest(t, second)
	second.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, second))
	if second.RegState() != regFailed || len(second.RegistrationErrors()) != 1 {
		t.Fatal("completed candidate set did not request fresh P-CSCF discovery")
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
	if len(s.RegistrationErrors()) != 1 {
		t.Fatal("candidate exhaustion did not request fresh discovery")
	}
	waiting := replacementUsingStore(t, store)
	waiting.cfg.Registrar = s.cfg.Registrar
	_, err := waiting.selectRegistrarCandidate(context.Background(), "tcp")
	var unavailable *allRegistrarCandidatesUnavailableError
	if !errors.As(err, &unavailable) || !unavailable.RetryAt().After(time.Now()) {
		t.Fatalf("same rediscovered set did not enter cooldown: %v", err)
	}
	store.mu.Lock()
	store.downlinkRound.retryAt = time.Now().Add(-time.Second)
	store.mu.Unlock()
	replacement := replacementUsingStore(t, store)
	replacement.cfg.Registrar = s.cfg.Registrar
	if selected, err := replacement.selectRegistrarCandidate(context.Background(), "tcp"); err != nil || selected != s.cfg.Registrar {
		t.Fatalf("new round did not allow rediscovery/re-registration: %s, %v", selected, err)
	}
	startProtectedReplacementForTest(t, replacement)
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
	deadline := s.registrarPenalties.states(time.Now())[s.cfg.Registrar].retryNotBefore
	if deadline.Before(before.Add(time.Hour)) || s.RegState() != regFailed || len(s.RegistrationErrors()) != 1 {
		t.Fatalf("Retry-After lost during rediscovery: %s", deadline)
	}
}

func TestMTReport488StillReplacesDeferredDownlink(t *testing.T) {
	s := singleCandidateReplacement(t)
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
	s.pcscfRecoveryPending.Store(true)
	s.recoverPCSCFAfterPortSReset(s.cfg.Registrar, time.Now())
	if s.RegState() == regRegistered || len(s.RegistrationErrors()) != 1 {
		t.Fatal("confirmed RST was blocked by the pending validation round")
	}
}

func TestStale488DoesNotRestartDeferredDownlinkRound(t *testing.T) {
	s := singleCandidateReplacement(t)
	s.mu.RLock()
	watch, deadline := s.replacementDownlinkWatch, s.replacementDownlinkWatch.deadline
	s.mu.RUnlock()
	s.pcscfRecoveryPending.Store(true)
	s.recoverPCSCFAfterMTReportReject("old.example:5060", 488, time.Now().Add(time.Minute))
	s.mu.RLock()
	unchanged := s.replacementDownlinkWatch == watch && watch.deadline.Equal(deadline)
	s.mu.RUnlock()
	if !unchanged || len(s.RegistrationErrors()) != 0 {
		t.Fatal("stale path rejection bypassed or reset the current recovery round")
	}
}

func TestOtherCarriersDoNotUseVodafoneDownlinkRounds(t *testing.T) {
	s := singleCandidateReplacement(t)
	for _, carrier := range []string{"2degrees_nz", "ctexcel", ""} {
		replacement := replacementUsingStore(t, s.registrarPenalties)
		replacement.cfg.CarrierPresetID = carrier
		if _, err := replacement.selectRegistrarCandidate(context.Background(), "tcp"); err != nil {
			t.Fatalf("Vodafone round leaked into %q: %v", carrier, err)
		}
	}
}

func TestReplacementSingleCandidateRequestsFreshDiscovery(t *testing.T) {
	s := singleCandidateReplacement(t)
	watch := expireReplacementWatchForTest(t, s)
	s.replacementDownlinkWatchFired(watch)
	if s.RegState() != regFailed || len(s.RegistrationErrors()) != 1 {
		t.Fatal("single assigned P-CSCF was retried without fresh discovery")
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
	watch := expireReplacementWatchForTest(t, s)
	peer := s.registrationTCP
	s.recordCurrentDownlinkRequest(peer, s.captureDownlinkCheckpoint())
	s.replacementDownlinkWatchFired(watch)
	s.mu.RLock()
	pending := s.replacementDownlinkWatch
	s.mu.RUnlock()
	if pending != nil || s.registrarPenalties.recoveryInProgress() || len(s.RegistrationErrors()) != 0 {
		t.Fatal("late current-path downlink did not cancel deferred recovery")
	}
}
