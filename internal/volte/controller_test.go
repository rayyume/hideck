package volte

import (
	"context"
	"testing"
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
