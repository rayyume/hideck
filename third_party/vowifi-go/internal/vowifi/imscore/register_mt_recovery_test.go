package imscore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMTReport488DuringRegisterRefreshPreservesReplacementRecovery(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	conn := newRecoveryCompletionPortS(t)
	service.transport.SetSendFn(func(request string) error {
		service.transport.DeliverResponse(registerResponseForRequest(request, 200, nil))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Start(ctx); err != nil {
		t.Fatal(err)
	}
	service.trackProtectedConnection(conn)
	service.transport.SetSendFn(func(request string) error {
		if strings.HasPrefix(request, "REGISTER ") {
			service.triggerMTReportPCSCFRecovery(&rpReportRejectError{Status: 488, Registrar: "pcscf-a.example:5060"})
		}
		service.transport.DeliverResponse(registerResponseForRequest(request, 200, nil))
		return nil
	})
	if err := service.Register(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-service.RegistrationErrors():
	case <-ctx.Done():
		t.Fatal("missing runtime rebuild after report rejection")
	}
	store := service.registrarPenalties
	if !store.recoveryInProgress() || store.states(time.Now())["pcscf-a.example:5060"].consecutiveFailures != 1 {
		t.Fatal("old REGISTER and existing port-s cleared the recovery incident")
	}
	assertReplacementRecoveryRetryAfter(t, store)
}

func assertReplacementRecoveryRetryAfter(t *testing.T, store *RegistrarPenaltyStore) {
	t.Helper()
	replacement := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	replacement.registrarPenalties = store
	replacement.registrar = "pcscf-b.example:5060"
	replacement.registrarCandidates = []string{replacement.registrar}
	replacement.transport.SetSendFn(func(request string) error {
		replacement.transport.DeliverResponse(registerResponseForRequest(request, 503, map[string]string{"Retry-After": "3600"}))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	before := time.Now()
	err := replacement.Start(ctx)
	var scheduled *registrarRecoveryRetryError
	if !errors.As(err, &scheduled) || scheduled.RetryAt().Before(before.Add(time.Hour)) {
		t.Fatalf("replacement lost recovery Retry-After: %v", err)
	}
	entry := store.states(time.Now())[replacement.registrar]
	if entry.consecutiveFailures != 1 || !entry.retryNotBefore.Equal(scheduled.RetryAt()) {
		t.Fatalf("replacement lost failure history: %+v", entry)
	}
}

func TestMTReport488RejectsOldDownlinkAfterRegisterSuccess(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.transport.SetSendFn(func(request string) error {
		service.transport.DeliverResponse(registerResponseForRequest(request, 200, nil))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Start(ctx); err != nil {
		t.Fatal(err)
	}
	service.registerMu.Lock()
	defer service.registerMu.Unlock()
	service.triggerMTReportPCSCFRecovery(&rpReportRejectError{Status: 488, Registrar: "pcscf-a.example:5060"})
	service.trackProtectedConnection(newRecoveryCompletionPortS(t))
	service.inboundSIPHandledRequest.Add(1)
	service.confirmCurrentRegistrarDownlinkHealthy()
	if !service.registrarPenalties.recoveryInProgress() {
		t.Fatal("late downlink on the rejected path ended recovery")
	}
}

func TestRecoveryCompletionMatchesAttemptAndRegistrar(t *testing.T) {
	store := NewRegistrarPenaltyStore()
	now := time.Now()
	const registrar = "pcscf-a.example:5060"
	recordTestRegistrarFailure(store, registrar, now)
	attempt := registrarRecoveryAttempt{registrar: registrar, generation: store.recoveryGeneration()}
	// Even a duplicate rejection inside the cooldown invalidates the old attempt.
	recordTestRegistrarFailure(store, registrar, now)
	store.clearFailures(attempt)
	if !store.recoveryInProgress() || store.states(now)[registrar].consecutiveFailures != 1 {
		t.Fatal("stale attempt cleared a newer failure")
	}
	attempt.generation = store.recoveryGeneration()
	// A late report from another node must not invalidate this proven path.
	recordTestRegistrarFailure(store, "pcscf-b.example:5060", now)
	store.clearFailures(attempt)
	if store.recoveryInProgress() || store.states(now)[registrar].consecutiveFailures != 0 {
		t.Fatal("matching attempt failed to complete recovery")
	}
	if store.states(now)["pcscf-b.example:5060"].consecutiveFailures != 1 {
		t.Fatal("completion erased another registrar's failures")
	}
}
