package imscore

import (
	"net"
	"strings"
	"testing"
)

func TestPCSCFPreloadedRouteUsesProtectedServerPort(t *testing.T) {
	service := &Service{
		cfg:                &IMSConfig{Transport: "udp"},
		registrationRemote: &net.UDPAddr{IP: net.ParseIP("10.128.120.17"), Port: 5060},
		regSession: &registerSession{
			security: &securityAgreement{server: &securityMechanism{PortS: 50600}},
		},
	}
	service.registrationTCP = nil
	got := service.pcscfPreloadedRouteLocked()
	want := "<sip:10.128.120.17:50600;transport=udp;lr>"
	if got != want {
		t.Fatalf("P-CSCF Route = %q, want %q", got, want)
	}
}

func TestRegisterResponseKeepsServiceRouteList(t *testing.T) {
	raw := "SIP/2.0 200 OK\r\n" +
		"Via: SIP/2.0/TCP 192.0.2.10:5060;branch=z9hG4bK1\r\n" +
		"From: <sip:user@ims.example>;tag=local\r\n" +
		"To: <sip:user@ims.example>;tag=remote\r\n" +
		"Call-ID: call-1\r\nCSeq: 2 REGISTER\r\n" +
		"Service-Route: <sip:orig@scscf.ims.example;lr>\r\n" +
		"Service-Route: <sip:icscf.ims.example;lr>\r\n" +
		"Content-Length: 0\r\n\r\n"
	response, err := parseSIPResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(response.HeaderValues("Service-Route"), ",")
	want := "<sip:orig@scscf.ims.example;lr>,<sip:icscf.ims.example;lr>"
	if got != want {
		t.Fatalf("Service-Route list = %q, want %q", got, want)
	}
}
