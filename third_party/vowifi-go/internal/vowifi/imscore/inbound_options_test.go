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
}
