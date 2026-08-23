package volte

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureWritesBackupWithSchemaIMEIAndTargets(t *testing.T) {
	modem := newFakeModem()
	dir := t.TempDir()
	p := NewProvisioner(modem, &FileStore{Dir: dir})
	res, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || !res.Verified || res.Restored {
		t.Fatalf("result %+v", res)
	}
	raw, err := os.ReadFile(res.SnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Schema != SnapshotSchema || snap.IMEI != modem.IMEI {
		t.Fatalf("snap %+v", snap)
	}
	if snap.Original.IMSEnabled || snap.Original.VoLTEEnabled {
		t.Fatalf("original should be pre-write: %+v", snap.Original)
	}
	if !snap.Target.IMSEnabled || !snap.Target.VoLTEEnabled {
		t.Fatalf("target %+v", snap.Target)
	}
	if strings.Contains(strings.ToLower(string(raw)), "ttyusb") || strings.Contains(string(raw), "/dev/") {
		t.Fatalf("snapshot leaked port: %s", raw)
	}
	if strings.Contains(strings.ToLower(string(raw)), "password") {
		t.Fatal("snapshot leaked secret")
	}
	info, err := os.Stat(res.SnapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm %s", info.Mode())
	}
	if filepath.Base(res.SnapshotPath) == modem.IMEI+".json" {
		t.Fatal("filename should be hashed, not raw IMEI")
	}
}

func TestApplyOnlyChangesApprovedFieldsAndPreservesVIDPID(t *testing.T) {
	modem := newFakeModem()
	p := NewProvisioner(modem, &MemoryStore{})
	_, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true), UACEnabled: boolPtr(true)})
	if !errors.Is(err, ErrRebootRequired) {
		t.Fatalf("want reboot required without waiter, got %v", err)
	}
	if modem.USB[0] != "0x2C7C" || modem.USB[1] != "0x125" {
		t.Fatalf("VID/PID changed: %v", modem.USB)
	}
	if modem.USB[len(modem.USB)-1] != "1" {
		t.Fatalf("UAC bit not written: %v", modem.USB)
	}
	for _, cmd := range modem.Commands {
		if strings.Contains(cmd, "/dev/") || strings.Contains(strings.ToLower(cmd), "ttyusb") {
			t.Fatalf("command used hardcoded port: %s", cmd)
		}
		if strings.Contains(cmd, "0x2C7C") && strings.Contains(cmd, "usbcfg") {
			if !strings.Contains(cmd, "0x125") {
				t.Fatalf("usbcfg dropped PID: %s", cmd)
			}
		}
	}
}

func TestApplyIMS11ErrorFallsBackAndReportsVoLTEDrift(t *testing.T) {
	modem := newFakeModem()
	modem.FailIMS11 = true
	p := NewProvisioner(modem, &MemoryStore{})
	res, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true)})
	if !errors.Is(err, ErrFieldDrift) {
		t.Fatalf("want field drift, got %v", err)
	}
	se := &StageError{}
	if !errors.As(err, &se) || se.Stage != StageApplyIMS {
		t.Fatalf("stage %+v err=%v", se, err)
	}
	if res.Restored || res.OK {
		t.Fatalf("must not claim success/restored: %+v", res)
	}
	if !res.Current.IMSEnabled || res.Current.VoLTEEnabled {
		t.Fatalf("current %+v", res.Current)
	}
	if len(res.Drift) != 1 || res.Drift[0] != "volte" {
		t.Fatalf("drift %v", res.Drift)
	}
}

func TestApplyFailureDoesNotClaimRestored(t *testing.T) {
	modem := newFakeModem()
	modem.Fail = map[string]error{`AT+QCFG="ims",1,1`: errors.New("bus down"), `AT+QCFG="ims",1`: errors.New("bus down")}
	p := NewProvisioner(modem, &MemoryStore{})
	res, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true)})
	if err == nil || res.Restored || res.OK {
		t.Fatalf("res %+v err=%v", res, err)
	}
	se := &StageError{}
	if !errors.As(err, &se) || se.Stage != StageApplyIMS {
		t.Fatalf("stage %+v", se)
	}
}

func TestRestoreRequiresIMEIAndVIDPIDMatch(t *testing.T) {
	modem := newFakeModem()
	store := &MemoryStore{}
	p := NewProvisioner(modem, store)
	if _, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	modem.IMS, modem.VoLTE = 1, 1
	other := newFakeModem()
	other.IMEI = "869999999999999"
	other.ID = "wwan0"
	_, err := NewProvisioner(other, store).Restore(context.Background(), "wwan0")
	if !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("other IMEI should not load this snapshot: %v", err)
	}
	modem.USB[0] = "0xDEAD"
	_, err = p.Restore(context.Background(), "wwan1")
	if !errors.Is(err, ErrVIDPIDMismatch) {
		t.Fatalf("want VID/PID mismatch, got %v", err)
	}
}

func TestRestoreSucceedsWhenIdentityMatches(t *testing.T) {
	modem := newFakeModem()
	p := NewProvisioner(modem, &MemoryStore{})
	if _, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	res, err := p.Restore(context.Background(), "wwan1")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Restored || !res.OK || res.Current.IMSEnabled {
		t.Fatalf("restore %+v", res)
	}
	if modem.IMS != 0 || modem.VoLTE != 0 {
		t.Fatalf("modem state IMS=%d volte=%d", modem.IMS, modem.VoLTE)
	}
}

func TestReenumerationUsesIMEINotPort(t *testing.T) {
	modem := newFakeModem()
	modem.UACImmediate = false
	modem.NextID = "wwan1-re"
	modem.NextPort = "/dev/ttyUSB99"
	p := NewProvisioner(modem, &MemoryStore{}).WithWaiter(&reenumWaiter{modem: modem}).WithOptions(ApplyOptions{WaitReenumerate: time.Second})
	res, err := p.Ensure(context.Background(), "wwan1", Desired{UACEnabled: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if res.DeviceID != "wwan1-re" {
		t.Fatalf("device after reenum %q", res.DeviceID)
	}
	if !res.Current.UACEnabled || !res.OK {
		t.Fatalf("uac after reenum %+v", res)
	}
	for _, cmd := range modem.Commands {
		if strings.Contains(cmd, "/dev/ttyUSB") {
			t.Fatalf("used port in AT: %s", cmd)
		}
	}
}

func TestWaitTimeoutDoesNotClaimRestored(t *testing.T) {
	modem := newFakeModem()
	modem.UACImmediate = false
	p := NewProvisioner(modem, &MemoryStore{}).WithWaiter(&reenumWaiter{modem: modem, timeout: true}).WithOptions(ApplyOptions{WaitReenumerate: 10 * time.Millisecond})
	res, err := p.Ensure(context.Background(), "wwan1", Desired{UACEnabled: boolPtr(true)})
	if err == nil || res.Restored || res.OK || res.Verified {
		t.Fatalf("res %+v err=%v", res, err)
	}
	se := &StageError{}
	if !errors.As(err, &se) || se.Stage != StageReenumerate {
		t.Fatalf("stage %+v", se)
	}
}

func TestRestoreWriteFailureStage(t *testing.T) {
	modem := newFakeModem()
	p := NewProvisioner(modem, &MemoryStore{})
	if _, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	modem.Fail = map[string]error{
		`AT+QCFG="ims",0,0`: errors.New("nv locked"),
		`AT+QCFG="ims",0`:   errors.New("nv locked"),
	}
	res, err := p.Restore(context.Background(), "wwan1")
	if err == nil || res.Restored {
		t.Fatalf("res %+v err=%v", res, err)
	}
	se := &StageError{}
	if !errors.As(err, &se) || se.Stage != StageRestoreWrite {
		t.Fatalf("stage %+v err=%v", se, err)
	}
}

func TestFirstBackupOriginalNotOverwritten(t *testing.T) {
	modem := newFakeModem()
	store := &MemoryStore{}
	p := NewProvisioner(modem, store)
	if _, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Ensure(context.Background(), "wwan1", Desired{UACEnabled: boolPtr(true)}); !errors.Is(err, ErrRebootRequired) {
		t.Fatalf("second apply: %v", err)
	}
	snap, _, err := store.Load(modem.IMEI)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Original.IMSEnabled || snap.Original.UACEnabled {
		t.Fatalf("original mutated %+v", snap.Original)
	}
	if !snap.Target.UACEnabled {
		t.Fatalf("target not updated %+v", snap.Target)
	}
}

func TestIdentityMissingIMEI(t *testing.T) {
	modem := newFakeModem()
	modem.IMEI = ""
	p := NewProvisioner(modem, &MemoryStore{})
	_, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true)})
	se := &StageError{}
	if !errors.As(err, &se) || se.Stage != StageIdentity {
		t.Fatalf("stage %+v err=%v", se, err)
	}
}

func TestMidApplyUSBCFGFailureKeepsIMSWriteAndReportsStage(t *testing.T) {
	modem := newFakeModem()
	modem.Fail = map[string]error{
		`AT+QCFG="usbcfg",0x2C7C,0x125,1,1,1,1,1,0,1`: errors.New("usb busy"),
	}
	p := NewProvisioner(modem, &MemoryStore{})
	res, err := p.Ensure(context.Background(), "wwan1", Desired{IMSEnabled: boolPtr(true), VoLTEEnabled: boolPtr(true), UACEnabled: boolPtr(true)})
	if err == nil || res.Restored || res.OK {
		t.Fatalf("res %+v err=%v", res, err)
	}
	se := &StageError{}
	if !errors.As(err, &se) || se.Stage != StageApplyUSBCFG {
		t.Fatalf("stage %+v err=%v", se, err)
	}
	if !res.Current.IMSEnabled {
		t.Fatal("IMS write should have happened before USBCFG failure")
	}
}
