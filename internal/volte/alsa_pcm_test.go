package volte

import "testing"

func TestValidateALSADevice(t *testing.T) {
	if err := validateALSADevice("hw:1,0"); err != nil {
		t.Fatal(err)
	}
	if err := validateALSADevice("hw:0,1"); err != nil {
		t.Fatal(err)
	}
	if err := validateALSADevice("default"); err == nil {
		t.Fatal("default must be rejected")
	}
	if err := validateALSADevice("hw:1,0;reboot"); err == nil {
		t.Fatal("injected args must be rejected")
	}
	card, dev, err := parseALSADevice("hw:2,0")
	if err != nil || card != 2 || dev != 0 {
		t.Fatalf("parse %d %d %v", card, dev, err)
	}
}
