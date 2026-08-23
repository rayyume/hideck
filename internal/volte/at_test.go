package volte

import "testing"

func TestParseIMSConfig(t *testing.T) {
	got, err := ParseIMSConfig("AT+QCFG=\"ims\"?\r\n+QCFG: \"ims\",0,0\r\n\r\nOK\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if got.IMSEnabled || got.VoLTEEnabled {
		t.Fatalf("want disabled, got %+v", got)
	}
	got, err = ParseIMSConfig("+QCFG: \"IMS\",1,1\nOK")
	if err != nil || !got.IMSEnabled || !got.VoLTEEnabled {
		t.Fatalf("want enabled, got %+v err=%v", got, err)
	}
	if _, err := ParseIMSConfig("ERROR"); err == nil {
		t.Fatal("missing line should fail")
	}
}

func TestParseUSBConfig(t *testing.T) {
	got, err := ParseUSBConfig(`+QCFG: "usbcfg",0x2C7C,0x125,1,1,1,1,1,0,0`)
	if err != nil {
		t.Fatal(err)
	}
	if got.UACEnabled {
		t.Fatal("last bit 0 should be UAC off")
	}
	if got.EnableCommand != `AT+QCFG="usbcfg",0x2C7C,0x125,1,1,1,1,1,0,1` {
		t.Fatalf("enable command = %q", got.EnableCommand)
	}
	on, err := ParseUSBConfig(`+QCFG: "usbcfg",0x2C7C,0x0125,1,1,1,1,1,0,1`)
	if err != nil || !on.UACEnabled || on.EnableCommand != "" {
		t.Fatalf("UAC on: %+v err=%v", on, err)
	}
}
