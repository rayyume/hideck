package phonelookup

import (
	"strings"
	"testing"

	"github.com/nyaruka/phonenumbers/v2"
)

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
	got := LookupWithRegion("18600001111", "CN")
	if got.Carrier != "中国联通" || got.Kind != "mobile" {
		t.Fatalf("%+v", got)
	}
	if got.Region != "北京" {
		t.Fatalf("region=%q %+v", got.Region, got)
	}
	cm := Lookup("+8613800138000")
	if cm.Carrier != "中国移动" {
		t.Fatalf("%+v", cm)
	}
	if cm.Region != "北京" {
		t.Fatalf("region=%q %+v", cm.Region, cm)
	}
	ct := LookupWithRegion("13300001111", "CN")
	if ct.Carrier != "中国电信" {
		t.Fatalf("%+v", ct)
	}
	if ct.Region != "广西南宁" {
		t.Fatalf("region=%q %+v", ct.Region, ct)
	}
}

func TestLookupInternationalCountry(t *testing.T) {
	got := Lookup("+447840844894")
	if got.Country != "英国" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Subtitle, "英国") {
		t.Fatalf("subtitle=%q", got.Subtitle)
	}
}

func TestLookupIntlAreaAndCarrier(t *testing.T) {
	us := Lookup("+17047181840")
	if us.Country != "美国" {
		t.Fatalf("%+v", us)
	}
	if us.Region == "" || us.Region == "美国" {
		t.Fatalf("expected US area, got %+v", us)
	}
	cn := Lookup("+8613702032331")
	if cn.Carrier != "中国移动" || cn.Region != "天津" {
		t.Fatalf("%+v", cn)
	}
}

func TestLookupCNLandlineRegion(t *testing.T) {
	got := LookupWithRegion("01012345678", "CN")
	if got.Region != "北京" || got.Kind != "landline" {
		t.Fatalf("%+v", got)
	}
}

func TestNormalizeNationalNumberUsesExplicitRegion(t *testing.T) {
	tests := []struct {
		name, number, region, want string
	}{
		{name: "UK mobile", number: "07911123456", region: "GB", want: "+447911123456"},
		{name: "North American", number: "14165550123", region: "CA", want: "+14165550123"},
		{name: "China mobile", number: "18600001111", region: "CN", want: "+8618600001111"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeWithRegion(test.number, test.region); got != test.want {
				t.Fatalf("NormalizeWithRegion(%q, %q)=%q want %q", test.number, test.region, got, test.want)
			}
		})
	}
}

func TestNormalizeNationalNumberDoesNotAssumeChina(t *testing.T) {
	for _, number := range []string{"07911123456", "14165550123"} {
		if got := Normalize(number); got != number {
			t.Fatalf("Normalize(%q)=%q want unchanged", number, got)
		}
		if got := Lookup(number); got.Country != "" {
			t.Fatalf("Lookup(%q) inferred country %q without a region", number, got.Country)
		}
	}
}

func TestLookupWithContactName(t *testing.T) {
	got := Lookup("10086").WithName("移动客服")
	if got.Title != "移动客服" || got.Name != "移动客服" || got.Carrier != "中国移动" {
		t.Fatalf("%+v", got)
	}
}

func TestLookupWorldExampleNumbersHaveCountry(t *testing.T) {
	missing := 0
	for iso := range phonenumbers.GetSupportedRegions() {
		ex := phonenumbers.GetExampleNumber(iso)
		if ex == nil {
			continue
		}
		got := Lookup(phonenumbers.Format(ex, phonenumbers.E164))
		if strings.TrimSpace(got.Country) == "" {
			missing++
			if missing <= 15 {
				t.Errorf("region %s example %s country empty: %+v", iso, phonenumbers.Format(ex, phonenumbers.E164), got)
			}
		}
	}
	if missing > 15 {
		t.Errorf("%d regions missing country", missing)
	}
}

func TestLookupForeignCountries(t *testing.T) {
	cases := []struct {
		in, country string
	}{
		{"+37251234567", "爱沙尼亚"},
		{"+3548212345", "冰岛"},
		{"+421905123456", "斯洛伐克"},
		{"+77011234567", "哈萨克斯坦"},
		{"+12422591234", "巴哈马"},
		{"+18764501234", "牙买加"},
		{"+14165550123", "加拿大"},
		{"+12025550123", "美国"},
		{"+85251234567", "香港"},
		{"+886912345678", "台湾"},
		{"+80012345678", "国际免费电话"},
		{"+447700900123", "英国"},
	}
	for _, tc := range cases {
		got := Lookup(tc.in)
		if got.Country != tc.country {
			t.Fatalf("Lookup(%q).Country=%q want %q (%+v)", tc.in, got.Country, tc.country, got)
		}
	}
}
