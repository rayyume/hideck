package imscore

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/smscodec"
	"github.com/iniwex5/vowifi-go/internal/vowifi/events"
	"github.com/warthog618/sms/encoding/tpdu"
)

type captureIMSEventSubscriber struct {
	events  chan events.Event
	onEvent func(events.Event)
}

func (s *captureIMSEventSubscriber) OnIMSEvent(event events.Event) {
	if s.onEvent != nil {
		s.onEvent(event)
	}
	s.events <- event
}

func TestInboundSMSDeliversEventAndSendsRPAck(t *testing.T) {
	service, subscriber, outbound := newInboundSMSTestService(t)
	rpMR := byte(0x33)
	raw := inboundSMSRequest(t, imsSMSContentType, inboundRPData(t, rpMR, "+447700900123", "hello"))
	var response string
	if err := service.dispatchInboundSIP(raw, func(value string) error {
		response = value
		return nil
	}); err != nil {
		t.Fatalf("dispatchInboundSIP: %v", err)
	}
	if !strings.HasPrefix(response, "SIP/2.0 202") {
		t.Fatalf("SIP response = %q", response)
	}

	select {
	case event := <-subscriber.events:
		received, ok := event.(*events.EventSMSReceived)
		if !ok || !strings.HasSuffix(received.Sender, "447700900123") || received.Content != "hello" {
			t.Fatalf("received event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("inbound SMS event was not published")
	}

	request := waitForOutboundSMSControl(t, outbound)
	body, err := rawSIPBody(request)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(smscodec.BuildRPAck(rpMR)) {
		t.Fatalf("RP-ACK body = %x", body)
	}
	if !strings.HasPrefix(request, "MESSAGE sip:+447802002606@ims.example SIP/2.0") {
		t.Fatalf("RP-ACK request target = %q", strings.SplitN(request, "\r\n", 2)[0])
	}
	if got := rawSIPHeaderValue(request, "In-Reply-To"); got != "inbound-sms" {
		t.Fatalf("RP-ACK In-Reply-To = %q", got)
	}
	if got := rawSIPHeaderValue(request, "Accept-Contact"); got != "" {
		t.Fatalf("RP-ACK Accept-Contact = %q", got)
	}
	if got := rawSIPHeaderValue(request, "P-Preferred-Service"); got != "" {
		t.Fatalf("RP-ACK P-Preferred-Service = %q", got)
	}
	if got := rawSIPHeaderValue(request, "Content-Transfer-Encoding"); got != "binary" {
		t.Fatalf("RP-ACK Content-Transfer-Encoding = %q", got)
	}
	if got := rawSIPHeaderValue(request, "Request-Disposition"); got != "" {
		t.Fatalf("RP-ACK Request-Disposition = %q", got)
	}
}

func TestInboundRPAckTargetsAssertedIPSMGW(t *testing.T) {
	service, _, outbound := newInboundSMSTestService(t)
	raw := inboundSMSRequest(t, imsSMSContentType, inboundRPData(t, 0x35, "+447700900123", "hello"))
	raw = strings.Replace(raw, "From: <sip:+447802002606@ims.example>;tag=remote\r\n",
		"P-Asserted-Identity: <sip:ipsmgw@ims.example>\r\n"+
			"From: <sip:+447802002606@ims.example>;tag=remote\r\n", 1)
	if err := service.dispatchInboundSIP(raw, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	request := waitForOutboundSMSControl(t, outbound)
	if !strings.HasPrefix(request, "MESSAGE sip:ipsmgw@ims.example SIP/2.0") {
		t.Fatalf("RP-ACK request target = %q", strings.SplitN(request, "\r\n", 2)[0])
	}
	if got := rawSIPHeaderValue(request, "To"); got != "<sip:ipsmgw@ims.example>" {
		t.Fatalf("RP-ACK To = %q", got)
	}
}

func TestInboundRPAckPrefersContactOverFrom(t *testing.T) {
	service, _, outbound := newInboundSMSTestService(t)
	raw := inboundSMSRequest(t, imsSMSContentType, inboundRPData(t, 0x36, "+447700900123", "hello"))
	raw = strings.Replace(raw, "To: <sip:234102356143376@ims.example>\r\n",
		"Contact: <sip:ipsmgw-term@ims.example;transport=tcp>\r\n"+
			"To: <sip:234102356143376@ims.example>\r\n", 1)
	if err := service.dispatchInboundSIP(raw, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	request := waitForOutboundSMSControl(t, outbound)
	if !strings.HasPrefix(request, "MESSAGE sip:ipsmgw-term@ims.example;transport=tcp SIP/2.0") {
		t.Fatalf("RP-ACK request target = %q", strings.SplitN(request, "\r\n", 2)[0])
	}
}

func TestInboundRPAckDoesNotRotateURIAfter488(t *testing.T) {
	service, _, _ := newInboundSMSTestService(t)
	raw := inboundSMSRequest(t, imsSMSContentType, inboundRPData(t, 0x37, "+447700900123", "hello"))
	raw = strings.Replace(raw, "From: <sip:+447802002606@ims.example>;tag=remote\r\n",
		"P-Asserted-Identity: <sip:smsc@ims.example>\r\n"+
			"Contact: <sip:ipsmgw@ims.example>\r\n"+
			"From: <sip:+447802002606@ims.example>;tag=remote\r\n", 1)
	var targets []string
	service.transport.SetSendFn(func(request string) error {
		targets = append(targets, strings.SplitN(request, "\r\n", 2)[0])
		service.transport.DeliverResponse(registerResponseForRequest(request, 488, nil))
		return nil
	})
	if err := service.dispatchInboundSIP(raw, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(targets) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if len(targets) == 0 || service.mtAckSendOK.Load() != 0 {
		t.Fatalf("ok=%d err=%d targets=%v", service.mtAckSendOK.Load(), service.mtAckSendErr.Load(), targets)
	}
	if len(targets) == 0 || strings.Contains(targets[0], "sip:ipsmgw@ims.example") || !strings.Contains(targets[0], "sip:smsc@ims.example") {
		t.Fatalf("first RP-ACK left PAI: %v", targets)
	}
	for _, target := range targets {
		if strings.Contains(target, "sip:ipsmgw@ims.example") {
			t.Fatalf("RP-ACK rotated to Contact after 488: %v", targets)
		}
	}
}

func TestInboundRPAckCopiesQuotedAndCompactCallID(t *testing.T) {
	tests := []struct {
		name, header, want string
	}{
		{name: "quoted", header: "Call-ID: \"quoted-inbound\"\r\n", want: "quoted-inbound"},
		{name: "compact", header: "i: compact-inbound\r\n", want: "compact-inbound"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, _, outbound := newInboundSMSTestService(t)
			raw := inboundSMSRequest(t, imsSMSContentType, inboundRPData(t, 0x38, "+447700900123", "hello"))
			raw = strings.Replace(raw, "Call-ID: inbound-sms\r\n", test.header, 1)
			if err := service.dispatchInboundSIP(raw, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
			request := waitForOutboundSMSControl(t, outbound)
			if got := rawSIPHeaderValue(request, "In-Reply-To"); got != test.want {
				t.Fatalf("In-Reply-To = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInboundMalformedSMSReturnsProtocolErrorWithoutEvent(t *testing.T) {
	service, subscriber, outbound := newInboundSMSTestService(t)
	raw := inboundSMSRequest(t, imsSMSContentType, []byte{0x01, 0x44, 0x08, 0x91})
	var response string
	err := service.dispatchInboundSIP(raw, func(value string) error {
		response = value
		return nil
	})
	if err == nil || !strings.HasPrefix(response, "SIP/2.0 400") {
		t.Fatalf("dispatch error = %v, response = %q", err, response)
	}
	select {
	case event := <-subscriber.events:
		t.Fatalf("malformed SMS published event %#v", event)
	default:
	}
	request := waitForOutboundSMSControl(t, outbound)
	body, bodyErr := rawSIPBody(request)
	if bodyErr != nil {
		t.Fatal(bodyErr)
	}
	if info := smscodec.ClassifyRPDU(body); info.Kind != smscodec.RPDUKindError || info.MR != 0x44 {
		t.Fatalf("RP-ERROR body = %x", body)
	}
}

func TestInboundSMSRejectsUnsupportedContentType(t *testing.T) {
	service, subscriber, outbound := newInboundSMSTestService(t)
	raw := inboundSMSRequest(t, "text/plain", []byte("hello"))
	var response string
	if err := service.dispatchInboundSIP(raw, func(value string) error {
		response = value
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(response, "SIP/2.0 415") {
		t.Fatalf("SIP response = %q", response)
	}
	select {
	case event := <-subscriber.events:
		t.Fatalf("unsupported MESSAGE published event %#v", event)
	case request := <-outbound:
		t.Fatalf("unsupported MESSAGE sent RP control %q", request)
	default:
	}
}

func newInboundSMSTestService(t *testing.T) (*Service, *captureIMSEventSubscriber, <-chan string) {
	t.Helper()
	bus := NewEventBus()
	subscriber := &captureIMSEventSubscriber{events: make(chan events.Event, 2)}
	bus.Subscribe(subscriber)
	service, err := New(&IMSConfig{
		DeviceID: "wwan0", IMSI: "234102356143376", IMPI: "234102356143376@ims.example",
		IMPU: "sip:234102356143376@ims.example", Domain: "ims.example", SMSC: "+447802002606",
		LocalIP: net.IPv4(10, 0, 0, 2), LocalPort: 5060, Transport: "tcp", EventBus: bus,
	})
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.regState = regRegistered
	service.externalTransport = true
	service.regSession = &registerSession{
		contactUser: "registered-contact", cseq: 3,
		publicID: "sip:+447840844894@o2.co.uk", serviceRoute: "<sip:pcscf.ims.example;lr>",
		security: &securityAgreement{verifyHeader: "ipsec-3gpp;alg=hmac-sha-1-96"},
	}
	service.mu.Unlock()
	outbound := make(chan string, 2)
	service.transport.SetSendFn(func(request string) error {
		outbound <- request
		service.transport.DeliverResponse(registerResponseForRequest(request, 200, nil))
		return nil
	})
	t.Cleanup(service.StopCurrent)
	return service, subscriber, outbound
}

func waitForOutboundSMSControl(t *testing.T, outbound <-chan string) string {
	t.Helper()
	select {
	case request := <-outbound:
		return request
	case <-time.After(time.Second):
		t.Fatal("RP control MESSAGE was not sent")
		return ""
	}
}

func inboundSMSRequest(t *testing.T, contentType string, body []byte) string {
	t.Helper()
	transactionID := 1
	if len(body) > 1 {
		transactionID = int(body[1]) + 1
	}
	return fmt.Sprintf("MESSAGE sip:234102356143376@ims.example SIP/2.0\r\n"+
		"Via: SIP/2.0/TCP 10.0.0.1:5060;branch=z9hG4bK-inbound-%d\r\n"+
		"From: <sip:+447802002606@ims.example>;tag=remote\r\n"+
		"To: <sip:234102356143376@ims.example>\r\n"+
		"Call-ID: inbound-sms\r\nCSeq: %d MESSAGE\r\n"+
		"Content-Type: %s\r\nContent-Length: %d\r\n\r\n%s",
		transactionID, transactionID, contentType, len(body), body)
}

func inboundRPData(t *testing.T, mr byte, sender, text string) []byte {
	t.Helper()
	originator := smscodec.EncodeAddress("+447802002606")
	tpduBytes := deliverTPDU(t, sender, text)
	body := []byte{0x01, mr}
	body = append(body, originator...)
	body = append(body, 0x00, byte(len(tpduBytes)))
	return append(body, tpduBytes...)
}

func deliverTPDU(t *testing.T, sender, text string) []byte {
	t.Helper()
	pdu, err := tpdu.NewDeliver(tpdu.WithOA(tpdu.NewAddress(tpdu.FromNumber(sender))))
	if err != nil {
		t.Fatal(err)
	}
	pdu.SetPID(0)
	pdu.SetDCS(0)
	userData, _, _ := tpdu.EncodeUserData([]byte(text))
	pdu.SetUD(userData)
	raw, err := pdu.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
