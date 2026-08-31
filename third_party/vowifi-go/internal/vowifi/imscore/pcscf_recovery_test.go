package imscore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imsendpoint"
)

func TestDecidePCSCF503RecoveryFollowsTimerB(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	timerB := 32 * time.Second
	tests := []struct {
		name       string
		retryAfter string
		want       bool
		wantUntil  time.Time
	}{
		{name: "absent", want: true},
		{name: "short", retryAfter: "10", want: false},
		{name: "equal", retryAfter: "32", want: false},
		{name: "long", retryAfter: "60 (maintenance)", want: true, wantUntil: now.Add(time.Minute)},
		// Long Retry-After still failovers because waiting past Timer B would
		// strand the originating INVITE. IR.92 2.2.1 only mandates this when
		// Retry-After is absent; this extra branch is documented tolerance.
		{name: "invalid", retryAfter: "later", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &sipResponse{StatusCode: 503}
			if test.retryAfter != "" {
				response.Headers = map[string]string{"Retry-After": test.retryAfter}
			}
			decision := decidePCSCF503Recovery(response, timerB, now)
			if decision.recover != test.want || !decision.unavailableUntil.Equal(test.wantUntil) {
				t.Fatalf("decision = %+v, want recover=%t until=%s", decision, test.want, test.wantUntil)
			}
		})
	}
}

func TestPCSCFUnavailableCandidatesAreSkipped(t *testing.T) {
	service := newRegisteredClientInviteService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{
		"pcscf-a.example:5060", "pcscf-b.example:5060", "pcscf-c.example:5060",
	}
	service.registrarUnavailable = map[string]time.Time{
		"pcscf-b.example:5060": time.Now().Add(time.Minute),
	}
	service.mu.Unlock()

	next, current := service.markRegistrarUnavailableAndAdvance("pcscf-a.example:5060", time.Time{})
	if !current || next != "pcscf-c.example:5060" {
		t.Fatalf("advance = %q current=%t", next, current)
	}
	if service.advanceRegistrarForNextRetry("network") {
		t.Fatal("retry selected a P-CSCF that is still unavailable")
	}
}

func TestInitialInvite503WithoutAlternateInvalidatesRegistration(t *testing.T) {
	service := newRegisteredClientInviteService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{"pcscf-a.example:5060"}
	service.signalingReady = true
	service.mu.Unlock()
	outbound := recordTransactionWrites(service.transport)
	request := mustClientInviteRequest(t, "invite-503-recovery")
	result := make(chan legacyClientInviteOutcome, 1)
	go func() {
		value, err := service.StartClientInvite(t.Context(), service.DeviceID(), imsendpoint.ClientInviteOptions{
			Request: request, Contact: request.Contact(),
		})
		result <- legacyClientInviteOutcome{result: value, err: err}
	}()

	written := waitTransactionWrite(t, outbound)
	service.transport.DeliverResponse(mustTransactionResponse(t, written, 503))
	outcome := <-result
	if outcome.result == nil || outcome.result.Response == nil || outcome.result.Response.StatusCode != 503 {
		t.Fatalf("result = %+v", outcome.result)
	}
	if outcome.err == nil || !strings.Contains(outcome.err.Error(), "503") {
		t.Fatalf("error = %v", outcome.err)
	}
	assertPCSCFRecoveryRequested(t, service)
}

func TestCanceledInitialInviteRetains503ForPCSCFRecovery(t *testing.T) {
	service := newRegisteredClientInviteService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{"pcscf-a.example:5060"}
	service.signalingReady = true
	service.mu.Unlock()
	outbound := recordTransactionWrites(service.transport)
	request := mustClientInviteRequest(t, "canceled-invite-503-recovery")
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan legacyClientInviteOutcome, 1)
	go func() {
		value, err := service.StartClientInvite(ctx, service.DeviceID(), imsendpoint.ClientInviteOptions{
			Request: request, Contact: request.Contact(),
		})
		result <- legacyClientInviteOutcome{result: value, err: err}
	}()

	writtenInvite := waitTransactionWrite(t, outbound)
	service.transport.DeliverResponse(mustTransactionResponse(t, writtenInvite, 100))
	cancel()
	writtenCancel := waitForTransactionMethod(t, outbound, "CANCEL")
	service.transport.DeliverResponse(mustTransactionResponse(t, writtenInvite, 503))
	service.transport.DeliverResponse(mustTransactionResponse(t, writtenCancel, 481))
	outcome := <-result
	if !errors.Is(outcome.err, context.Canceled) || outcome.result == nil ||
		outcome.result.Response == nil || outcome.result.Response.StatusCode != 503 {
		t.Fatalf("result=%+v error=%v", outcome.result, outcome.err)
	}
	assertPCSCFRecoveryRequested(t, service)
}

func assertPCSCFRecoveryRequested(t *testing.T, service *Service) {
	t.Helper()
	select {
	case err := <-service.RegistrationErrors():
		if err == nil || !strings.Contains(err.Error(), "no alternate is available") {
			t.Fatalf("runtime recovery error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("503 did not request P-CSCF rediscovery")
	}
	if service.IsRegistered() {
		t.Fatal("503 left the failed P-CSCF registration active")
	}
}
