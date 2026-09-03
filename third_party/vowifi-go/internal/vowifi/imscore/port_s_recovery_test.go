package imscore

import (
	"context"
	"testing"
	"time"
)

func TestRFC5626RecoveryUpperBound(t *testing.T) {
	tests := []struct {
		name           string
		failures       uint32
		allFlowsFailed bool
		want           time.Duration
	}{
		{name: "all flows first failure", failures: 1, allFlowsFailed: true, want: time.Minute},
		{name: "live flow first failure", failures: 1, want: 3 * time.Minute},
		{name: "live flow second failure", failures: 2, want: 6 * time.Minute},
		{name: "maximum", failures: 8, want: 30 * time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := rfc5626RecoveryUpperBound(test.failures, test.allFlowsFailed); got != test.want {
				t.Fatalf("upper bound = %s, want %s", got, test.want)
			}
		})
	}
}

func TestPortSRecoveryRetryAfterCanOnlyExtendBackoff(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	now := time.Unix(1_700_000_000, 0)

	short := service.recordPortSRecoveryFailure(registerResponseErrorWithRetryAfter(t, "10"), now)
	if short.delay != 90*time.Second || short.retryAfter != 10*time.Second || !short.retryAfterSet {
		t.Fatalf("short Retry-After schedule = %+v", short)
	}
	service.resetPortSRecoveryBackoff()

	long := service.recordPortSRecoveryFailure(registerResponseErrorWithRetryAfter(t, "300 (maintenance)"), now)
	if long.delay != 5*time.Minute || long.retryAt != now.Add(5*time.Minute) {
		t.Fatalf("long Retry-After schedule = %+v", long)
	}
}

func TestPortSRecoveryBackoffCoalescesRepeatedClosures(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.portSReconnectWaiting.Store(true)
	service.recordPortSRecoveryFailure(context.DeadlineExceeded, time.Now())

	service.handleProtectedServerPushClosed()
	service.handleProtectedServerPushClosed()
	time.Sleep(20 * time.Millisecond)
	if service.reRegisterPending.Load() || service.portSRecoveryPending.Load() {
		t.Fatal("repeated port-s closure bypassed recovery backoff")
	}
}

func TestPortSRecoveryRetriesWhenBackoffExpires(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.portSReconnectWaiting.Store(true)
	registrar := service.currentPortSRecoveryRegistrar()
	service.portSWatchMu.Lock()
	service.portSBackoff = portSRecoveryBackoff{
		registrar: registrar, failures: 1,
		retryAt: time.Now().Add(10 * time.Millisecond),
	}
	retryAt := service.portSBackoff.retryAt
	service.portSWatchMu.Unlock()

	service.schedulePortSReconnectWatchAt(retryAt)
	waitForPortSCondition(t, func() bool { return service.reRegisterPending.Load() })
	if !service.portSRecoveryPending.Load() {
		t.Fatal("expired backoff REGISTER was not marked as port-s recovery")
	}
}

func TestSuccessfulRegisterKeepsWatchingMissingPortS(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.portSReconnectGrace = time.Millisecond
	service.portSReconnectWaiting.Store(true)

	service.completePortSRecovery(nil, true)
	waitForPortSCondition(t, func() bool { return service.reRegisterPending.Load() })
	if !service.portSRecoveryPending.Load() {
		t.Fatal("successful REGISTER stopped monitoring the missing port-s flow")
	}
}

func TestCanceledPortSWatchCannotTriggerRecovery(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.schedulePortSReconnectWatchAt(time.Now().Add(time.Hour))
	service.portSWatchMu.Lock()
	generation := service.portSWatchGeneration
	service.portSWatchMu.Unlock()

	service.cancelPortSReconnectWatch()
	service.portSReconnectWatchFired(generation, service.currentPortSRecoveryRegistrar())
	if service.reRegisterPending.Load() || service.portSRecoveryPending.Load() {
		t.Fatal("canceled port-s timer triggered a stale recovery")
	}
}

func TestPortSWatchCannotCrossPCSCFChange(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.mu.Unlock()
	service.schedulePortSReconnectWatchAt(time.Now().Add(time.Hour))
	service.portSWatchMu.Lock()
	generation := service.portSWatchGeneration
	service.portSWatchMu.Unlock()

	service.mu.Lock()
	service.registrar = "pcscf-b.example:5060"
	service.mu.Unlock()
	service.portSReconnectWatchFired(generation, "pcscf-a.example:5060")
	if service.reRegisterPending.Load() || service.portSRecoveryPending.Load() {
		t.Fatal("old P-CSCF timer triggered recovery on the new P-CSCF")
	}
}

func TestPortSTrafficClearsRecoveryBackoff(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.recordPortSRecoveryFailure(context.DeadlineExceeded, time.Now())
	service.handlePortSTraffic()
	if _, waiting := service.portSRecoveryDeadline(time.Now()); waiting {
		t.Fatal("working port-s traffic kept recovery backoff")
	}
}

func TestPortSRecoveryBackoffIsScopedToRegistrar(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.mu.Unlock()
	service.recordPortSRecoveryFailure(context.DeadlineExceeded, time.Now())

	service.mu.Lock()
	service.registrar = "pcscf-b.example:5060"
	service.mu.Unlock()
	if _, waiting := service.portSRecoveryDeadline(time.Now()); waiting {
		t.Fatal("new P-CSCF inherited the previous P-CSCF backoff")
	}
}

func registerResponseErrorWithRetryAfter(t *testing.T, value string) error {
	t.Helper()
	return registrationResponseError(&sipResponse{
		StatusCode: 503,
		Reason:     "Service Unavailable",
		Headers:    map[string]string{"Retry-After": value},
	}, true)
}
