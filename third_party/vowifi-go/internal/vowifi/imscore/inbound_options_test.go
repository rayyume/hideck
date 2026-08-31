package imscore

import (
	"strings"
	"testing"
)

func TestBuildInboundOPTIONSResponseAdvertisesCapabilities(t *testing.T) {
	service := &Service{cfg: &IMSConfig{}}
	got, err := service.buildInboundOPTIONSResponse(optionsRequest("opt-cap"))
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"Allow:", "Supported:", "Accept: application/sdp"} {
		if !strings.Contains(got, header) {
			t.Fatalf("OPTIONS 200 missing %q: %s", header, got)
		}
	}
	if rawSIPHeaderValue(got, "CSeq") != "1 OPTIONS" {
		t.Fatalf("CSeq = %q", rawSIPHeaderValue(got, "CSeq"))
	}
	allow := rawSIPHeaderValue(got, "Allow")
	if allow != inboundSIPAllowHeader {
		t.Fatalf("Allow = %q, want %q", allow, inboundSIPAllowHeader)
	}
	supported := rawSIPHeaderValue(got, "Supported")
	for _, token := range []string{"path", "sec-agree", "outbound", "precondition"} {
		if !containsSIPListToken(supported, token) {
			t.Fatalf("Supported missing %s: %q", token, supported)
		}
	}
	for _, method := range []string{"UPDATE", "PRACK"} {
		if !containsSIPListToken(allow, method) {
			t.Fatalf("Allow missing %s: %q", method, allow)
		}
	}
	for _, unsupported := range []string{"REGISTER", "SUBSCRIBE", "PUBLISH"} {
		if containsSIPListToken(rawSIPHeaderValue(got, "Allow"), unsupported) {
			t.Fatalf("Allow advertises unsupported inbound method %s", unsupported)
		}
	}
}

func TestInboundOPTIONSSupportedMergesPreconditionWithoutDuplicates(t *testing.T) {
	service := &Service{cfg: &IMSConfig{RegisterTemplate: IMSRegisterTemplate{
		SupportedHeader: "path, PRECONDITION, outbound",
	}}}
	got, err := service.buildInboundOPTIONSResponse(optionsRequest("opt-pre"))
	if err != nil {
		t.Fatal(err)
	}
	supported := rawSIPHeaderValue(got, "Supported")
	count := 0
	for _, token := range strings.Split(supported, ",") {
		if strings.EqualFold(strings.TrimSpace(token), "precondition") {
			count++
		}
	}
	if count != 1 || !containsSIPListToken(supported, "path") || !containsSIPListToken(supported, "outbound") {
		t.Fatalf("Supported = %q", supported)
	}
}

func containsSIPListToken(value, token string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), token) {
			return true
		}
	}
	return false
}
