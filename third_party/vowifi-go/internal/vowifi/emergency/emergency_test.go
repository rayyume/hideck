package emergency

import "testing"

func TestIsEmergencyDestination(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{in: "999", want: true},
		{in: "112", want: true},
		{in: "+112", want: true},
		{in: "tel:911", want: true},
		{in: "urn:service:sos", want: true},
		{in: "URN:service:sos.police", want: true},
		{in: "+447700900123", want: false},
		{in: "43430", want: false},
		{in: "sip:+447700900123@ims.example;user=phone", want: false},
		{in: "", want: false},
	}
	for _, test := range tests {
		if got := IsEmergencyDestination(test.in); got != test.want {
			t.Fatalf("%q: got %v want %v", test.in, got, test.want)
		}
	}
}

func TestServiceURNFor(t *testing.T) {
	if got := ServiceURNFor("999"); got != ServiceURN {
		t.Fatalf("999 -> %q", got)
	}
	if got := ServiceURNFor("urn:service:sos.ambulance"); got != "urn:service:sos.ambulance" {
		t.Fatalf("urn subtype -> %q", got)
	}
	if got := ServiceURNFor("+447700900123"); got != "" {
		t.Fatalf("normal number -> %q", got)
	}
}

func TestEmergencyEPDGAddr(t *testing.T) {
	got := EmergencyEPDGAddr("234", "15")
	want := "sos.epdg.epc.mnc015.mcc234.pub.3gppnetwork.org"
	if got != want {
		t.Fatalf("ePDG = %q, want %q", got, want)
	}
}
