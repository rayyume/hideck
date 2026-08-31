package runtimecore

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/engine/swu"
	"github.com/iniwex5/vowifi-go/internal/vowifi/netstack"
	"github.com/iniwex5/vowifi-go/internal/vowifi/policy"
	"github.com/iniwex5/vowifi-go/internal/vowifi/profile"
)

type recordingPDNStarter struct {
	started  []string
	stopped  []string
	waited   []string
	fail     string
	failWait string
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

func (r *recordingPDNStarter) WaitSlot(_ context.Context, _, slot string, _ time.Duration) (swu.SessionSnapshot, error) {
	r.waited = append(r.waited, slot)
	if slot == r.failWait {
		return swu.SessionSnapshot{}, errors.New("xcap wait failed")
	}
	return swu.SessionSnapshot{Established: true, IPv4: net.IPv4(10, 0, 0, 2)}, nil
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

func TestStartAdditionalPDNsWaitsThenStopsFailedSlot(t *testing.T) {
	starter := &recordingPDNStarter{failWait: policy.XCAPSessionSlot}
	base := &swu.Config{APN: "ims", EPDGAddr: "epdg.example"}
	err := StartAdditionalPDNs(context.Background(), starter, "dev-1", base, policy.EffectiveCarrierConfig{
		APN: "ims", XCAPAPN: "xcap",
	})
	if err == nil {
		t.Fatal("expected wait error")
	}
	if len(starter.waited) != 1 || starter.waited[0] != policy.XCAPSessionSlot {
		t.Fatalf("waited = %v", starter.waited)
	}
	if len(starter.stopped) != 1 || starter.stopped[0] != policy.XCAPSessionSlot {
		t.Fatalf("stopped = %v", starter.stopped)
	}
}

func TestXCAPDialContextDoesNotFallBackWhenXCAPRequired(t *testing.T) {
	result := &SessionResult{XCAPRequired: true, IMSNetwork: &netstack.Network{}}
	if result.XCAPDialContext() != nil {
		t.Fatal("distinct XCAP APN must not dial the IMS PDN")
	}
}

func TestXCAPDialContextUsesIMSNetworkWhenNoExtraAPN(t *testing.T) {
	result := &SessionResult{}
	if result.XCAPDialContext() != nil {
		t.Fatal("missing IMS network should yield no dialer")
	}
}

func TestAttachAdditionalPDNsNoopsWithoutManager(t *testing.T) {
	result := &SessionResult{}
	attachAdditionalPDNs(context.Background(), SessionConfig{
		Prepared: profile.PreparedSession{
			CarrierPlan: policy.CarrierPlan{EPDG: policy.EPDGPlan{APN: "ims", XCAPAPN: "xcap"}},
		},
	}, result)
	if result.XCAPRequired {
		t.Fatal("nil ePDG manager must not start a second PDN")
	}
}
