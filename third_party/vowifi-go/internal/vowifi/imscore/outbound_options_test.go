package imscore

import (
	"strings"
	"testing"
)

func TestParsePeerCapabilityResponseReadsAllowAndICSI(t *testing.T) {
	snapshot := parsePeerCapabilityResponse(
		"INVITE, ACK, OPTIONS, BYE",
		`<sip:pcscf.example;lr>;+g.3gpp.icsi-ref="urn:urn-7:3gpp-service.ims.icsi.mmtel,urn:urn-7:3gpp-service.ims.icsi.sms"`,
	)
	if strings.Join(snapshot.Allow, ",") != "INVITE,ACK,OPTIONS,BYE" {
		t.Fatalf("Allow = %#v", snapshot.Allow)
	}
	if len(snapshot.ICSI) != 2 || !strings.Contains(snapshot.ICSI[0], "mmtel") {
		t.Fatalf("ICSI = %#v", snapshot.ICSI)
	}
}

func TestCapabilityDiscoveryErrorDoesNotForceReregister(t *testing.T) {
	service := &Service{}
	if service.keepaliveFailureRequiresRefresh(errOPTIONSCapabilityDiscovery) {
		t.Fatal("capability OPTIONS failure must not refresh REGISTER")
	}
	if service.keepaliveFailureRequiresRefresh(errOPTIONSKeepalive) {
		t.Fatal("keepalive OPTIONS failure must not refresh REGISTER")
	}
}
