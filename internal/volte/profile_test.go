package volte

import (
	"errors"
	"testing"
)

func TestUniqueMBNChinaCarriers(t *testing.T) {
	list := []MBNEntry{
		{Name: "ROW_Generic_3GPP"},
		{Name: "Volte_OpenMkt-Commercial-CMCC"},
		{Name: "VoLTE_OPNMKT_CT"},
		{Name: "CU-VoLTE"},
	}
	cases := []struct{ mcc, mnc, want string }{
		{"460", "00", "Volte_OpenMkt-Commercial-CMCC"},
		{"460", "11", "VoLTE_OPNMKT_CT"},
		{"460", "01", "CU-VoLTE"},
		{"460", "7", "Volte_OpenMkt-Commercial-CMCC"},
		{"460", "15", "Volte_OpenMkt-Commercial-CMCC"},
	}
	for _, tc := range cases {
		got, err := UniqueMBN(tc.mcc, tc.mnc, list)
		if err != nil || got != tc.want {
			t.Fatalf("%s/%s: got %q err=%v want %q", tc.mcc, tc.mnc, got, err, tc.want)
		}
	}
}

func TestUniqueMBNUnknownOrMissing(t *testing.T) {
	list := []MBNEntry{{Name: "ROW_Generic_3GPP"}}
	if _, err := UniqueMBN("234", "33", list); !errors.Is(err, ErrNoUniqueProfile) {
		t.Fatalf("UK IMSI must not guess ROW: %v", err)
	}
	if _, err := UniqueMBN("460", "00", list); !errors.Is(err, ErrNoUniqueProfile) {
		t.Fatalf("missing CMCC name must fail: %v", err)
	}
}

func TestUniqueMBNChinaBroadnetPrefersDedicatedProfile(t *testing.T) {
	list := []MBNEntry{
		{Name: "ROW_Generic_3GPP"},
		{Name: "Volte_OpenMkt-Commercial-CMCC"},
		{Name: "CBN-VoLTE"},
	}
	got, err := UniqueMBN("460", "15", list)
	if err != nil || got != "CBN-VoLTE" {
		t.Fatalf("want dedicated CBN profile, got %q err=%v", got, err)
	}
}
