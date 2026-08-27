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

func TestParseActivationCodePrefersLPAOverLabeledHost(t *testing.T) {
	parsed, err := ParseActivationCode("SM-DP+ Address: rsp.truphone.com\n\nScan this QR:\nLPA:1$rsp.truphone.com$GG-MATCH-1")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.SMDP != "rsp.truphone.com" || parsed.MatchingID != "GG-MATCH-1" {
		t.Fatalf("mixed paste = %#v", parsed)
	}
	parsed, err = ParseActivationCode("SM-DP+ Address: rsp.truphone.com\nActivation Code: LPA:1$rsp.truphone.com$GG-MATCH-1")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.MatchingID != "GG-MATCH-1" {
		t.Fatalf("activation code LPA matching = %q", parsed.MatchingID)
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
	host, matching, err = ResolveDownloadAddress("consumer.e-sim.global\nAIRALO-TOKEN", "")
	if err != nil {
		t.Fatal(err)
	}
	if host != "consumer.e-sim.global" || matching != "AIRALO-TOKEN" {
		t.Fatalf("two-line host=%q matching=%q", host, matching)
	}
	host, matching, err = ResolveDownloadAddress("SM-DP+ Address: smdp.esim.wo.com.cn\n激活码: CU-TOKEN-22", "")
	if err != nil {
		t.Fatal(err)
	}
	if host != "smdp.esim.wo.com.cn" || matching != "CU-TOKEN-22" {
		t.Fatalf("labeled host=%q matching=%q", host, matching)
	}
}

func TestResolveDownloadQueryFallsBackToSMDP(t *testing.T) {
	host, matching, err := ResolveDownloadQuery("use the SM-DP+ and Matching ID below", "rsp.truphone.com", "HAND-EDIT")
	if err != nil {
		t.Fatal(err)
	}
	if host != "rsp.truphone.com" || matching != "HAND-EDIT" {
		t.Fatalf("fallback host=%q matching=%q", host, matching)
	}
}
