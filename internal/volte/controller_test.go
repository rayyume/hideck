package volte

import (
	"context"
	"errors"
	"testing"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

func TestControllerEnablesIMSAndUACWithoutClaimingUACReady(t *testing.T) {
	host := newFakeModem()
	ctl := NewControllerWithBackup(host, t.TempDir())
	if err := ctl.Enable(context.Background(), "wwan1"); err != nil {
		t.Fatal(err)
	}
	if host.StopIMS != 1 || host.SetNative != 1 {
		t.Fatalf("stop=%d setNative=%d", host.StopIMS, host.SetNative)
	}
	st := ctl.Status("wwan1")
	if !st.IMSEnabled || !st.VoLTEEnabled || !st.IMSRegistered || st.Phase != PhaseRegistered {
		t.Fatalf("status %+v", st)
	}
	if !st.RebootRequired {
		t.Fatal("UAC enable should mark reboot_required")
	}
	if st.UACEnabled {
		t.Fatal("must not claim UAC ready before re-enumeration verify")
	}
	var sawEnableIMS, sawEnableUAC bool
	for _, cmd := range host.Commands {
		if cmd == IMSEnableCommand() {
			sawEnableIMS = true
		}
		if cmd == `AT+QCFG="usbcfg",0x2C7C,0x125,1,1,1,1,1,0,1` {
			sawEnableUAC = true
		}
	}
	if !sawEnableIMS || !sawEnableUAC {
		t.Fatalf("commands %v", host.Commands)
	}
}

func TestControllerUnverifiedWhenIMSAMissing(t *testing.T) {
	host := newFakeModem()
	host.IMS, host.VoLTE = 1, 1
	host.USB[len(host.USB)-1] = "1"
	host.RegErr = context.DeadlineExceeded
	ctl := NewControllerWithBackup(host, t.TempDir())
	if err := ctl.Enable(context.Background(), "wwan1"); err != nil {
		t.Fatal(err)
	}
	st := ctl.Status("wwan1")
	if st.Phase != PhaseUnverified || !st.IMSEnabled {
		t.Fatalf("want unverified, got %+v", st)
	}
}

func TestControllerFailsWhenIMSClientsUnavailable(t *testing.T) {
	host := newFakeModem()
	host.IMS, host.VoLTE = 1, 1
	host.USB[len(host.USB)-1] = "1"
	host.EnsureErr = errors.New("QMI 服务未就绪: IMSA")
	ctl := NewControllerWithBackup(host, t.TempDir())
	if err := ctl.Enable(context.Background(), "wwan1"); err == nil {
		t.Fatal("want typed IMSA unavailable")
	}
	st := ctl.Status("wwan1")
	if !st.QMIIMSUnavailable || st.Phase != PhaseFailed {
		t.Fatalf("status %+v", st)
	}
}

func TestControllerReleasesIMSClientsAndFollowsIndications(t *testing.T) {
	host := newFakeModem()
	host.IMS, host.VoLTE = 1, 1
	host.USB[len(host.USB)-1] = "1"
	host.RegErr = context.DeadlineExceeded
	ctl := NewControllerWithBackup(host, t.TempDir())
	if err := ctl.Enable(context.Background(), "wwan1"); err != nil {
		t.Fatal(err)
	}
	if host.EnsureCount != 1 {
		t.Fatalf("ensure=%d", host.EnsureCount)
	}
	host.fireIMSRegistration(&qmi.IMSARegistrationStatus{
		HasStatus: true,
		Status:    qmi.IMSARegistrationStateRegistered,
	})
	st := ctl.Status("wwan1")
	if st.Phase != PhaseRegistered || !st.IMSRegistered {
		t.Fatalf("after indication %+v", st)
	}
	ctl.Disable("wwan1")
	if host.ReleaseCount != 1 {
		t.Fatalf("release=%d", host.ReleaseCount)
	}
	if ctl.Status("wwan1").Phase != PhaseIdle {
		t.Fatalf("after disable %+v", ctl.Status("wwan1"))
	}
}

func TestControllerRejectsUnknownPLMNWithoutGuessingMBN(t *testing.T) {
	host := newFakeModem()
	host.COPS = "23433"
	ctl := NewControllerWithBackup(host, t.TempDir())
	if err := ctl.Enable(context.Background(), "wwan1"); err == nil {
		t.Fatal("UK PLMN must not guess an MBN")
	}
	if ctl.Status("wwan1").Phase != PhaseFailed {
		t.Fatalf("status %+v", ctl.Status("wwan1"))
	}
}

func TestControllerKeepsGoingWhenVoLTEBitStuck(t *testing.T) {
	host := newFakeModem()
	host.FailIMS11 = true
	host.Reg = Registration{}
	ctl := NewControllerWithBackup(host, t.TempDir())
	if err := ctl.Enable(context.Background(), "wwan1"); err != nil {
		t.Fatal(err)
	}
	st := ctl.Status("wwan1")
	if !st.IMSEnabled || st.VoLTEEnabled {
		t.Fatalf("status %+v", st)
	}
	if st.ProvisionStage != StageApplyIMS {
		t.Fatalf("want apply_ims drift recorded, got %+v", st)
	}
}
