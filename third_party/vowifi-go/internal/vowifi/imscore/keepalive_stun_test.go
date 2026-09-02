package imscore

import (
	"errors"
	"net"
	"testing"
	"time"
)

func TestUDPKeepaliveSendsSTUNBindingNotOPTIONS(t *testing.T) {
	registrar, service := newUDPKeepaliveTestService(t)
	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 1500)
		_ = registrar.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := registrar.ReadFromUDP(buf)
		if err != nil {
			done <- err
			return
		}
		if !isSTUNMessage(buf[:n]) {
			done <- errors.New("UDP keepalive did not send STUN")
			return
		}
		msg, parseErr := parseSTUNMessage(buf[:n])
		if parseErr != nil || msg.Type != stunBindingRequest {
			done <- errors.New("UDP keepalive is not a STUN Binding Request")
			return
		}
		response := buildSTUNBindingSuccess(msg.TxID, &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 5060})
		_, err = registrar.WriteToUDP(response, addr)
		done <- err
	}()
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("sendIMSKeepalive: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if service.reRegisterPending.Load() {
		t.Fatal("successful STUN keepalive scheduled REGISTER refresh")
	}
}

func TestTCPKeepaliveDoesNotSendSTUN(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	defer server.Close()
	service.activateProtectedRegistrationTCP(client)
	buf := make([]byte, 16)
	done := make(chan error, 1)
	go func() {
		n, err := server.Read(buf)
		if err != nil {
			done <- err
			return
		}
		if isSTUNMessage(buf[:n]) {
			done <- errors.New("TCP keepalive sent STUN")
			return
		}
		done <- nil
	}()
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("sendIMSKeepalive: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestUDPSTUNMappedAddressChangeFailsFlow(t *testing.T) {
	registrar, service := newUDPKeepaliveTestService(t)
	reply := func(addr *net.UDPAddr) {
		buf := make([]byte, 1500)
		_ = registrar.SetReadDeadline(time.Now().Add(time.Second))
		n, remote, err := registrar.ReadFromUDP(buf)
		if err != nil {
			t.Error(err)
			return
		}
		msg, err := parseSTUNMessage(buf[:n])
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = registrar.WriteToUDP(buildSTUNBindingSuccess(msg.TxID, addr), remote)
	}
	go reply(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 5060})
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("first STUN: %v", err)
	}
	go reply(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 9), Port: 5060})
	err := service.sendIMSKeepalive()
	if !errors.Is(err, errSTUNMappedChanged) {
		t.Fatalf("mapped-address change error = %v", err)
	}
}

func TestUDPSTUNMappedAddressChangeTriesMOBIKEBeforeReregister(t *testing.T) {
	registrar, service := newUDPKeepaliveTestService(t)
	var sawOld, sawNew net.IP
	service.cfg.OnLocalAddressChange = func(oldIP, newIP net.IP) error {
		sawOld, sawNew = append(net.IP(nil), oldIP...), append(net.IP(nil), newIP...)
		return nil
	}
	reply := func(addr *net.UDPAddr) {
		buf := make([]byte, 1500)
		_ = registrar.SetReadDeadline(time.Now().Add(time.Second))
		n, remote, err := registrar.ReadFromUDP(buf)
		if err != nil {
			t.Error(err)
			return
		}
		msg, err := parseSTUNMessage(buf[:n])
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = registrar.WriteToUDP(buildSTUNBindingSuccess(msg.TxID, addr), remote)
	}
	go reply(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 5060})
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("first STUN: %v", err)
	}
	go reply(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 9), Port: 5060})
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("MOBIKE mapped-address change = %v", err)
	}
	if !sawOld.Equal(net.IPv4(192, 0, 2, 1)) || !sawNew.Equal(net.IPv4(192, 0, 2, 9)) {
		t.Fatalf("MOBIKE addrs old=%v new=%v", sawOld, sawNew)
	}
	service.recordIMSKeepaliveResult(nil, time.Now())
	if service.reRegisterPending.Load() {
		t.Fatal("successful MOBIKE scheduled REGISTER refresh")
	}
}

func TestUDPSTUNMappedAddressChangeFallsBackWhenMOBIKEFails(t *testing.T) {
	registrar, service := newUDPKeepaliveTestService(t)
	service.cfg.OnLocalAddressChange = func(net.IP, net.IP) error {
		return errors.New("mobike rejected")
	}
	reply := func(addr *net.UDPAddr) {
		buf := make([]byte, 1500)
		_ = registrar.SetReadDeadline(time.Now().Add(time.Second))
		n, remote, err := registrar.ReadFromUDP(buf)
		if err != nil {
			t.Error(err)
			return
		}
		msg, err := parseSTUNMessage(buf[:n])
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = registrar.WriteToUDP(buildSTUNBindingSuccess(msg.TxID, addr), remote)
	}
	go reply(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 5060})
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatalf("first STUN: %v", err)
	}
	go reply(&net.UDPAddr{IP: net.IPv4(192, 0, 2, 9), Port: 5060})
	err := service.sendIMSKeepalive()
	if !errors.Is(err, errSTUNMappedChanged) {
		t.Fatalf("failed MOBIKE error = %v", err)
	}
}

func TestSTUNKeepaliveFailuresScheduleRegistrationRefresh(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.registrationRefreshAt = time.Now().Add(time.Hour)
	service.mu.Unlock()
	for attempt := 1; attempt <= imsKeepaliveFailureLimit; attempt++ {
		service.recordIMSKeepaliveResult(errSTUNKeepaliveTimeout, time.Now())
	}
	if !service.reRegisterPending.Load() {
		t.Fatal("STUN keepalive failures did not mark registration refresh pending")
	}
}

func TestUDPKeepaliveIntervalWithoutFlowTimerIs24To29(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.registrationTCP = nil
	service.registrationIO = nil
	service.registrationTransport = "udp"
	service.flowTimer = 0
	service.stunKeepaliveInterval = 0
	service.keepaliveInterval = time.Minute
	service.mu.Unlock()
	for range 20 {
		service.mu.RLock()
		got := service.keepaliveIntervalLocked()
		service.mu.RUnlock()
		if got < imsUDPSTUNMinInterval || got > imsUDPSTUNMaxInterval {
			t.Fatalf("UDP STUN interval = %s, want 24-29s", got)
		}
	}
}

func TestFlowTimerSetsUDPKeepaliveInterval(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.logRegisterFlowNegotiation(&sipResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Flow-Timer": "40"},
	})
	service.mu.Lock()
	service.registrationTCP = nil
	service.registrationTransport = "udp"
	got := service.keepaliveIntervalLocked()
	flow := service.flowTimer
	service.mu.Unlock()
	if flow != 40*time.Second {
		t.Fatalf("flow timer = %s", flow)
	}
	min, max := 40*time.Second*80/100, 40*time.Second
	if got < min || got > max {
		t.Fatalf("Flow-Timer interval = %s, want %s-%s", got, min, max)
	}
}

func TestRFC5626KeepaliveIdle(t *testing.T) {
	if got, want := rfc5626KeepaliveIdle(0), 114*time.Second; got != want {
		t.Fatalf("default idle = %s, want %s", got, want)
	}
	if got, want := rfc5626KeepaliveIdle(120*time.Second), 114*time.Second; got != want {
		t.Fatalf("120s idle = %s, want %s", got, want)
	}
	if got, want := rfc5626KeepaliveIdle(40*time.Second), 38*time.Second; got != want {
		t.Fatalf("40s idle = %s, want %s", got, want)
	}
}

func TestTCPKeepaliveIntervalHonorsFlowTimer(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	client, server := net.Pipe()
	defer server.Close()
	service.activateProtectedRegistrationTCP(client)
	service.logRegisterFlowNegotiation(&sipResponse{
		StatusCode: 200,
		Headers:    map[string]string{"Flow-Timer": "40"},
	})
	service.mu.RLock()
	got := service.keepaliveIntervalLocked()
	flow := service.flowTimer
	service.mu.RUnlock()
	if flow != 40*time.Second {
		t.Fatalf("flow timer = %s", flow)
	}
	want := rfc5626KeepaliveIdle(40 * time.Second)
	if got != want {
		t.Fatalf("TCP CRLF interval = %s, want RFC 5626 %s", got, want)
	}
}

func TestTCPSocketKeepaliveIdleDefaultsTo30s(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	if got, want := service.tcpSocketKeepaliveIdle(), 30*time.Second; got != want {
		t.Fatalf("socket keepalive idle = %s, want %s", got, want)
	}
	service.cfg.TCPKeepaliveSeconds = 45
	if got, want := service.tcpSocketKeepaliveIdle(), 45*time.Second; got != want {
		t.Fatalf("configured socket keepalive idle = %s, want %s", got, want)
	}
}

func TestTCPKeepaliveIntervalDefaultsToRFC5626(t *testing.T) {
	service := newProtectedKeepaliveTestService(t)
	service.mu.Lock()
	service.keepaliveInterval = 0
	service.flowTimer = 0
	got := service.keepaliveIntervalLocked()
	service.mu.Unlock()
	want := rfc5626KeepaliveIdle(0)
	if got != want {
		t.Fatalf("TCP keepalive interval = %s, want RFC 5626 %s", got, want)
	}
}

func TestInboundUDPDemuxesSTUNFromSIP(t *testing.T) {
	registrar, service := newUDPKeepaliveTestService(t)
	go func() {
		buf := make([]byte, 1500)
		_ = registrar.SetReadDeadline(time.Now().Add(time.Second))
		n, addr, err := registrar.ReadFromUDP(buf)
		if err != nil {
			return
		}
		msg, err := parseSTUNMessage(buf[:n])
		if err != nil {
			return
		}
		_, _ = registrar.WriteToUDP(buildSTUNBindingSuccess(msg.TxID, &net.UDPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 3478}), addr)
	}()
	if err := service.sendIMSKeepalive(); err != nil {
		t.Fatal(err)
	}
}

func newUDPKeepaliveTestService(t *testing.T) (*net.UDPConn, *Service) {
	t.Helper()
	registrar, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	client, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		_ = registrar.Close()
		t.Fatal(err)
	}
	service, err := New(&IMSConfig{
		DeviceID: "wwan0", Domain: "ims.example", SMSC: "+447802002606",
		LocalIP: net.IPv4(127, 0, 0, 1), LocalPort: client.LocalAddr().(*net.UDPAddr).Port,
		Transport: "udp",
	})
	if err != nil {
		_ = client.Close()
		_ = registrar.Close()
		t.Fatal(err)
	}
	remote := registrar.LocalAddr().(*net.UDPAddr)
	service.mu.Lock()
	service.regState = regRegistered
	service.smsReceiverReady = true
	service.registrationIO = client
	service.registrationRemote = cloneUDPAddr(remote)
	service.registrationTransport = "udp"
	service.registrationTCP = nil
	service.stunRTO = 15 * time.Millisecond
	service.stunRc = 3
	service.regSession = &registerSession{
		contactUser: "registered-contact", fromTag: "register-tag", cseq: 3,
		publicID: "sip:+447840844894@o2.co.uk",
	}
	service.mu.Unlock()
	service.networkDone.Add(1)
	go service.readRegistrationResponses(client)
	t.Cleanup(service.StopCurrent)
	t.Cleanup(func() {
		_ = registrar.Close()
		_ = client.Close()
	})
	return registrar, service
}
