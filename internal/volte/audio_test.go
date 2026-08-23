package volte

import (
	"errors"
	"testing"
)

type fakeRunner struct {
	starts [][]string
	pids   []int
	stops  []int
	fail   error
}

func (f *fakeRunner) Start(_ string, args ...string) (int, error) {
	cp := append([]string(nil), args...)
	f.starts = append(f.starts, cp)
	if f.fail != nil {
		return 0, f.fail
	}
	pid := 100 + len(f.starts)
	f.pids = append(f.pids, pid)
	return pid, nil
}
func (f *fakeRunner) Stop(pid int) error {
	f.stops = append(f.stops, pid)
	return nil
}

type fakeCards map[string]string

func (f fakeCards) DeviceForUSBParent(usbParent string) (string, error) {
	dev, ok := f[usbParent]
	if !ok {
		return "", errors.New("alsa not found")
	}
	return dev, nil
}

func testIdentity(device, usb string) AudioIdentity {
	return AudioIdentity{
		DeviceID:   device,
		IMEI:       "861234567890123",
		Firmware:   "QDC507GLEFM21",
		Kernel:     "6.1.0",
		USBParent:  usb,
		HelperHash: "abc",
	}
}

func TestAudioRuntimeBindsByUSBParentNotCardIndex(t *testing.T) {
	runner := &fakeRunner{}
	rt := NewAudioRuntime(testIdentity("wwan1", "1-2"), runner, fakeCards{
		"1-2": "hw:CARD=QDC1",
		"1-4": "hw:CARD=QDC2",
	}, "/opt/hideck/volte-helper")
	if err := rt.Bind(testIdentity("wwan1", "1-2")); err != nil {
		t.Fatal(err)
	}
	other := testIdentity("wwan0", "1-4")
	other.IMEI = "869999999999999"
	if err := rt.Bind(other); !errors.Is(err, ErrAudioUnsupported) {
		t.Fatalf("want IMEI mismatch, got %v", err)
	}
	rt2 := NewAudioRuntime(testIdentity("wwan0", "1-4"), runner, fakeCards{"1-4": "hw:CARD=QDC2"}, "/opt/hideck/volte-helper")
	if err := rt2.Bind(testIdentity("wwan0", "1-4")); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start("wwan1", "c1"); err != nil {
		t.Fatal(err)
	}
	if err := rt2.Start("wwan0", "c2"); err != nil {
		t.Fatal(err)
	}
	if got := runner.starts[0][1]; got != "hw:CARD=QDC1" {
		t.Fatalf("device arg %s", got)
	}
	if got := runner.starts[1][1]; got != "hw:CARD=QDC2" {
		t.Fatalf("device arg %s", got)
	}
	a, _ := rt.Owner("c1")
	b, _ := rt2.Owner("c2")
	if a.alsa == b.alsa {
		t.Fatal("devices must not share ALSA endpoints")
	}
}

func TestAudioRuntimeStartFailureCleansOwnedPID(t *testing.T) {
	runner := &fakeRunner{fail: errors.New("helper failed")}
	rt := NewAudioRuntime(testIdentity("wwan1", "1-2"), runner, fakeCards{"1-2": "hw:CARD=QDC1"}, "helper")
	if err := rt.Bind(testIdentity("wwan1", "1-2")); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start("wwan1", "c1"); err == nil {
		t.Fatal("want start error")
	}
	if _, ok := rt.Owner("c1"); ok {
		t.Fatal("failed start must not keep ownership")
	}
	if len(runner.stops) != 0 {
		t.Fatalf("must not stop unknown pid: %v", runner.stops)
	}
}

func TestAudioRuntimeStopOnlyOwnedHelper(t *testing.T) {
	runner := &fakeRunner{}
	rt := NewAudioRuntime(testIdentity("wwan1", "1-2"), runner, fakeCards{"1-2": "hw:CARD=QDC1"}, "helper")
	if err := rt.Bind(testIdentity("wwan1", "1-2")); err != nil {
		t.Fatal(err)
	}
	if err := rt.Start("wwan1", "c1"); err != nil {
		t.Fatal(err)
	}
	if err := rt.Stop("c1"); err != nil {
		t.Fatal(err)
	}
	if len(runner.stops) != 1 || runner.stops[0] != runner.pids[0] {
		t.Fatalf("stops %v pids %v", runner.stops, runner.pids)
	}
	if err := rt.Stop("missing"); err != nil {
		t.Fatal(err)
	}
	if len(runner.stops) != 1 {
		t.Fatalf("stop missing should not kill extras: %v", runner.stops)
	}
}

func TestAudioRuntimeRejectsWrongHelperHash(t *testing.T) {
	rt := NewAudioRuntime(testIdentity("wwan1", "1-2"), &fakeRunner{}, fakeCards{"1-2": "hw:CARD=QDC1"}, "helper")
	id := testIdentity("wwan1", "1-2")
	id.HelperHash = "nope"
	if err := rt.Bind(id); !errors.Is(err, ErrAudioUnsupported) {
		t.Fatalf("got %v", err)
	}
}
