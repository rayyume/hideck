package imscore

import (
	"bufio"
	"context"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseMWISummary(t *testing.T) {
	summary := parseMWISummary("Messages-Waiting: yes\r\n" +
		"Message-Account: sip:user@example\r\n" +
		"Voice-Message: 2/1 (1/0)\r\n")
	if !summary.waiting || summary.account != "sip:user@example" ||
		summary.voiceNew != 2 || summary.voiceOld != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	if parseMWISummary("Messages-Waiting: no\r\n").waiting {
		t.Fatal("no waiting was treated as yes")
	}
}

func TestBuildMWISubscriptionUsesMessageSummaryEvent(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	service.activateProtectedRegistrationTCP(client)
	t.Cleanup(func() { _ = server.Close() })

	requests := make(chan string, 1)
	errorsSeen := make(chan error, 1)
	go serveSubscriptionResponses(server, requests, errorsSeen, 1)
	if err := service.sendSubscribeMWI(context.Background()); err != nil {
		t.Fatalf("sendSubscribeMWI: %v", err)
	}
	if err := <-errorsSeen; err != nil {
		t.Fatal(err)
	}
	request := <-requests
	if rawSIPHeaderValue(request, "Event") != mwiEventPackage {
		t.Fatalf("Event = %q", rawSIPHeaderValue(request, "Event"))
	}
	if rawSIPHeaderValue(request, "Accept") != mwiContentType {
		t.Fatalf("Accept = %q", rawSIPHeaderValue(request, "Accept"))
	}
	if rawSIPHeaderValue(request, "Expires") != "3600" {
		t.Fatalf("Expires = %q", rawSIPHeaderValue(request, "Expires"))
	}
	if !strings.HasPrefix(request, "SUBSCRIBE sip:+447840844894@o2.co.uk SIP/2.0") {
		t.Fatalf("request line = %q", strings.SplitN(request, "\r\n", 2)[0])
	}
	if !service.hasMWISubscriptionDialog() {
		t.Fatal("MWI dialog was not learned")
	}
}

func TestMWINotifyUpdatesSummaryWithoutTouchingRegDialog(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.subscriptionDialog = registrationSubscriptionDialog{
		callID: "reg-call", localTag: "reg-local", remoteTag: "reg-remote",
	}
	body := "Messages-Waiting: yes\r\nMessage-Account: sip:user@example\r\nVoice-Message: 3/0\r\n"
	raw := mwiNotifyRequest(body)
	replied := make(chan string, 1)
	if err := service.dispatchInboundSIP(raw, func(response string) error {
		replied <- response
		return nil
	}); err != nil {
		t.Fatalf("dispatchInboundSIP: %v", err)
	}
	if !strings.HasPrefix(<-replied, "SIP/2.0 200 OK") {
		t.Fatal("MWI NOTIFY was not acknowledged")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !service.mwiMessagesWaiting {
		time.Sleep(time.Millisecond)
	}
	service.mu.RLock()
	waiting := service.mwiMessagesWaiting
	regDialog := service.subscriptionDialog
	service.mu.RUnlock()
	if !waiting {
		t.Fatal("MWI waiting flag was not set")
	}
	if regDialog.callID != "reg-call" || regDialog.remoteTag != "reg-remote" {
		t.Fatalf("reg dialog mutated: %+v", regDialog)
	}
}

func TestIMSMaintenancePrioritizesDueMWIAfterRegistrationSubscribe(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	service.activateProtectedRegistrationTCP(client)
	t.Cleanup(func() { _ = server.Close() })
	now := time.Date(2026, 8, 8, 18, 0, 0, 0, time.UTC)
	service.mu.Lock()
	service.registrationRefreshAt = now.Add(time.Hour)
	service.subscriptionRefreshAt = now.Add(time.Hour)
	service.mwiSubscriptionRefreshAt = now
	service.mwiSubscriptionClosed = false
	service.lastPingAt = now.Add(-time.Hour)
	service.mu.Unlock()
	if got := service.nextIMSMaintenanceAction(now); got != imsMaintenanceSubscribeMWI {
		t.Fatalf("maintenance action = %d, want MWI subscribe", got)
	}
}

func TestZeroMWIRefreshDoesNotStealKeepalive(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	service.activateProtectedRegistrationTCP(client)
	t.Cleanup(func() { _ = server.Close() })
	now := time.Now()
	service.mu.Lock()
	service.registrationRefreshAt = now.Add(time.Hour)
	service.subscriptionRefreshAt = now.Add(time.Hour)
	service.mwiSubscriptionRefreshAt = time.Time{}
	service.mu.Unlock()
	if action := service.nextIMSMaintenanceAction(now.Add(service.keepaliveInterval)); action != imsMaintenanceKeepalive {
		t.Fatalf("action = %d, want keepalive", action)
	}
}

func TestBuildRegisterIncludesFeatureCaps(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	request := service.buildRegister(&registerSession{callID: "call-1", fromTag: "tag-1", cseq: 1}, "")
	if got := rawSIPHeaderValue(request, "Feature-Caps"); got != registerFeatureCapsHeader {
		t.Fatalf("Feature-Caps = %q", got)
	}
}

func TestInboundOPTIONSAcceptsMWI(t *testing.T) {
	service := &Service{cfg: &IMSConfig{}}
	got, err := service.buildInboundOPTIONSResponse(optionsRequest("opt-mwi"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rawSIPHeaderValue(got, "Accept"), "application/simple-message-summary") {
		t.Fatalf("Accept = %q", rawSIPHeaderValue(got, "Accept"))
	}
}

func mwiNotifyRequest(body string) string {
	return "NOTIFY sip:user@example SIP/2.0\r\n" +
		"Via: SIP/2.0/TCP 192.0.2.1:6060;branch=z9hG4bK-mwi\r\n" +
		"From: <sip:server@example>;tag=mwi-server\r\n" +
		"To: <sip:user@example>;tag=mwi-client\r\n" +
		"Call-ID: mwi-call\r\n" +
		"CSeq: 1 NOTIFY\r\n" +
		"Event: message-summary\r\n" +
		"Subscription-State: active;expires=3600\r\n" +
		"Content-Type: application/simple-message-summary\r\n" +
		"Content-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
}

func TestMWISubscribe481RetriesAsInitial(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	service.activateProtectedRegistrationTCP(client)
	t.Cleanup(func() { _ = server.Close() })
	service.mwiSubscriptionDialog = registrationSubscriptionDialog{
		callID: "mwi-old", localTag: "local", remoteTag: "remote", cseq: 4,
	}
	requests := make(chan string, 2)
	errorsSeen := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		for i := 0; i < 2; i++ {
			request, err := readSIPStreamMessage(reader)
			if err != nil {
				errorsSeen <- err
				return
			}
			requests <- request
			status := 200
			if i == 0 {
				status = 481
			}
			if _, err = io.WriteString(server, subscriptionWireResponse(request, status, "Expires: 120\r\n")); err != nil {
				errorsSeen <- err
				return
			}
		}
		errorsSeen <- nil
	}()
	if err := service.sendSubscribeMWI(context.Background()); err != nil {
		t.Fatalf("sendSubscribeMWI: %v", err)
	}
	if err := <-errorsSeen; err != nil {
		t.Fatal(err)
	}
	first, second := <-requests, <-requests
	if sipAddressTag(rawSIPHeaderValue(first, "To")) != "remote" {
		t.Fatalf("in-dialog SUBSCRIBE To = %q", rawSIPHeaderValue(first, "To"))
	}
	if sipAddressTag(rawSIPHeaderValue(second, "To")) != "" {
		t.Fatalf("481 retry kept To tag: %q", rawSIPHeaderValue(second, "To"))
	}
}
