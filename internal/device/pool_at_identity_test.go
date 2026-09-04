package device

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yibaiba/hideck/internal/backend"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/modem"
)

func TestRefreshIdentityLiveFallsBackToATWhenQMIEmpty(t *testing.T) {
	orig := probeATIdentityFn
	t.Cleanup(func() { probeATIdentityFn = orig })
	probeATIdentityFn = func(atPort string, _ time.Duration) (modem.ATIdentity, error) {
		if atPort != "/dev/ttyUSB2" {
			t.Fatalf("atPort=%q want /dev/ttyUSB2", atPort)
		}
		return modem.ATIdentity{
			IMEI:  "860000000000001",
			IMSI:  "460010123456789",
			ICCID: "8986000000000000001",
		}, nil
	}

	w := &Worker{
		ID: "wwan1",
		Config: config.DeviceConfig{
			ID:            "wwan1",
			DeviceBackend: backend.BackendQMI,
			ATPort:        "/dev/ttyUSB2",
			ModemIMEI:     "860000000000001",
		},
		Backend: &workerStartupIdentityBackendStub{
			workerPhoneBackendStub: workerPhoneBackendStub{
				workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI},
			},
		},
	}
	result, err := w.refreshIdentityLive(context.Background(), "qmi_health_threshold")
	if err != nil {
		t.Fatalf("refreshIdentityLive() error=%v", err)
	}
	if result.IMSI != "460010123456789" || result.ICCID != "8986000000000000001" {
		t.Fatalf("result=%+v", result)
	}
	if w.state.Identity.NativeMCC != "460" || w.state.Identity.NativeMNC != "01" {
		t.Fatalf("mcc/mnc=%s/%s want 460/01", w.state.Identity.NativeMCC, w.state.Identity.NativeMNC)
	}
	if w.state.Identity.IMEI != "860000000000001" {
		t.Fatalf("imei=%q", w.state.Identity.IMEI)
	}
}

func TestProbeWorkerATIdentityRequiresPort(t *testing.T) {
	_, err := probeWorkerATIdentity(context.Background(), &Worker{})
	if err != errATIdentityUnavailable {
		t.Fatalf("err=%v want errATIdentityUnavailable", err)
	}
}

func TestRefreshIdentityLiveFillsMissingQMIFieldFromAT(t *testing.T) {
	orig := probeATIdentityFn
	t.Cleanup(func() { probeATIdentityFn = orig })
	probeATIdentityFn = func(string, time.Duration) (modem.ATIdentity, error) {
		return modem.ATIdentity{
			IMEI: "860000000000001", IMSI: "460010123456789", ICCID: "8986000000000000002",
		}, nil
	}
	w := &Worker{
		Config: config.DeviceConfig{ATPort: "/dev/ttyUSB2"},
		Backend: &workerStartupIdentityBackendStub{
			workerPhoneBackendStub: workerPhoneBackendStub{
				workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI},
			},
			liveIMSI: "460010123456789",
		},
	}
	w.state.Identity.ICCID = "8986000000000000001"
	w.state.Identity.IMSI = "460000123456789"

	result, err := w.refreshIdentityLive(context.Background(), "sim_swap")
	if err != nil {
		t.Fatal(err)
	}
	if result.ICCID != "8986000000000000002" || result.IMSI != "460010123456789" {
		t.Fatalf("result=%+v", result)
	}
	if w.state.Identity.ICCID != result.ICCID || w.state.Identity.IMSI != result.IMSI {
		t.Fatalf("stored identity=%+v", w.state.Identity)
	}
}

func TestRefreshIdentityLiveRejectsPartialReplacementIdentity(t *testing.T) {
	orig := probeATIdentityFn
	t.Cleanup(func() { probeATIdentityFn = orig })
	probeATIdentityFn = func(string, time.Duration) (modem.ATIdentity, error) {
		return modem.ATIdentity{IMSI: "460010123456789"}, errors.New("at identity probe incomplete")
	}
	w := &Worker{
		Config: config.DeviceConfig{ATPort: "/dev/ttyUSB2"},
		Backend: &workerStartupIdentityBackendStub{
			workerPhoneBackendStub: workerPhoneBackendStub{
				workerStatusBackendStub: workerStatusBackendStub{mode: backend.BackendQMI},
			},
			liveIMSI: "460010123456789",
		},
	}
	w.state.Identity.ICCID = "8986000000000000001"
	w.state.Identity.IMSI = "460000123456789"

	if _, err := w.refreshIdentityLive(context.Background(), "sim_swap"); err == nil {
		t.Fatal("partial replacement identity must remain pending")
	}
	if w.state.Identity.ICCID != "8986000000000000001" || w.state.Identity.IMSI != "460000123456789" {
		t.Fatalf("partial identity overwrote stable state: %+v", w.state.Identity)
	}
}
