package imscore

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestRecoveryEarlyPortSRejectedRegisterRetainsRetryAfter(t *testing.T) {
	service := newRecoveryCompletionTestService(t)
	conn := newRecoveryCompletionPortS(t)
	service.transport.SetSendFn(func(request string) error {
		service.trackProtectedConnection(conn)
		service.transport.DeliverResponse(registerResponseForRequest(request, 503, map[string]string{"Retry-After": "3600"}))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := service.Start(ctx)
	var scheduled *registrarRecoveryRetryError
	if !errors.As(err, &scheduled) {
		t.Fatalf("REGISTER rejection lost recovery schedule: %v", err)
	}
	store := service.registrarPenalties
	entry := store.states(time.Now())["pcscf-a.example:5060"]
	if !store.recoveryInProgress() || entry.consecutiveFailures != 2 || entry.retryNotBefore.Before(time.Now().Add(59*time.Minute)) {
		t.Fatalf("early port-s cleared failure history or Retry-After: %+v", entry)
	}
	if alternate := store.states(time.Now())["pcscf-b.example:5060"]; alternate.consecutiveFailures != 0 || !alternate.retryNotBefore.IsZero() {
		t.Fatalf("unattempted alternate inherited failed node's penalty: %+v", alternate)
	}
	if scheduled.RetryAt().After(time.Now()) {
		t.Fatal("available alternate must not wait for failed node's Retry-After")
	}
	var rejected *registerResponseError
	if !errors.As(err, &rejected) || rejected.statusCode != 503 {
		t.Fatalf("recovery lost original REGISTER response: %v", err)
	}
}

func TestRecoveryEarlyPortSSingleCandidateRetainsBackoff(t *testing.T) {
	for _, test := range []struct {
		name        string
		headers     map[string]string
		minimumWait time.Duration
	}{
		{name: "exponential backoff", minimumWait: time.Minute},
		{name: "Retry-After", headers: map[string]string{"Retry-After": "3600"}, minimumWait: time.Hour},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := newRecoveryCompletionTestService(t)
			service.cfg.Registrar = "pcscf-a.example:5060"
			service.registrarCandidates = []string{service.cfg.Registrar}
			conn := newRecoveryCompletionPortS(t)
			service.transport.SetSendFn(func(request string) error {
				service.trackProtectedConnection(conn)
				service.transport.DeliverResponse(registerResponseForRequest(request, 503, test.headers))
				return nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			before := time.Now()
			err := service.Start(ctx)
			var scheduled *registrarRecoveryRetryError
			if !errors.As(err, &scheduled) || scheduled.RetryAt().Before(before.Add(test.minimumWait)) {
				t.Fatalf("early port-s lost recovery deadline: %v", err)
			}
			entry := service.registrarPenalties.states(time.Now())[service.cfg.Registrar]
			if entry.consecutiveFailures != 2 || !entry.retryNotBefore.Equal(scheduled.RetryAt()) {
				t.Fatalf("failure history or deadline changed: %+v", entry)
			}
		})
	}
}

func TestRecoveryCompletionRequiresRegisterAndDownlinkInEitherOrder(t *testing.T) {
	for _, phase := range []string{"port-s before 200", "request before 200", "port-s after 200", "no downlink", "early port-s closed", "historical request"} {
		t.Run(phase, func(t *testing.T) {
			service := newRecoveryCompletionTestService(t)
			conn := newRecoveryCompletionPortS(t)
			if phase == "historical request" {
				service.inboundSIPHandledRequest.Add(1)
			}
			service.transport.SetSendFn(func(request string) error {
				if strings.HasPrefix(request, "REGISTER ") {
					observeRecoveryTestDownlink(service, conn, phase)
				}
				service.transport.DeliverResponse(registerResponseForRequest(request, 200, nil))
				return nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := service.Start(ctx); err != nil {
				t.Fatal(err)
			}
			if phase == "port-s after 200" {
				if !service.registrarPenalties.recoveryInProgress() {
					t.Fatal("REGISTER alone ended recovery")
				}
				service.trackProtectedConnection(conn)
			}
			wantPending := phase == "no downlink" || phase == "early port-s closed" || phase == "historical request"
			if got := service.registrarPenalties.recoveryInProgress(); got != wantPending {
				t.Fatalf("pending=%t want %t", got, wantPending)
			}
			entry := service.registrarPenalties.states(time.Now())["pcscf-a.example:5060"]
			if !wantPending && entry.consecutiveFailures != 0 {
				t.Fatal("successful recovery kept failures")
			}
			if wantPending && entry.consecutiveFailures != 1 {
				t.Fatal("unverified recovery lost failures")
			}
			if entry.deprioritizedUntil.IsZero() {
				t.Fatal("completion erased node preference")
			}
		})
	}
}

func observeRecoveryTestDownlink(service *Service, conn net.Conn, phase string) {
	switch phase {
	case "port-s before 200":
		service.trackProtectedConnection(conn)
	case "request before 200":
		service.inboundSIPHandledRequest.Add(1)
		service.recordCurrentDownlinkRequest(nil, service.captureDownlinkCheckpoint())
	case "early port-s closed":
		service.trackProtectedConnection(conn)
		service.untrackProtectedConnection(conn)
	}
}

func newRecoveryCompletionTestService(t *testing.T) *Service {
	t.Helper()
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	recordTestRegistrarFailure(service.registrarPenalties, "pcscf-a.example:5060", time.Now().Add(-2*time.Minute))
	return service
}

func newRecoveryCompletionPortS(t *testing.T) net.Conn {
	t.Helper()
	conn, peer := net.Pipe()
	t.Cleanup(func() { _ = conn.Close(); _ = peer.Close() })
	return conn
}
