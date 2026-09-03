package imscore

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestProtectedSIPStreamClosureInvalidatesRegistration(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	defer server.Close()
	service.activateProtectedRegistrationTCP(client)
	if readiness := service.SMSReadiness(); !readiness.TransportReady {
		t.Fatalf("initial transport readiness = %+v", readiness)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}

	waitForRegistrationState(t, service, regFailed)
	readiness := service.SMSReadiness()
	if readiness.Ready || readiness.TransportReady {
		t.Fatalf("readiness after close = %+v", readiness)
	}
	select {
	case err := <-service.RegistrationErrors():
		if !strings.Contains(err.Error(), "registration SIP stream closed") {
			t.Fatalf("registration error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("registration stream closure was not reported")
	}
}

func TestPreviousSIPStreamClosureDoesNotInvalidateReplacement(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	oldClient, oldServer := net.Pipe()
	service.activateProtectedRegistrationTCP(oldClient)
	newClient, newServer := net.Pipe()
	service.activateProtectedRegistrationTCP(newClient)
	t.Cleanup(func() {
		_ = oldServer.Close()
		_ = newServer.Close()
	})

	if err := oldServer.Close(); err != nil {
		t.Fatal(err)
	}
	request := "OPTIONS sip:test@example SIP/2.0\r\nCall-ID: replacement\r\nCSeq: 1 OPTIONS\r\nContent-Length: 0\r\n\r\n"
	done := make(chan error, 1)
	go func() {
		received, err := readSIPStreamMessage(bufio.NewReader(newServer))
		if err == nil && received != request {
			err = errors.New("replacement stream received unexpected request")
		}
		done <- err
	}()
	if err := service.transport.Send(request); err != nil {
		t.Fatalf("send on replacement stream: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if service.RegState() != regRegistered || !service.SMSReadiness().TransportReady {
		t.Fatalf("replacement registration was invalidated: %+v", service.SMSReadiness())
	}
}

func TestIMSKeepaliveUsesRegisteredProtectedProfile(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	defer server.Close()
	service.activateProtectedRegistrationTCP(client)
	done := make(chan error, 1)
	go func() {
		request, err := readSIPStreamMessage(bufio.NewReader(server))
		if err != nil {
			done <- err
			return
		}
		if !strings.HasPrefix(request, "OPTIONS sip:[2001:db8::20]:6060;transport=tcp SIP/2.0") {
			done <- errors.New("keepalive did not target the registered P-CSCF endpoint")
			return
		}
		if rawSIPHeaderValue(request, "Security-Verify") == "" ||
			rawSIPHeaderValue(request, "Route") == "" ||
			rawSIPHeaderValue(request, "P-Access-Network-Info") == "" ||
			rawSIPHeaderValue(request, "P-Preferred-Identity") == "" {
			done <- errors.New("keepalive omitted registered security routing")
			return
		}
		if rawSIPHeaderValue(request, "Contact") != "" {
			done <- errors.New("keepalive included Contact")
			return
		}
		if rawSIPHeaderValue(request, "Supported") != imsProtectedKeepaliveSupported {
			done <- errors.New("keepalive Supported header differs from v1.5.5")
			return
		}
		if !strings.Contains(rawSIPHeaderValue(request, "From"), ";tag=register-tag") ||
			rawSIPHeaderValue(request, "CSeq") != "6 OPTIONS" {
			done <- errors.New("keepalive did not reuse REGISTER dialog identity and shared CSeq")
			return
		}
		branch, branchErr := parseTopViaBranch(rawSIPHeaderValue(request, "Via"))
		if branchErr != nil || !strings.HasPrefix(branch, "z9hG4bK.") || len(branch) != len("z9hG4bK.")+20 ||
			len(rawSIPHeaderValue(request, "Call-ID")) != 20 {
			done <- errors.New("keepalive transaction identifiers differ from v1.5.5")
			return
		}
		if rawSIPHeaderValue(request, "Require") != "sec-agree" ||
			rawSIPHeaderValue(request, "Proxy-Require") != "sec-agree" {
			done <- errors.New("keepalive omitted negotiated sec-agree requirements")
			return
		}
		_, err = io.WriteString(server, registerWireResponse(request, 200, ""))
		done <- err
	}()
	if err := service.sendOPTIONSKeepalive(); err != nil {
		t.Fatalf("sendOPTIONSKeepalive: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestProtectedTCPKeepaliveSendsRFC5626CRLF(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	defer server.Close()
	service.activateProtectedRegistrationTCP(client)
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4)
		n, err := io.ReadFull(server, buf)
		if err != nil {
			done <- err
			return
		}
		if n != 4 || string(buf) != "\r\n\r\n" {
			done <- errors.New("protected TCP keepalive did not send RFC 5626 double CRLF")
			return
		}
		_, err = io.WriteString(server, "\r\n")
		done <- err
	}()
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("sendIMSKeepalive: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if service.reRegisterPending.Load() {
		t.Fatal("CRLF keepalive scheduled a REGISTER refresh")
	}
}

// RFC 5626 4.4.1 keepalives belong to the client that opened the flow, and
// port-s is dialed to us by the P-CSCF. Pinging it drew no pong across 582
// writes over 39m and never kept the flow alive, so nothing may go out on it.
func TestPortSFlowGetsNoCRLFKeepalive(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	regClient, regServer := net.Pipe()
	defer regServer.Close()
	service.activateProtectedRegistrationTCP(regClient)
	pushClient, pushServer := net.Pipe()
	defer pushServer.Close()
	if !service.trackProtectedConnection(pushClient) {
		t.Fatal("trackProtectedConnection")
	}
	pushed := make(chan []byte, 1)
	go func() {
		buf := make([]byte, 4)
		if n, err := pushServer.Read(buf); err == nil {
			pushed <- buf[:n]
		}
	}()
	go func() {
		buf := make([]byte, 4)
		_, _ = io.ReadFull(regServer, buf)
	}()
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("sendIMSKeepalive: %v", err)
	}
	select {
	case got := <-pushed:
		t.Fatalf("port-s received %q, want nothing", got)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestIMSMaintenanceChoosesEarliestRegistrationOrKeepaliveDeadline(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	now := time.Date(2026, 8, 8, 18, 0, 0, 0, time.UTC)
	service.mu.Lock()
	service.registrationRefreshAt = now.Add(30 * time.Second)
	service.subscriptionRefreshAt = now.Add(time.Hour)
	service.lastPingAt = now.Add(-45 * time.Second)
	service.keepaliveInterval = time.Minute
	service.mu.Unlock()

	if got, want := service.computeNextWakeTime(now), now.Add(5*time.Second); !got.Equal(want) {
		t.Fatalf("next poll wake = %s, want %s", got, want)
	}
	if got := service.nextIMSMaintenanceAction(now.Add(15 * time.Second)); got != imsMaintenanceKeepalive {
		t.Fatalf("action at keepalive deadline = %d, want keepalive", got)
	}
	if got := service.nextIMSMaintenanceAction(now.Add(30 * time.Second)); got != imsMaintenanceRefresh {
		t.Fatalf("action at refresh deadline = %d, want refresh", got)
	}
}

func TestIMSMaintenancePrioritizesDueSubscriptionBeforeKeepalive(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	service.activateProtectedRegistrationTCP(client)
	t.Cleanup(func() { _ = server.Close() })
	now := time.Date(2026, 8, 8, 18, 0, 0, 0, time.UTC)
	service.mu.Lock()
	service.registrationRefreshAt = now.Add(time.Hour)
	service.subscriptionRefreshAt = now
	service.lastPingAt = now.Add(-time.Hour)
	service.mu.Unlock()
	if got := service.nextIMSMaintenanceAction(now); got != imsMaintenanceSubscribe {
		t.Fatalf("maintenance action = %d, want subscribe", got)
	}
}

func TestProtectedTCPMaintenanceKeepsSIPOptionsEnabled(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	service.activateProtectedRegistrationTCP(client)
	t.Cleanup(func() { _ = server.Close() })

	now := time.Now()
	service.mu.Lock()
	service.registrationRefreshAt = now.Add(time.Hour)
	service.subscriptionRefreshAt = now.Add(time.Hour)
	service.mu.Unlock()
	if action := service.nextIMSMaintenanceAction(now.Add(service.keepaliveInterval)); action != imsMaintenanceKeepalive {
		t.Fatalf("protected TCP maintenance action = %d, want keepalive", action)
	}
}

func TestInboundSIPTrafficDefersKeepaliveAndResetsFailures(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.lastPingAt = time.Now().Add(-time.Minute)
	service.mu.Unlock()
	service.pingFailCount.Store(2)
	service.lastPingOK.Store(false)

	raw := "SIP/2.0 200 OK\r\nCall-ID: traffic\r\nCSeq: 9 OPTIONS\r\nContent-Length: 0\r\n\r\n"
	before := time.Now()
	if err := service.dispatchInboundSIP(raw, nil); err != nil {
		t.Fatalf("dispatchInboundSIP: %v", err)
	}
	service.mu.RLock()
	lastTrafficAt := service.lastPingAt
	service.mu.RUnlock()
	if lastTrafficAt.Before(before) {
		t.Fatalf("last traffic = %s, want at or after %s", lastTrafficAt, before)
	}
	if status := service.StatusCurrent(); status.PingFailCount != 0 || !status.LastPingOK {
		t.Fatalf("ping status = count %d ok %v, want count 0 and ok", status.PingFailCount, status.LastPingOK)
	}
}

func TestThreeKeepaliveFailuresScheduleRegistrationRefresh(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.registrationRefreshAt = time.Now().Add(time.Hour)
	service.sipOutboundKeepalive = true
	service.mu.Unlock()
	keepaliveErr := errTCPCRLFPongTimeout
	for attempt := 1; attempt <= imsKeepaliveFailureLimit; attempt++ {
		service.recordIMSKeepaliveResult(keepaliveErr, time.Now())
	}
	if service.RegState() != regRegistered {
		t.Fatalf("registration state = %s, want %s", service.RegState(), regRegistered)
	}
	if !service.reRegisterPending.Load() {
		t.Fatal("keepalive failures did not mark registration refresh pending")
	}
	if action := service.nextIMSMaintenanceAction(time.Now()); action != imsMaintenanceRefresh {
		t.Fatalf("maintenance action = %d, want refresh", action)
	}
	select {
	case err := <-service.RegistrationErrors():
		t.Fatalf("keepalive failures tore down runtime before REGISTER refresh: %v", err)
	default:
	}
}

func TestOPTIONSKeepaliveFailuresDoNotScheduleRegistrationRefresh(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.registrationRefreshAt = time.Now().Add(time.Hour)
	service.mu.Unlock()
	keepaliveErr := fmt.Errorf("%w: timeout", errOPTIONSKeepalive)
	for attempt := 1; attempt <= imsKeepaliveFailureLimit; attempt++ {
		service.recordIMSKeepaliveResult(keepaliveErr, time.Now())
	}
	if service.reRegisterPending.Load() {
		t.Fatal("OPTIONS keepalive failures scheduled a REGISTER refresh")
	}
}

func TestTCPCRLFKeepaliveDoesNotWaitForPongWithoutOutbound(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	defer server.Close()
	service.activateProtectedRegistrationTCP(client)
	service.tcpCRLFPongWait = 30 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(server, buf); err != nil {
			done <- err
			return
		}
		if string(buf) != "\r\n\r\n" {
			done <- fmt.Errorf("ping = %q", buf)
			return
		}
		done <- nil
	}()
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("unnegotiated CRLF keepalive: %v", err)
	}
	if waitErr := <-done; waitErr != nil {
		t.Fatal(waitErr)
	}
	if service.reRegisterPending.Load() {
		t.Fatal("unnegotiated CRLF keepalive scheduled a REGISTER refresh")
	}
}

func TestTCPCRLFKeepaliveWaitsForPongWhenOutboundNegotiated(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	defer server.Close()
	service.activateProtectedRegistrationTCP(client)
	service.mu.Lock()
	service.sipOutboundKeepalive = true
	service.mu.Unlock()
	service.tcpCRLFPongWait = 30 * time.Millisecond
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4)
		if _, err := io.ReadFull(server, buf); err != nil {
			done <- err
			return
		}
		done <- nil
	}()
	err := service.sendIMSKeepalive()
	if !errors.Is(err, errTCPCRLFPongTimeout) {
		t.Fatalf("missing pong error = %v", err)
	}
	if waitErr := <-done; waitErr != nil {
		t.Fatal(waitErr)
	}
}

func TestUnnegotiatedPongTimeoutDoesNotScheduleRegistrationRefresh(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.registrationRefreshAt = time.Now().Add(time.Hour)
	service.mu.Unlock()
	for attempt := 1; attempt <= imsKeepaliveFailureLimit; attempt++ {
		service.recordIMSKeepaliveResult(errTCPCRLFPongTimeout, time.Now())
	}
	if service.reRegisterPending.Load() {
		t.Fatal("unnegotiated pong timeout scheduled a REGISTER refresh")
	}
}

func TestRegisterFlowNegotiationEnablesCRLFPong(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	outbound := &sipResponse{
		StatusCode: 200,
		Headers: map[string]string{
			"Require": "outbound",
			"Via":     "SIP/2.0/TCP pcscf.example;branch=z9hG4bK;keep=120",
		},
	}
	service.logRegisterFlowNegotiation(outbound)
	if !service.expectsCRLFPong() {
		t.Fatal("Require: outbound did not enable CRLF pong")
	}
	plain := &sipResponse{StatusCode: 200, Headers: map[string]string{"Via": "SIP/2.0/TCP pcscf.example;branch=z9hG4bK"}}
	service.logRegisterFlowNegotiation(plain)
	if service.expectsCRLFPong() {
		t.Fatal("REGISTER without outbound still expected a CRLF pong")
	}
	unvalued := &sipResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Via": "SIP/2.0/TCP pcscf.example;branch=z9hG4bK;keep"},
	}
	service.logRegisterFlowNegotiation(unvalued)
	if service.expectsCRLFPong() {
		t.Fatal("RFC 6223 unvalued Via keep expected a CRLF pong")
	}
	valued := &sipResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Via": "SIP/2.0/TCP pcscf.example;branch=z9hG4bK;keep=120"},
	}
	service.logRegisterFlowNegotiation(valued)
	if !service.expectsCRLFPong() {
		t.Fatal("RFC 6223 keep interval did not enable CRLF pong")
	}
}

func TestRegistrationRefreshDelayFollowsTS24229(t *testing.T) {
	tests := []struct {
		expires time.Duration
		want    time.Duration
	}{
		{expires: time.Hour, want: 50 * time.Minute},
		{expires: 1200 * time.Second, want: 600 * time.Second},
		{expires: 600 * time.Second, want: 300 * time.Second},
		{expires: time.Second, want: 500 * time.Millisecond},
	}
	for _, test := range tests {
		if got := registrationRefreshDelay(test.expires); got != test.want {
			t.Fatalf("registrationRefreshDelay(%s) = %s, want %s", test.expires, got, test.want)
		}
	}
	if got := subscriptionRefreshDelay(time.Hour); got != 59*time.Minute {
		t.Fatalf("subscription refresh must stay on the 60s advance: %s", got)
	}
}

func newProtectedKeepaliveTestService(t *testing.T) *Service {
	t.Helper()
	service, err := New(&IMSConfig{
		DeviceID: "wwan0", Domain: "ims.example", SMSC: "+447802002606",
		LocalIP: net.IPv4(10, 0, 0, 2), Transport: "tcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	service.regState = regRegistered
	service.smsReceiverReady = true
	service.protectedClientPort = 16082
	service.protectedServerPort = 16083
	service.regSession = &registerSession{
		contactUser: "registered-contact", fromTag: "register-tag", cseq: 3,
		publicID: "sip:+447840844894@o2.co.uk", serviceRoute: "<sip:pcscf.ims.example;lr>",
		security: &securityAgreement{
			verifyHeader: "ipsec-3gpp;alg=hmac-sha-1-96",
			server:       &securityMechanism{PortS: 6060},
		},
	}
	service.registrationRemote = &net.UDPAddr{IP: net.ParseIP("2001:db8::20"), Port: 5060}
	service.mu.Unlock()
	t.Cleanup(service.StopCurrent)
	return service
}

func waitForRegistrationState(t *testing.T, service *Service, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if service.RegState() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("registration state = %s, want %s", service.RegState(), want)
}
