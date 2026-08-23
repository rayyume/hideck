package volte

import (
	"context"
	"testing"
	"time"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

type fakeHost struct {
	imsQuery  string
	usbQuery  string
	commands  []string
	stopIMS   int
	setNative int
	ensureErr error
	reg       Registration
	regErr    error
	audio     string
}

func (f *fakeHost) ExecuteAT(_ string, cmd string, _ time.Duration) (string, error) {
	f.commands = append(f.commands, cmd)
	switch cmd {
	case IMSQueryCommand():
		return f.imsQuery, nil
	case USBConfigQueryCommand():
		return f.usbQuery, nil
	default:
		if cmd == IMSEnableCommand() {
			f.imsQuery = `+QCFG: "ims",1,1`
		}
		if len(cmd) > 10 && cmd[:10] == `AT+QCFG="u` {
			f.usbQuery = `+QCFG: "usbcfg",0x2C7C,0x125,1,1,1,1,1,0,1`
		}
		return "OK", nil
	}
}
func (f *fakeHost) StopSoftwareIMS(string) error { f.stopIMS++; return nil }
func (f *fakeHost) SetNativeIMS(context.Context, string, bool) error {
	f.setNative++
	return nil
}
func (f *fakeHost) EnsureIMSClients(context.Context, string) error { return f.ensureErr }
func (f *fakeHost) IMSAStatus(context.Context, string) (Registration, error) {
	return f.reg, f.regErr
}
func (f *fakeHost) AudioDevice(string) string { return f.audio }
func (f *fakeHost) VOICEDial(context.Context, string, string) (uint8, error) {
	return 1, nil
}
func (f *fakeHost) VOICEAnswer(context.Context, string, uint8) error { return nil }
func (f *fakeHost) VOICEHangup(context.Context, string, uint8) error { return nil }
func (f *fakeHost) VOICEBurstDTMF(context.Context, string, uint8, string) error {
	return nil
}
func (f *fakeHost) OnVoiceStatus(string, func(*qmi.VoiceAllCallInfo)) error { return nil }

func TestControllerEnablesIMSAndUACWithoutRebootClaim(t *testing.T) {
	host := &fakeHost{
		imsQuery: `+QCFG: "ims",0,0`,
		usbQuery: `+QCFG: "usbcfg",0x2C7C,0x125,1,1,1,1,1,0,0`,
		reg:      Registration{Registered: true, VoiceAvailable: true},
	}
	ctl := NewController(host)
	if err := ctl.Enable(context.Background(), "wwan1"); err != nil {
		t.Fatal(err)
	}
	if host.stopIMS != 1 || host.setNative != 1 {
		t.Fatalf("stop=%d setNative=%d", host.stopIMS, host.setNative)
	}
	st := ctl.Status("wwan1")
	if !st.IMSEnabled || !st.VoLTEEnabled || !st.IMSRegistered || st.Phase != PhaseRegistered {
		t.Fatalf("status %+v", st)
	}
	if !st.RebootRequired {
		t.Fatal("UAC enable should mark reboot_required")
	}
	var sawEnableIMS, sawEnableUAC bool
	for _, cmd := range host.commands {
		if cmd == IMSEnableCommand() {
			sawEnableIMS = true
		}
		if cmd == `AT+QCFG="usbcfg",0x2C7C,0x125,1,1,1,1,1,0,1` {
			sawEnableUAC = true
		}
	}
	if !sawEnableIMS || !sawEnableUAC {
		t.Fatalf("commands %v", host.commands)
	}
}

func TestControllerUnverifiedWhenIMSAMissing(t *testing.T) {
	host := &fakeHost{
		imsQuery: `+QCFG: "ims",1,1`,
		usbQuery: `+QCFG: "usbcfg",0x2C7C,0x125,1,1,1,1,1,0,1`,
		regErr:   context.DeadlineExceeded,
	}
	ctl := NewController(host)
	if err := ctl.Enable(context.Background(), "wwan1"); err != nil {
		t.Fatal(err)
	}
	st := ctl.Status("wwan1")
	if st.Phase != PhaseUnverified || !st.IMSEnabled {
		t.Fatalf("want unverified, got %+v", st)
	}
}
