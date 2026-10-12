package device

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	modemhost "github.com/yibaiba/hideck/internal/modemvoice/host"
	"github.com/yibaiba/hideck/internal/volte"
)

func TestRemoveWorkerRetiresNativeSession(t *testing.T) {
	p, port := testModemVoicePort(t)
	id := port.worker.ID
	p.volteCtl = volte.NewControllerWithBackup(&nativeStartEffectHost{}, t.TempDir())
	if err := p.volteCtl.Enable(context.Background(), id); err == nil {
		t.Fatal("expected failed native startup")
	}
	if err := p.RemoveWorker(id); err != nil {
		t.Fatal(err)
	}
	if got := p.volteCtl.Status(id); got.Phase != volte.PhaseIdle {
		t.Fatalf("removed worker retained native state: %+v", got)
	}
	p.mu.Lock()
	p.workers[id] = &Worker{ID: id, Config: port.worker.Config}
	p.mu.Unlock()
	if got := p.volteCtl.Status(id); got.Phase != volte.PhaseIdle {
		t.Fatalf("replacement inherited native state: %+v", got)
	}
}

func TestAbandonDeviceReturnsCleanupFailureAndCanRetry(t *testing.T) {
	p, port := testModemVoicePort(t)
	id := port.worker.ID
	want := errors.New("cleanup failed")
	var fail atomic.Bool
	fail.Store(true)
	p.modemVoiceCtl = modemhost.New(modemhost.Options{
		Prepare: func(context.Context, string) (*modemhost.Resources, error) {
			return &modemhost.Resources{Close: func(context.Context) error {
				if fail.Load() {
					return want
				}
				return nil
			}}, want
		},
	})
	p.modemVoiceCtl.Enable(p.Context(), id)
	t.Cleanup(func() { fail.Store(false); _ = p.stopModemVoice(id) })
	deadline := time.Now().Add(2 * time.Second)
	for p.modemVoiceCtl.DeviceStatus(id)["phase"] != "failed" {
		if time.Now().After(deadline) {
			t.Fatal("modem preparation did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if err := p.AbandonDevice(id); !errors.Is(err, want) {
		t.Fatalf("cleanup error lost: %v", err)
	}
	if p.GetWorker(id) != port.worker {
		t.Fatal("failed cleanup removed the worker")
	}
	fail.Store(false)
	if err := p.AbandonDevice(id); err != nil {
		t.Fatal("cleanup retry", err)
	}
	if p.GetWorker(id) != nil {
		t.Fatal("retry retained worker")
	}
}
