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

func TestParseIMEIAndMBN(t *testing.T) {
	imei, err := ParseIMEI("AT+CGSN\r\n861234567890123\r\n\r\nOK\r\n")
	if err != nil || imei != "861234567890123" {
		t.Fatalf("imei %q err=%v", imei, err)
	}
	imei, err = ParseIMEI("+CGSN: 861234567890123\nOK")
	if err != nil || imei != "861234567890123" {
		t.Fatalf("prefixed imei %q err=%v", imei, err)
	}
	if _, err := ParseIMEI("ERROR"); err == nil {
		t.Fatal("missing IMEI should fail")
	}
	on, _, err := ParseMBNAutoSel("+QMBNCFG: \"AutoSel\",1\r\nOK")
	if err != nil || !on {
		t.Fatalf("autosel %v err=%v", on, err)
	}
	entries, err := ParseMBNList(`+QMBNCFG: "List",0,1,1,"ROW_Generic_3GPP",0x0501081F,202112292
+QMBNCFG: "List",12,0,0,"VoLTE_OPNMKT_CT",0x050113FC,202201101
OK`)
	if err != nil {
		t.Fatal(err)
	}
	sel, ok := SelectedMBN(entries)
	if !ok || sel.Name != "ROW_Generic_3GPP" || sel.Index != 0 {
		t.Fatalf("selected %+v ok=%v", sel, ok)
	}
}

func TestUSBCFGHelpers(t *testing.T) {
	fields := withUACFields([]string{"0x2C7C", "0x125", "1", "1", "1", "1", "1", "0", "0"}, true)
	if fields[len(fields)-1] != "1" || fields[0] != "0x2C7C" {
		t.Fatalf("fields %v", fields)
	}
	if canonHexID("0x0125") != canonHexID("0x125") {
		t.Fatal("PID forms should match")
	}
	if USBConfigSetCommand(fields) != `AT+QCFG="usbcfg",0x2C7C,0x125,1,1,1,1,1,0,1` {
		t.Fatalf("cmd %s", USBConfigSetCommand(fields))
	}
	if !isATError("AT+QCFG=\"ims\",1,1\r\nERROR") {
		t.Fatal("ERROR should be detected")
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
