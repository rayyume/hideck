package vowifihost

import (
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
)

func TestWiFiCallingHealthMeasuresRuntimeInterruptions(t *testing.T) {
	store := newWiFiCallingHealthStore()
	started := time.Date(2026, 9, 4, 8, 0, 0, 0, time.UTC)
	observeHealth(store, started, true, "ims_ready", "")
	observeHealth(store, started.Add(10*time.Second), false, "interrupted", "IMS transport lost")
	observeHealth(store, started.Add(20*time.Second), false, "retrying", "retrying")
	observeHealth(store, started.Add(40*time.Second), true, "ims_ready", "")

	snapshot, ok := store.Snapshot("wwan0", started.Add(50*time.Second))
	if !ok || !snapshot.Measured || snapshot.State != "healthy" {
		t.Fatalf("snapshot = %+v, ok=%t", snapshot, ok)
	}
	if snapshot.SessionSeconds != 50 || snapshot.HealthySeconds != 20 || snapshot.InterruptedSeconds != 30 {
		t.Fatalf("durations = total:%d healthy:%d interrupted:%d",
			snapshot.SessionSeconds, snapshot.HealthySeconds, snapshot.InterruptedSeconds)
	}
	if snapshot.InterruptionCount != 1 || snapshot.LongestInterruptionSeconds != 30 || snapshot.StableSeconds != 10 {
		t.Fatalf("interruption metrics = %+v", snapshot)
	}
	if snapshot.Availability != 40 {
		t.Fatalf("availability = %v, want 40", snapshot.Availability)
	}
	if len(snapshot.Events) != 3 || snapshot.Events[1].Kind != "interrupted" || snapshot.Events[2].Kind != "recovered" {
		t.Fatalf("events = %+v", snapshot.Events)
	}
}

func TestWiFiCallingHealthRecordsIntentionalStopWithoutDowntime(t *testing.T) {
	store := newWiFiCallingHealthStore()
	started := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	observeHealth(store, started, true, "ims_ready", "")
	store.End("wwan0", "disable", started.Add(time.Minute))

	stopped, ok := store.Snapshot("wwan0", started.Add(time.Hour))
	if !ok || stopped.Active || stopped.State != "stopped" {
		t.Fatalf("stopped snapshot = %+v, ok=%t", stopped, ok)
	}
	if stopped.SessionSeconds != 60 || stopped.HealthySeconds != 60 || stopped.InterruptedSeconds != 0 {
		t.Fatalf("intentional stop counted as downtime: %+v", stopped)
	}
	if got := stopped.Events[len(stopped.Events)-1]; got.Kind != "stopped" || got.Reason != "disable" {
		t.Fatalf("stop event = %+v", got)
	}

	observeHealth(store, started.Add(2*time.Hour), false, "connecting", "starting")
	stillStopped, _ := store.Snapshot("wwan0", started.Add(2*time.Hour))
	if stillStopped.State != "stopped" || stillStopped.Active {
		t.Fatalf("teardown observation reopened stopped session: %+v", stillStopped)
	}

	store.Begin("wwan0", started.Add(2*time.Hour))
	observeHealth(store, started.Add(2*time.Hour), false, "connecting", "starting")
	checking, _ := store.Snapshot("wwan0", started.Add(2*time.Hour))
	if checking.Measured || len(checking.Events) == 0 || checking.Events[len(checking.Events)-1].Kind != "stopped" {
		t.Fatalf("new session did not retain stop history: %+v", checking)
	}
}

func TestWiFiCallingHealthMeasuresPortSOutageFromSMSReadiness(t *testing.T) {
	store := newWiFiCallingHealthStore()
	started := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)
	observeHealth(store, started, true, "ims_ready", "")
	store.Observe("wwan0", runtimehost.State{
		DeviceID: "wwan0", IMSReady: true, SMSReady: false, Phase: "sms_ready",
		SMSReadyReason: "IMS SMS receiver is not ready", UpdatedAt: started.Add(time.Minute),
	})
	store.Observe("wwan0", runtimehost.State{
		DeviceID: "wwan0", IMSReady: true, SMSReady: true, Phase: "sms_ready",
		SMSReadyReason: "IMS SMS receiver ready", UpdatedAt: started.Add(90 * time.Second),
	})

	snapshot, _ := store.Snapshot("wwan0", started.Add(2*time.Minute))
	if snapshot.State != "healthy" || snapshot.InterruptionCount != 1 {
		t.Fatalf("port-s outage was not recorded: %+v", snapshot)
	}
	if snapshot.InterruptedSeconds != 30 || snapshot.Availability != 75 {
		t.Fatalf("port-s outage duration = %+v", snapshot)
	}
	if got := snapshot.Events[1]; got.Kind != "interrupted" || got.Reason != "IMS SMS receiver is not ready" {
		t.Fatalf("port-s interruption event = %+v", got)
	}
	if got := snapshot.Events[2]; got.Kind != "recovered" || got.At != started.Add(90*time.Second) {
		t.Fatalf("port-s recovery event = %+v", got)
	}
}

func TestWiFiCallingHealthStartsWhenSMSReceiverIsReady(t *testing.T) {
	store := newWiFiCallingHealthStore()
	started := time.Date(2026, 9, 4, 10, 30, 0, 0, time.UTC)
	store.Begin("wwan0", started)
	store.Observe("wwan0", runtimehost.State{
		DeviceID: "wwan0", IMSReady: true, Phase: "ims_ready",
		SMSReadyReason: "IMS SMS receiver is not ready", UpdatedAt: started.Add(time.Second),
	})

	checking, _ := store.Snapshot("wwan0", started.Add(2*time.Second))
	if checking.Measured || checking.State != "checking" {
		t.Fatalf("IMS-only readiness started health measurement: %+v", checking)
	}

	store.Observe("wwan0", runtimehost.State{
		DeviceID: "wwan0", IMSReady: true, SMSReady: true, Phase: "sms_ready",
		SMSReadyReason: "IMS SMS receiver ready", UpdatedAt: started.Add(3 * time.Second),
	})
	snapshot, _ := store.Snapshot("wwan0", started.Add(4*time.Second))
	if !snapshot.Measured || snapshot.SessionStartedAt != started.Add(3*time.Second) {
		t.Fatalf("SMS readiness did not start health measurement: %+v", snapshot)
	}
	if got := snapshot.Events[0]; got.Kind != "started" || got.Reason != "IMS SMS receiver ready" {
		t.Fatalf("start event = %+v", got)
	}
}

func TestWiFiCallingHealthRecordsFailureBeforeFirstRegistration(t *testing.T) {
	store := newWiFiCallingHealthStore()
	started := time.Date(2026, 9, 4, 11, 0, 0, 0, time.UTC)
	store.Begin("wwan0", started)
	store.FailStart("wwan0", "prepare SIM identity: unavailable", started.Add(time.Second))

	snapshot, ok := store.Snapshot("wwan0", started.Add(time.Minute))
	if !ok || snapshot.Active || snapshot.Measured || snapshot.State != "unavailable" {
		t.Fatalf("failed startup snapshot = %+v, ok=%t", snapshot, ok)
	}
	if snapshot.LastReason != "prepare SIM identity: unavailable" {
		t.Fatalf("failure reason = %q", snapshot.LastReason)
	}
	if len(snapshot.Events) != 1 || snapshot.Events[0].Kind != "failed" {
		t.Fatalf("failure events = %+v", snapshot.Events)
	}
}

func observeHealth(store *wifiCallingHealthStore, at time.Time, imsReady bool, phase, reason string) {
	store.Observe("wwan0", runtimehost.State{
		DeviceID: "wwan0", IMSReady: imsReady, SMSReady: imsReady,
		Phase: phase, LastReason: reason, UpdatedAt: at,
	})
}
