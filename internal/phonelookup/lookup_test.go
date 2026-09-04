package phonelookup

import "testing"

func TestCanonicalServiceNumber(t *testing.T) {
	if got := Canonical("+8610010"); got != "10010" {
		t.Fatalf("Canonical(+8610010)=%q", got)
	}
}

func TestLookupChineseServiceNumbers(t *testing.T) {
	cases := []struct {
		in, carrier string
	}{
		{"10086", "中国移动"},
		{"10010", "中国联通"},
		{"+8610010", "中国联通"},
		{"tel:10000", "中国电信"},
		{"10099", "中国广电"},
	}
	for _, tc := range cases {
		got := Lookup(tc.in)
		if got.Carrier != tc.carrier || got.Kind != "service" || got.Country != "中国" {
			t.Fatalf("Lookup(%q)=%+v want carrier %s service/中国", tc.in, got, tc.carrier)
		}
		if got.Subtitle == "" {
			t.Fatalf("Lookup(%q) missing subtitle", tc.in)
		}
	}
}

func TestLookupCNMobileCarrier(t *testing.T) {
	got := Lookup("18600001111")
	if got.Carrier != "中国联通" || got.Kind != "mobile" {
		t.Fatalf("%+v", got)
	}
	cm := Lookup("+8613800138000")
	if cm.Carrier != "中国移动" {
		t.Fatalf("%+v", cm)
	}
	ct := Lookup("13300001111")
	if ct.Carrier != "中国电信" {
		t.Fatalf("%+v", ct)
	}
}

func TestLookupInternationalCountry(t *testing.T) {
	got := Lookup("+447840844894")
	if got.Country != "英国" {
		t.Fatalf("%+v", got)
	}
	if got.Subtitle != "英国" {
		t.Fatalf("subtitle=%q", got.Subtitle)
	}
}

func TestLookupCNLandlineRegion(t *testing.T) {
	got := Lookup("01012345678")
	if got.Region != "北京" || got.Kind != "landline" {
		t.Fatalf("%+v", got)
	}
}

func TestLookupWithContactName(t *testing.T) {
	got := Lookup("10086").WithName("移动客服")
	if got.Title != "移动客服" || got.Name != "移动客服" || got.Carrier != "中国移动" {
		t.Fatalf("%+v", got)
	}
}
