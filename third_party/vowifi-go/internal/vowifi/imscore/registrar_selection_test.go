package imscore

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func recordTestRegistrarFailure(store *RegistrarPenaltyStore, registrar string, now time.Time) registrarPenaltyEntry {
	return store.recordDeprioritizedFailure(registrar, registrarRecoveryInput{
		now: now, reason: "downlink_validation_timeout",
		nextRetry: func(failures uint32) time.Time {
			return now.Add(rfc5626RecoveryUpperBound(failures, true) / 2)
		},
	})
}

func TestRegistrarSelectionUsesDeprioritizedNodesAfterBackoff(t *testing.T) {
	now := time.Now()
	store := NewRegistrarPenaltyStore()
	a := recordTestRegistrarFailure(store, "a:5060", now.Add(-2*time.Minute))
	recordTestRegistrarFailure(store, "b:5060", now.Add(-time.Minute))
	service, err := New(&IMSConfig{Registrar: "a:5060;b:5060", LocalAddr: "192.0.2.10", RegistrarPenalties: store})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.StopCurrent)
	selected, err := service.selectRegistrarCandidate(context.Background(), "tcp")
	if err != nil || selected != "a:5060" {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
	entry := store.states(now)[selected]
	if entry.consecutiveFailures != 1 || !entry.deprioritizedUntil.Equal(a.deprioritizedUntil) {
		t.Fatalf("selection erased recovery history: %+v", entry)
	}
}

func TestRegistrarSelectionPrefersNewCandidateAndRespectsHardDeadlines(t *testing.T) {
	now := time.Now()
	store := NewRegistrarPenaltyStore()
	recordTestRegistrarFailure(store, "a:5060", now.Add(-time.Minute))
	recordTestRegistrarFailure(store, "b:5060", now.Add(-time.Minute))
	candidates := []string{"a:5060", "b:5060", "new:5060"}
	index, ok := preferredRegistrarIndex(candidates, 0, store.states(now))
	if !ok || index != 2 {
		t.Fatalf("new candidate: index=%d ok=%t", index, ok)
	}
	store.mark("a:5060", now.Add(time.Hour))
	store.mark("new:5060", now.Add(time.Minute))
	index, ok = preferredRegistrarIndex(candidates, 0, store.states(now))
	if !ok || index != 1 {
		t.Fatalf("eligible degraded node: index=%d ok=%t", index, ok)
	}
	store.mark("b:5060", now.Add(2*time.Minute))
	if _, ok = preferredRegistrarIndex(candidates, 0, store.states(now)); ok {
		t.Fatal("crossed retry deadline")
	}
	if got := earliestRegistrarAvailability(candidates, store.states(now)); !got.Equal(now.Add(time.Minute)) {
		t.Fatalf("retry is not the earliest hard deadline: %s", got)
	}
}

func TestRegistrarPreferenceDoesNotShortenRetryAfter(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	entry := service.markVodafoneRegistrarFailure("pcscf-a.example:5060", "initial_registration_failed", registerResponseErrorWithRetryAfter(t, "3600"))
	deadline := entry.retryNotBefore
	entry = service.markVodafoneRegistrarFailure("pcscf-a.example:5060", "mt_report_488", nil)
	if !entry.retryNotBefore.Equal(deadline) || entry.consecutiveFailures != 1 {
		t.Fatalf("deadline changed: %+v", entry)
	}
	if !deadline.After(entry.deprioritizedUntil) {
		t.Fatal("fixture did not distinguish preference from Retry-After")
	}
}

func TestRegistrarConcurrentReportsDoNotInflateRecoveryFailures(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	now := time.Now()
	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() { defer group.Done(); recordTestRegistrarFailure(store, "a:5060", now) }()
	}
	group.Wait()
	entry := store.states(now)["a:5060"]
	if entry.consecutiveFailures != 1 || !entry.retryNotBefore.Equal(now.Add(30*time.Second)) {
		t.Fatalf("duplicate reports: %+v", entry)
	}
	entry = recordTestRegistrarFailure(store, "a:5060", now.Add(time.Minute))
	if entry.consecutiveFailures != 2 || !entry.retryNotBefore.Equal(now.Add(2*time.Minute)) {
		t.Fatalf("next attempt did not back off: %+v", entry)
	}
}

func TestVodafoneUnverifiedCandidatesRetryBeforePreferenceExpires(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	before := time.Now()
	service.markVodafoneRegistrarFailure("pcscf-a.example:5060", "port_s_peer_reset", nil)
	service.rejectUnverifiedPortSRegistrar("pcscf-b.example:5060", portSFailoverCause{observedAt: before}, "downlink validation timed out")
	replacement, err := New(&IMSConfig{Registrar: "pcscf-a.example:5060;pcscf-b.example:5060", LocalAddr: "192.0.2.10", RegistrarPenalties: service.registrarPenalties})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(replacement.StopCurrent)
	_, err = replacement.selectRegistrarCandidate(context.Background(), "udp")
	var unavailable *allRegistrarCandidatesUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("missing retry schedule: %v", err)
	}
	if unavailable.RetryAt().Before(before.Add(30*time.Second)) || unavailable.RetryAt().After(time.Now().Add(time.Minute)) {
		t.Fatalf("recovery waits for preference instead of backoff: %s", unavailable.RetryAt())
	}
	states := service.registrarPenalties.states(before.Add(time.Minute))
	index, ok := preferredRegistrarIndex(splitRegistrarCandidates(replacement.cfg.Registrar), 0, states)
	if !ok || index != 0 {
		t.Fatalf("retry remained blocked after cooldown: index=%d ok=%t", index, ok)
	}
	if states["pcscf-a.example:5060"].deprioritizedUntil.IsZero() {
		t.Fatal("retry removed preference history")
	}
}

func TestReplacementStartSchedulesFailedRegistrarRecovery(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go serveRegisterStatus(listener, 503, make(chan string, 1))
	registrar := listener.LocalAddr().String()
	store := NewRegistrarPenaltyStore()
	recordTestRegistrarFailure(store, registrar, time.Now().Add(-2*time.Minute))
	config := registerTransportTestConfig("udp", registrar)
	config.CarrierPresetID = vodafoneUKCarrierPresetID
	config.RegistrarPenalties = store
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.StopCurrent)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	before := time.Now()
	err = service.Start(ctx)
	var scheduled *registrarRecoveryRetryError
	if !errors.As(err, &scheduled) {
		t.Fatalf("missing replacement recovery deadline: %v", err)
	}
	if scheduled.RetryAt().Before(before.Add(time.Minute)) || scheduled.RetryAt().After(time.Now().Add(time.Minute)) {
		t.Fatalf("second failure should wait 60 seconds: %s", scheduled.RetryAt())
	}
	if got := store.states(time.Now())[registrar].consecutiveFailures; got != 2 {
		t.Fatalf("failures=%d", got)
	}
}

func TestInitialRecoverySchedulingDoesNotChangeOtherCarriersOrCancellation(t *testing.T) {
	for _, preset := range []string{"2degrees_nz", "ctexcel_uk", vodafoneUKCarrierPresetID} {
		service := newPortSSessionTestService(t, preset)
		err := errors.New("transport failed")
		if got := service.scheduleInitialRecoveryFailure(err); got != err {
			t.Fatal("ordinary startup changed")
		}
		recordTestRegistrarFailure(service.registrarPenalties, "pcscf-a.example:5060", time.Now().Add(-time.Minute))
		if got := service.scheduleInitialRecoveryFailure(context.Canceled); got != context.Canceled {
			t.Fatal("cancellation changed")
		}
		if preset != vodafoneUKCarrierPresetID && service.scheduleInitialRecoveryFailure(err) != err {
			t.Fatal("other carrier changed")
		}
	}
}

func TestProvenReplacementDownlinkEndsRecoveryWithoutErasingPreferences(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	store := service.registrarPenalties
	recordTestRegistrarFailure(store, "pcscf-a.example:5060", time.Now().Add(-time.Minute))
	// The newly discovered healthy node need not have a penalty entry.
	store.clearFailures(registrarRecoveryAttempt{
		registrar: "pcscf-new.example:5060", generation: store.recoveryGeneration(),
	})
	if store.recoveryInProgress() {
		t.Fatal("healthy replacement did not end incident")
	}
	if store.states(time.Now())["pcscf-a.example:5060"].deprioritizedUntil.IsZero() {
		t.Fatal("healthy replacement erased other node history")
	}
	err := errors.New("later unrelated startup failure")
	if service.scheduleInitialRecoveryFailure(err) != err {
		t.Fatal("old preferences reactivated recovery policy")
	}
}
