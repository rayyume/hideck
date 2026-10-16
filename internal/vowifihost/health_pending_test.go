package vowifihost

import (
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
)

func TestHealthMeasuresIMSRegistrationWhileReceptionIsPending(t *testing.T) {
	store := newWiFiCallingHealthStore()
	start := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	store.Begin("sim", start)
	state := runtimehost.State{IMSReady: true, SMSRXPending: true, UpdatedAt: start}
	store.Observe("sim", state)
	got, _ := store.Snapshot("sim", start.Add(time.Hour))
	if got.State != "healthy" || !got.Measured || got.HealthySeconds != 3600 || got.InterruptedSeconds != 0 || got.Availability != 100 {
		t.Fatalf("pending reception suppressed IMS health = %+v", got)
	}
	state.UpdatedAt = start.Add(time.Hour)
	store.Observe("sim", state)
	state.SMSRXPending, state.SMSReady, state.SMSHealthReady = false, true, true
	state.UpdatedAt = start.Add(90 * time.Minute)
	store.Observe("sim", state)
	got, _ = store.Snapshot("sim", start.Add(2*time.Hour))
	if got.State != "healthy" || got.SessionStartedAt != start || got.HealthySeconds != 7200 || got.InterruptionCount != 0 || len(got.Events) != 1 {
		t.Fatalf("SMS verification reset IMS health = %+v", got)
	}
}

func TestHealthRecoversOnIMSRegistrationWithoutWaitingForReception(t *testing.T) {
	store := newWiFiCallingHealthStore()
	start := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	store.Observe("sim", runtimehost.State{IMSReady: true, SMSReady: true, UpdatedAt: start})
	store.Observe("sim", runtimehost.State{Phase: "interrupted", UpdatedAt: start.Add(time.Second)})
	store.Observe("sim", runtimehost.State{IMSReady: true, SMSRXPending: true, UpdatedAt: start.Add(2 * time.Second)})
	got, _ := store.Snapshot("sim", start.Add(time.Minute))
	if got.State != "healthy" || got.InterruptionCount != 1 || got.InterruptedSeconds != 1 || got.HealthySeconds != 59 {
		t.Fatalf("IMS recovery lost outage history or waited for SMS = %+v", got)
	}
}

func TestHealthRecordsFailureWhileReceptionIsUnverified(t *testing.T) {
	store := newWiFiCallingHealthStore()
	start := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	store.Observe("sim", runtimehost.State{IMSReady: true, SMSRXPending: true, UpdatedAt: start})
	store.Observe("sim", runtimehost.State{Phase: "error", LastError: "connection reset", UpdatedAt: start.Add(time.Second)})
	got, _ := store.Snapshot("sim", start.Add(time.Minute))
	if got.State != "unavailable" || got.LastReason != "connection reset" || got.InterruptionCount != 1 || got.InterruptedSeconds != 59 {
		t.Fatalf("unverified reception hid connection failure = %+v", got)
	}
	store.Observe("sim", runtimehost.State{Phase: "reconnecting", UpdatedAt: start.Add(time.Minute)})
	got, _ = store.Snapshot("sim", start.Add(2*time.Minute))
	if got.State != "recovering" || got.InterruptedSeconds != 119 {
		t.Fatalf("unregistered connection stopped counting downtime = %+v", got)
	}
	store.Observe("sim", runtimehost.State{IMSReady: true, SMSRXPending: true, UpdatedAt: start.Add(2*time.Minute)})
	got, _ = store.Snapshot("sim", start.Add(3*time.Minute))
	if got.State != "healthy" || got.InterruptionCount != 1 || got.InterruptedSeconds != 119 || got.HealthySeconds != 61 {
		t.Fatalf("IMS recovery hid failure history = %+v", got)
	}
}
