package imscore

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestPortSCloseClassification(t *testing.T) {
	if got := classifyPortSClose(nil); got != portSCloseOther {
		t.Fatalf("nil close kind = %q, want other", got)
	}
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{"EOF", io.EOF, portSCloseEOF},
		{"RST", syscall.ECONNRESET, portSClosePeerReset},
		{"transport timeout", syscall.ETIMEDOUT, "transport_timeout"},
		{"read deadline", os.ErrDeadlineExceeded, "read_deadline"},
		{"context deadline", context.DeadlineExceeded, "read_deadline"},
		{"local close", net.ErrClosed, portSCloseLocal},
		{"cancelled", context.Canceled, portSCloseLocal},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := &net.OpError{Op: "read", Net: "tcp", Err: fmt.Errorf("wrapped: %w", test.err)}
			if got := classifyPortSClose(err); got != test.want {
				t.Fatalf("close kind = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPriorPeerReconnectOnlySuppressesCleanEOFFlowRecovery(t *testing.T) {
	for _, preset := range []string{vodafoneUKCarrierPresetID, "2degrees_nz", "ctexcel", ""} {
		for _, closeErr := range []error{io.EOF, syscall.ETIMEDOUT, syscall.ECONNRESET, net.ErrClosed} {
			t.Run(preset+"/"+closeErr.Error(), func(t *testing.T) {
				service := newProtectedKeepaliveTestService(t)
				service.cfg.CarrierPresetID = preset
				// Exercise the close-kind gate independently of timer delivery.
				service.portSReconnectGrace = time.Hour
				service.portSOnDemandObserved.Store(true)
				closeTestPortS(t, service, closeErr)
				service.handleProtectedServerPushClosed()
				service.portSWatchMu.Lock()
				timer, generation := service.portSWatchTimer, service.portSWatchGeneration
				service.portSWatchMu.Unlock()
				wantRecovery := closeErr != io.EOF && closeErr != net.ErrClosed
				if (timer != nil) != wantRecovery {
					t.Fatalf("recovery scheduled = %t, want %t", timer != nil, wantRecovery)
				}
				if !wantRecovery {
					return
				}
				timer.Stop()
				service.portSReconnectWatchFired(generation, service.currentPortSRecoveryRegistrar())
				if !service.reRegisterPending.Load() || !service.portSRecoveryPending.Load() {
					t.Fatal("abnormal close did not request REGISTER recovery")
				}
				if service.RegState() != regRegistered || service.pcscfRecoveryPending.Load() {
					t.Fatal("push failure discarded the registration or forced proxy failover")
				}
			})
		}
	}
}

func TestHistoricalReconnectDoesNotHideAbnormalSMSHealth(t *testing.T) {
	for _, closeErr := range []error{io.EOF, syscall.ETIMEDOUT, syscall.ECONNRESET, os.ErrDeadlineExceeded} {
		t.Run(closeErr.Error(), func(t *testing.T) {
			service := newProtectedKeepaliveTestService(t)
			registration, peer := net.Pipe()
			t.Cleanup(func() { registration.Close(); peer.Close() })
			service.registrationTCP = registration
			service.registrationTCPProtected = true
			service.portSOnDemandObserved.Store(true)
			closeTestPortS(t, service, closeErr)
			got := service.SMSReadiness()
			want := closeErr == io.EOF
			if got.Ready != want || got.HealthReady != want {
				t.Fatalf("readiness = %+v, want ready/healthy %t", got, want)
			}
		})
	}
}

func closeTestPortS(t *testing.T, service *Service, err error) {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { client.Close(); peer.Close() })
	service.recordPortSOpened(client, time.Now())
	if !service.trackProtectedConnection(client) {
		t.Fatal("track port-s")
	}
	service.recordPortSClosed(client, err, time.Now())
	service.untrackProtectedConnection(client)
}

func TestPeerReconnectThenTimeoutStartsRecoveryWithoutPeriodicRegister(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.cfg.CarrierPresetID = vodafoneUKCarrierPresetID
	service.registrationRefreshAt = time.Now().Add(17 * time.Minute)
	closeTestPortS(t, service, syscall.ECONNRESET)
	service.handleProtectedServerPushClosed()
	// Reproduce the peer's quick reconnect, followed hours later by timeout.
	closeTestPortS(t, service, syscall.ETIMEDOUT)
	if !service.portSOnDemandObserved.Load() {
		t.Fatal("test did not establish historical peer reconnect knowledge")
	}
	if got := service.portSReconnectWait(); got != 30*time.Second {
		t.Fatalf("timeout observation window = %s, want 30s (not the RST policy)", got)
	}
	service.handleProtectedServerPushClosed()
	fireTestPortSWatch(t, service)
	if !service.portSRecoveryPending.Load() || !service.reRegisterPending.Load() {
		t.Fatal("timeout recovery waited for the periodic REGISTER")
	}
	service.completePortSRecovery(nil, true)
	if !service.portSRecoveryAwaitingFlow.Load() || service.canAwaitOnDemandPortS() {
		t.Fatal("REGISTER success hid the still-missing downlink")
	}
	fireTestPortSWatch(t, service)
	if _, waiting := service.portSRecoveryDeadline(time.Now()); !waiting {
		t.Fatal("missing downlink after REGISTER did not enter backoff")
	}
	if service.pcscfRecoveryPending.Load() || service.RegState() != regRegistered {
		t.Fatal("plain timeout activated Vodafone reset failover or discarded registration")
	}
}

func TestAbnormalPortSCloseRespectsRetryAfterWithHistoricalReconnect(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.portSOnDemandObserved.Store(true)
	closeTestPortS(t, service, syscall.ETIMEDOUT)
	service.handleProtectedServerPushClosed()
	waitForPortSCondition(t, func() bool { return service.reRegisterPending.Load() })
	service.reRegisterPending.Store(false)
	service.completePortSRecovery(registerResponseErrorWithRetryAfter(t, "600"), true)
	retryAt, waiting := service.portSRecoveryDeadline(time.Now())
	if !waiting || time.Until(retryAt) < 9*time.Minute {
		t.Fatal("recovery failed to honor Retry-After")
	}
	service.handleProtectedServerPushClosed()
	fireTestPortSWatch(t, service)
	if service.reRegisterPending.Load() || service.portSRecoveryPending.Load() {
		t.Fatal("repeated close bypassed backoff")
	}
	if next, _ := service.portSRecoveryDeadline(time.Now()); !next.Equal(retryAt) {
		t.Fatal("repeated close changed the established retry deadline")
	}
}

func TestPeerReconnectWithinTimeoutGraceCancelsRecovery(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.cfg.CarrierPresetID = vodafoneUKCarrierPresetID
	service.portSOnDemandObserved.Store(true)
	closeTestPortS(t, service, syscall.ETIMEDOUT)
	service.handleProtectedServerPushClosed()
	service.portSWatchMu.Lock()
	generation := service.portSWatchGeneration
	service.portSWatchMu.Unlock()
	client, peer := net.Pipe()
	t.Cleanup(func() { client.Close(); peer.Close() })
	service.recordPortSOpened(client, time.Now())
	if !service.trackProtectedConnection(client) {
		t.Fatal("track reopened port-s")
	}
	service.portSReconnectWatchFired(generation, service.currentPortSRecoveryRegistrar())
	if service.reRegisterPending.Load() || service.portSRecoveryPending.Load() {
		t.Fatal("stale timeout watchdog refreshed a recovered connection")
	}
}

func fireTestPortSWatch(t *testing.T, service *Service) {
	t.Helper()
	service.portSWatchMu.Lock()
	timer, generation := service.portSWatchTimer, service.portSWatchGeneration
	service.portSWatchMu.Unlock()
	if timer == nil {
		t.Fatal("missing port-s recovery watchdog")
	}
	timer.Stop()
	service.portSReconnectWatchFired(generation, service.currentPortSRecoveryRegistrar())
}

func TestGenericPortSFailureDoesNotUseVodafoneGrace(t *testing.T) {
	for _, preset := range []string{"2degrees_nz", "ctexcel", ""} {
		t.Run(preset, func(t *testing.T) {
			service := newProtectedKeepaliveTestService(t)
			service.cfg.CarrierPresetID = preset
			service.portSOnDemandObserved.Store(true)
			closeTestPortS(t, service, syscall.ETIMEDOUT)
			service.handleProtectedServerPushClosed()
			waitForPortSCondition(t, func() bool { return service.reRegisterPending.Load() })
			if !service.portSRecoveryPending.Load() {
				t.Fatal("generic flow failure did not enter recovery")
			}
			service.completePortSRecovery(nil, true)
			if !service.portSRecoveryAwaitingFlow.Load() {
				t.Fatal("REGISTER did not await downlink validation")
			}
			if _, waiting := service.portSRecoveryDeadline(time.Now()); waiting {
				t.Fatal("zero initial grace incorrectly failed downlink validation immediately")
			}
			fireTestPortSWatch(t, service)
			if _, waiting := service.portSRecoveryDeadline(time.Now()); !waiting {
				t.Fatal("unvalidated downlink did not back off")
			}
		})
	}
}
