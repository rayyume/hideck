package imscore

import (
	"io"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestCarrierPresetsAwaitPortSWithoutPriorReconnect(t *testing.T) {
	for _, preset := range []string{globePHCarrierPresetID, oneNZCarrierPresetID, ""} {
		for _, closeErr := range []error{io.EOF, syscall.ECONNRESET} {
			t.Run(preset+"/"+closeErr.Error(), func(t *testing.T) {
				service := newCarrierAwaitTestService(t, preset)
				closeIdleTestPortS(t, service, closeErr, 20*time.Minute)
				service.handleProtectedServerPushClosed()
				timer := stopTestPortSWatch(service)
				await := (preset == globePHCarrierPresetID && closeErr == io.EOF) ||
					(preset == oneNZCarrierPresetID && closeErr == syscall.ECONNRESET)
				if (timer == nil) != await {
					t.Fatalf("awaited peer reconnect = %t, want %t", timer == nil, await)
				}
				if got := service.SMSReadiness(); got.Ready != await {
					t.Fatalf("readiness = %+v, want ready %t", got, await)
				}
			})
		}
	}
}

func TestOneNZBusyPortSResetSchedulesRecovery(t *testing.T) {
	service := newCarrierAwaitTestService(t, oneNZCarrierPresetID)
	closeIdleTestPortS(t, service, syscall.ECONNRESET, time.Minute)
	service.handleProtectedServerPushClosed()
	if stopTestPortSWatch(service) == nil {
		t.Fatal("reset of a busy port-s awaited the peer instead of scheduling recovery")
	}
}

func TestCarrierAwaitBacksOffWhenRegisterDoesNotRestorePortS(t *testing.T) {
	service := newCarrierAwaitTestService(t, oneNZCarrierPresetID)
	closeIdleTestPortS(t, service, syscall.ECONNRESET, 20*time.Minute)
	service.handleProtectedServerPushClosed()
	service.completePortSRecovery(nil, true)
	if !service.portSRecoveryAwaitingFlow.Load() {
		t.Fatal("periodic REGISTER did not start port-s validation")
	}
	fireTestPortSWatch(t, service)
	if _, waiting := service.portSRecoveryDeadline(time.Now()); !waiting {
		t.Fatal("port-s still missing after REGISTER did not enter backoff")
	}
	stopTestPortSWatch(service)
}

func newCarrierAwaitTestService(t *testing.T, preset string) *Service {
	t.Helper()
	service := newProtectedKeepaliveTestService(t)
	service.cfg.CarrierPresetID = preset
	service.portSReconnectGrace = time.Hour
	registration, peer := net.Pipe()
	t.Cleanup(func() { registration.Close(); peer.Close() })
	service.registrationTCP = registration
	service.registrationTCPProtected = true
	return service
}

func closeIdleTestPortS(t *testing.T, service *Service, err error, idle time.Duration) {
	t.Helper()
	client, peer := net.Pipe()
	t.Cleanup(func() { client.Close(); peer.Close() })
	now := time.Now()
	service.recordPortSOpened(client, now.Add(-idle))
	if !service.trackProtectedConnection(client) {
		t.Fatal("track port-s")
	}
	service.recordPortSClosed(client, err, now)
	service.untrackProtectedConnection(client)
}

func stopTestPortSWatch(service *Service) *time.Timer {
	service.portSWatchMu.Lock()
	timer := service.portSWatchTimer
	service.portSWatchMu.Unlock()
	if timer != nil {
		timer.Stop()
	}
	return timer
}
