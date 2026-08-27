package imscore

import (
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/emergency"
)

func TestBuildEmergencyREGISTERAddsSOSContactAndKeepsPANI(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	request, err := service.BuildEmergencyREGISTER(false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(request, "REGISTER sip:ims.example SIP/2.0") {
		t.Fatalf("request-line = %q", strings.SplitN(request, "\r\n", 2)[0])
	}
	contact := rawSIPHeaderValue(request, "Contact")
	if !strings.Contains(contact, ";sos") {
		t.Fatalf("Contact missing sos: %q", contact)
	}
	if got := rawSIPHeaderValue(request, "P-Access-Network-Info"); got == "" {
		t.Fatal("emergency REGISTER omitted PANI")
	}
	if strings.Contains(rawSIPHeaderValue(request, "From"), "anonymous") {
		t.Fatalf("authenticated emergency used anonymous From: %q", request)
	}
}

func TestBuildAnonymousEmergencyREGISTEROmitsAuthorization(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	request, err := service.BuildEmergencyREGISTER(true)
	if err != nil {
		t.Fatal(err)
	}
	if got := rawSIPHeaderValue(request, "From"); !strings.Contains(got, `"Anonymous"`) ||
		!strings.Contains(got, emergency.AnonymousIMPU) {
		t.Fatalf("From = %q", got)
	}
	if got := rawSIPHeaderValue(request, "To"); got != "<"+emergency.AnonymousIMPU+">" {
		t.Fatalf("To = %q", got)
	}
	if got := rawSIPHeaderValue(request, "Authorization"); got != "" {
		t.Fatalf("anonymous REGISTER sent Authorization = %q", got)
	}
	if !strings.Contains(rawSIPHeaderValue(request, "Contact"), ";sos") {
		t.Fatal("anonymous emergency REGISTER omitted sos")
	}
}

func TestNormalREGISTERContactDoesNotIncludeSOS(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	session := service.emergencyRegisterSession()
	request := service.buildRegister(session, "")
	if strings.Contains(strings.ToLower(rawSIPHeaderValue(request, "Contact")), ";sos") {
		t.Fatalf("normal REGISTER Contact includes sos: %q", request)
	}
}
