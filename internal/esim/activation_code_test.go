package esim

import "testing"

func TestParseActivationCodeVoxiStyle(t *testing.T) {
	parsed, err := ParseActivationCode("LPA:1$vfgb.esim.vodafone.com$JN-ABCDE-12345")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SMDP != "vfgb.esim.vodafone.com" || parsed.MatchingID != "JN-ABCDE-12345" {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestParseActivationCodeAppleWrapped(t *testing.T) {
	parsed, err := ParseActivationCode("https://esimsetup.apple.com/esim_qrcode_provisioning?carddata=LPA:1%24rsp.truphone.com%24MATCH-1")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SMDP != "rsp.truphone.com" || parsed.MatchingID != "MATCH-1" {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestParseActivationCodeMarketFormats(t *testing.T) {
	hostToken, err := ParseActivationCode("rsp.redtea.io$RT-TOKEN-9")
	if err != nil {
		t.Fatal(err)
	}
	if hostToken.SMDP != "rsp.redtea.io" || hostToken.MatchingID != "RT-TOKEN-9" {
		t.Fatalf("host token = %#v", hostToken)
	}
	labeled, err := ParseActivationCode("SM-DP+ Address: smdp.esim.wo.com.cn\n激活码: CU-TOKEN-22\n确认码: 654321")
	if err != nil {
		t.Fatal(err)
	}
	if labeled.SMDP != "smdp.esim.wo.com.cn" || labeled.MatchingID != "CU-TOKEN-22" || labeled.ConfirmationCode != "654321" {
		t.Fatalf("labeled = %#v", labeled)
	}
	twoLine, err := ParseActivationCode("consumer.e-sim.global\nAIRALO-TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if twoLine.SMDP != "consumer.e-sim.global" || twoLine.MatchingID != "AIRALO-TOKEN" {
		t.Fatalf("two-line = %#v", twoLine)
	}
}

func TestResolveDownloadAddressAcceptsHostOrLPA(t *testing.T) {
	host, matching, err := ResolveDownloadAddress("LPA:1$smdp.example.com$token", "")
	if err != nil {
		t.Fatal(err)
	}
	if host != "smdp.example.com" || matching != "token" {
		t.Fatalf("host=%q matching=%q", host, matching)
	}
	host, matching, err = ResolveDownloadAddress("https://rsp.truphone.com", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if host != "rsp.truphone.com" || matching != "abc" {
		t.Fatalf("host=%q matching=%q", host, matching)
	}
}
