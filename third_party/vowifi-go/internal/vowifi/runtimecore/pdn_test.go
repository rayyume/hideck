package runtimecore

import (
	"context"
	"errors"
	"testing"

	"github.com/iniwex5/vowifi-go/engine/swu"
	"github.com/iniwex5/vowifi-go/internal/vowifi/policy"
)

type recordingPDNStarter struct {
	started []string
	stopped []string
	fail    string
}

func (r *recordingPDNStarter) StartSlot(_ context.Context, _, slot string, cfg *swu.Config) (*swu.Session, error) {
	r.started = append(r.started, slot+":"+cfg.APN)
	if slot == r.fail {
		return nil, errors.New("xcap tunnel down")
	}
	return swu.NewSession(cfg), nil
}

func (r *recordingPDNStarter) StopSlot(_, slot string) error {
	r.stopped = append(r.stopped, slot)
	return nil
}

func (r *recordingPDNStarter) GetSlot(string, string) (*swu.Session, bool) {
	return nil, false
}

func TestStartAdditionalPDNsNoopsForSingleAPN(t *testing.T) {
	starter := &recordingPDNStarter{}
	cfg := &swu.Config{APN: "ims", EPDGAddr: "epdg.example"}
	if err := StartAdditionalPDNs(context.Background(), starter, "dev-1", cfg, policy.EffectiveCarrierConfig{APN: "ims"}); err != nil {
		t.Fatalf("StartAdditionalPDNs: %v", err)
	}
	if len(starter.started) != 0 {
		t.Fatalf("started = %v", starter.started)
	}
}

func TestStartAdditionalPDNsReusesEPDGAndIsolatesFailure(t *testing.T) {
	starter := &recordingPDNStarter{fail: policy.XCAPSessionSlot}
	base := &swu.Config{APN: "ims", EPDGAddr: "epdg.example"}
	err := StartAdditionalPDNs(context.Background(), starter, "dev-1", base, policy.EffectiveCarrierConfig{
		APN: "ims", XCAPAPN: "xcap",
	})
	if err == nil {
		t.Fatal("expected XCAP start error")
	}
	if len(starter.started) != 1 || starter.started[0] != "xcap:xcap" {
		t.Fatalf("started = %v", starter.started)
	}
	if len(starter.stopped) != 1 || starter.stopped[0] != policy.XCAPSessionSlot {
		t.Fatalf("stopped = %v", starter.stopped)
	}
	if base.APN != "ims" {
		t.Fatalf("base APN mutated = %q", base.APN)
	}
}
