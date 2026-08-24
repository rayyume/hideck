package config

import "testing"

func TestNormalizeIMEI(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain 15-digit", "860000000004004", "86000000000400"},
		{"trailing space", "860000000004004 ", "86000000000400"},
		{"leading space + newline", " 860000000004004\n", "86000000000400"},
		{"imeisv 16-digit", "8600000000040001", "86000000000400"},
		{"embedded non-digits", "86-0000 0000.04004", "86000000000400"},
		{"too short", "12345", ""},
		{"empty", "", ""},
		{"exactly 14", "86000000000400", "86000000000400"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeIMEI(tc.in); got != tc.want {
				t.Fatalf("NormalizeIMEI(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestIMEIMatches(t *testing.T) {
	if !IMEIMatches("860000000004004", " 860000000004004") {
		t.Fatal("whitespace-differing same IMEI should match")
	}
	if !IMEIMatches("860000000004004", "8600000000040001") {
		t.Fatal("IMEI(15) and IMEISV(16) of same modem should match")
	}
	if IMEIMatches("860000000004004", "860000000005005") {
		t.Fatal("different modems must not match")
	}
	if IMEIMatches("", "") {
		t.Fatal("empty must never match")
	}
	if IMEIMatches("12345", "12345") {
		t.Fatal("invalid (<14 digits) must never match")
	}
}
