package imscore

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestPortSSessionDiagnosticsRecordPeerReset(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.mu.Lock()
	service.outboundContactRegistered = true
	service.mu.Unlock()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	openedAt := time.Now().Add(-time.Minute)
	closedAt := time.Now()
	service.recordPortSOpened(client, openedAt)
	service.recordPortSInbound(closedAt.Add(-time.Second))
	service.recordPortSClosed(client, fmt.Errorf("read tcp: %w", syscallConnectionReset()), closedAt)

	status := service.StatusCurrent()
	if status.PortSGeneration != 1 || status.PortSPeerResetCount != 1 ||
		status.PortSLastCloseKind != portSClosePeerReset {
		t.Fatalf("port-s diagnostics = %+v", status)
	}
	if status.PortSOpenedAt != openedAt || status.PortSClosedAt != closedAt || status.PortSLastInboundAt.IsZero() {
		t.Fatalf("port-s timestamps = opened %s closed %s inbound %s",
			status.PortSOpenedAt, status.PortSClosedAt, status.PortSLastInboundAt)
	}
	if status.RegistrationRegID != 1 {
		t.Fatalf("registration reg-id = %d", status.RegistrationRegID)
	}
}

func TestVodafoneUKPeerResetPolicyIsCarrierAndErrorScoped(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		err    error
		age    time.Duration
		want   bool
	}{
		{name: "Vodafone UK early reset", preset: vodafoneUKCarrierPresetID, err: syscallConnectionReset(), age: time.Minute, want: true},
		{name: "Vodafone UK EOF", preset: vodafoneUKCarrierPresetID, err: io.EOF, age: time.Minute},
		{name: "Vodafone UK established EOF", preset: vodafoneUKCarrierPresetID, err: io.EOF, age: 9 * time.Minute},
		{name: "other carrier reset", preset: "2degrees_nz", err: syscallConnectionReset(), age: time.Minute},
		{name: "other carrier established reset", preset: "2degrees_nz", err: syscallConnectionReset(), age: 9 * time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newPortSSessionTestService(t, test.preset)
			client, server := net.Pipe()
			t.Cleanup(func() {
				_ = client.Close()
				_ = server.Close()
			})
			now := time.Now()
			service.recordPortSOpened(client, now.Add(-test.age))
			service.recordPortSClosed(client, test.err, now)
			if _, _, pending := service.pendingPortSResetFailover(); pending {
				t.Fatal("P-CSCF failover started before the recovery REGISTER")
			}
			service.markPortSResetRecoveryAttempt("pcscf-a.example:5060")
			if _, _, pending := service.pendingPortSResetFailover(); pending {
				t.Fatal("P-CSCF failover started before the recovery REGISTER succeeded")
			}
			service.markPortSResetRecoverySucceeded("pcscf-a.example:5060")
			if _, _, pending := service.pendingPortSResetFailover(); pending {
				t.Fatal("a successful recovery REGISTER incorrectly triggered P-CSCF failover")
			}

			recoveredClient, recoveredServer := net.Pipe()
			t.Cleanup(func() {
				_ = recoveredClient.Close()
				_ = recoveredServer.Close()
			})
			secondOpenedAt := time.Now().Add(time.Millisecond)
			service.recordPortSOpened(recoveredClient, secondOpenedAt)
			service.pcscfRecoveryPending.Store(true)
			service.recordPortSClosed(recoveredClient, test.err, secondOpenedAt.Add(test.age))
			_, _, got := service.pendingPortSResetFailover()
			service.pcscfRecoveryPending.Store(false)
			if got != test.want {
				t.Fatalf("failover pending = %t, want %t", got, test.want)
			}
		})
	}
}

func TestVodafoneUKPeerResetUsesShortReconnectGrace(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		kind   string
		want   time.Duration
	}{
		{name: "Vodafone UK peer reset", preset: vodafoneUKCarrierPresetID, kind: portSClosePeerReset, want: 5 * time.Second},
		{name: "Vodafone UK EOF", preset: vodafoneUKCarrierPresetID, kind: portSCloseEOF, want: defaultPortSReconnectGrace},
		{name: "other carrier reset", preset: "2degrees_nz", kind: portSClosePeerReset, want: defaultPortSReconnectGrace},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := newPortSSessionTestService(t, test.preset)
			service.portSSessionMu.Lock()
			service.portSSession.lastCloseKind = test.kind
			service.portSSessionMu.Unlock()
			if got := service.portSReconnectWait(); got != test.want {
				t.Fatalf("reconnect grace = %s, want %s", got, test.want)
			}
		})
	}
}

func TestVodafoneUKPeerResetKeepsWatchdogAfterOnDemandReconnect(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSOnDemandObserved.Store(true)
	service.mu.Lock()
	service.regState = regRegistered
	service.mu.Unlock()
	service.portSSessionMu.Lock()
	service.portSSession.lastCloseKind = portSClosePeerReset
	service.portSSessionMu.Unlock()

	service.handleProtectedServerPushClosed()
	if !service.portSReconnectWaiting.Load() {
		t.Fatal("Vodafone UK peer reset did not enter reconnect grace")
	}
	service.portSWatchMu.Lock()
	timer := service.portSWatchTimer
	generation := service.portSWatchGeneration
	service.portSWatchMu.Unlock()
	if timer == nil {
		t.Fatal("prior on-demand reconnect suppressed Vodafone UK peer-reset watchdog")
	}
	service.portSReconnectWatchFired(generation, service.currentPortSRecoveryRegistrar())
	if !service.reRegisterPending.Load() || !service.portSRecoveryPending.Load() {
		t.Fatal("expired Vodafone UK peer-reset watchdog did not schedule REGISTER recovery")
	}
}

func TestVodafoneUKEstablishedPeerResetArmsFailoverAfterRecovery(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})

	now := time.Now()
	service.recordPortSOpened(client, now.Add(-9*time.Minute))
	service.recordPortSClosed(client, syscallConnectionReset(), now)
	if _, _, pending := service.pendingPortSResetFailover(); pending {
		t.Fatal("established reset bypassed same-P-CSCF recovery")
	}
	service.markPortSResetRecoveryAttempt("pcscf-a.example:5060")
	if _, _, pending := service.pendingPortSResetFailover(); pending {
		t.Fatal("established reset armed failover before recovery succeeded")
	}
	service.markPortSResetRecoverySucceeded("pcscf-a.example:5060")
	registrar, observedAt, pending := service.pendingPortSResetFailover()
	if !pending || registrar != "pcscf-a.example:5060" || !observedAt.Equal(now) {
		t.Fatalf("failover = (%q, %s, %t), want recovered established reset", registrar, observedAt, pending)
	}
}

func TestVodafoneUKEstablishedResetOnDemandReconnectCancelsFailover(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	failedClient, failedServer := net.Pipe()
	reopenedClient, reopenedServer := net.Pipe()
	t.Cleanup(func() {
		_ = failedClient.Close()
		_ = failedServer.Close()
		_ = reopenedClient.Close()
		_ = reopenedServer.Close()
	})

	now := time.Now()
	service.recordPortSOpened(failedClient, now.Add(-9*time.Minute))
	service.recordPortSClosed(failedClient, syscallConnectionReset(), now)
	service.portSReconnectWaiting.Store(true)
	if !service.trackProtectedConnection(reopenedClient) {
		t.Fatal("track on-demand port-s reconnect")
	}
	service.markPortSResetRecoveryAttempt("pcscf-a.example:5060")
	service.markPortSResetRecoverySucceeded("pcscf-a.example:5060")
	if _, _, pending := service.pendingPortSResetFailover(); pending {
		t.Fatal("on-demand port-s reconnect kept established-reset failover")
	}
}

func TestLocalPortSCloseIsNotTreatedAsPeerReset(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	now := time.Now()
	service.recordPortSOpened(client, now.Add(-time.Minute))
	service.markPortSLocalClose(client)
	service.recordPortSClosed(client, syscallConnectionReset(), now)
	service.markPortSResetRecoveryAttempt("pcscf-a.example:5060")
	service.markPortSResetRecoverySucceeded("pcscf-a.example:5060")
	if _, _, pending := service.pendingPortSResetFailover(); pending {
		t.Fatal("local close armed P-CSCF failover")
	}
	if kind := service.StatusCurrent().PortSLastCloseKind; kind != portSCloseLocal {
		t.Fatalf("close kind = %q, want %q", kind, portSCloseLocal)
	}
}

func TestVodafoneUKResetBeforeRegisterRecoverySuccessDoesNotArmFailover(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	firstClient, firstServer := net.Pipe()
	recoveredClient, recoveredServer := net.Pipe()
	t.Cleanup(func() {
		_ = firstClient.Close()
		_ = firstServer.Close()
		_ = recoveredClient.Close()
		_ = recoveredServer.Close()
	})

	now := time.Now()
	service.recordPortSOpened(firstClient, now.Add(-time.Minute))
	service.recordPortSClosed(firstClient, syscallConnectionReset(), now)
	service.markPortSResetRecoveryAttempt("pcscf-a.example:5060")
	secondOpenedAt := time.Now().Add(time.Millisecond)
	service.recordPortSOpened(recoveredClient, secondOpenedAt)
	service.recordPortSClosed(recoveredClient, syscallConnectionReset(), secondOpenedAt.Add(time.Second))
	service.markPortSResetRecoverySucceeded("pcscf-a.example:5060")

	if _, _, pending := service.pendingPortSResetFailover(); pending {
		t.Fatal("a reset before REGISTER recovery success armed P-CSCF failover")
	}
}

func TestVodafoneUKPortSResetRecoverySwitchesPCSCFTemporarily(t *testing.T) {
	first, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	firstSeen := make(chan string, 1)
	secondSeen := make(chan string, 1)
	go serveRegisterStatus(first, 200, firstSeen)
	go serveRegisterStatus(second, 200, secondSeen)

	config := registerTransportTestConfig("udp", first.LocalAddr().String()+";"+second.LocalAddr().String())
	config.CarrierPresetID = vodafoneUKCarrierPresetID
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer service.StopCurrent()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := service.Register(ctx); err != nil {
		t.Fatalf("initial Register: %v", err)
	}
	<-firstSeen

	service.pcscfRecoveryPending.Store(true)
	service.recoverPCSCFAfterPortSReset(first.LocalAddr().String(), time.Now())
	select {
	case <-secondSeen:
	case <-ctx.Done():
		t.Fatal("alternate P-CSCF did not receive REGISTER")
	}
	status := service.StatusCurrent()
	if status.Registrar != second.LocalAddr().String() || status.RegistrationGeneration != 2 {
		t.Fatalf("status after P-CSCF switch = %+v", status)
	}
	until := status.DeprioritizedPCSCF[first.LocalAddr().String()]
	if until.Before(time.Now().Add(29 * time.Minute)) {
		t.Fatalf("failed P-CSCF penalty expires too early: %s", until)
	}
}

func TestVodafoneUKPortSResetWithoutAlternateRequestsFreshRuntime(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.mu.Lock()
	service.registrarCandidates = []string{"pcscf-a.example:5060"}
	service.regState = regRegistered
	service.signalingReady = true
	service.mu.Unlock()

	service.pcscfRecoveryPending.Store(true)
	service.recoverPCSCFAfterPortSReset("pcscf-a.example:5060", time.Now())
	select {
	case err := <-service.RegistrationErrors():
		if err == nil || !strings.Contains(err.Error(), "fresh runtime required") {
			t.Fatalf("runtime recovery error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing fresh runtime recovery request")
	}
	if service.RegState() == regRegistered {
		t.Fatal("single failed P-CSCF remained registered")
	}
}

func TestVodafoneUKFailoverValidationRequiresProvenDownlink(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSFailoverVerifyWait = 5 * time.Millisecond
	service.mu.Lock()
	service.regSession = &registerSession{security: &securityAgreement{verifyHeader: "ipsec-3gpp"}}
	service.mu.Unlock()

	if validatedBy, ok := service.waitForPortSFailoverValidation(0); ok || validatedBy != "" {
		t.Fatalf("unproven downlink validation = (%q, %t)", validatedBy, ok)
	}

	service.portSPushReady.Store(true)
	if validatedBy, ok := service.waitForPortSFailoverValidation(0); !ok || validatedBy != "port-s" {
		t.Fatalf("port-s validation = (%q, %t)", validatedBy, ok)
	}
	service.portSPushReady.Store(false)
	service.inboundSIPHandledRequest.Add(1)
	if validatedBy, ok := service.waitForPortSFailoverValidation(0); !ok || validatedBy != "inbound_sip_request" {
		t.Fatalf("inbound request validation = (%q, %t)", validatedBy, ok)
	}
}

func TestVodafoneUKUnverifiedFailoverPreservesCandidatePenalties(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.mu.Lock()
	service.registrar = "pcscf-b.example:5060"
	service.registrarIndex = 1
	service.regState = regRegistered
	service.mu.Unlock()
	firstUntil := time.Now().Add(vodafoneUKPCSCFDeprioritizedPeriod)
	service.registrarPenalties.mark("pcscf-a.example:5060", firstUntil)

	service.rejectUnverifiedPortSRegistrar("pcscf-b.example:5060", time.Now(), "downlink validation timed out")
	status := service.StatusCurrent()
	if len(status.DeprioritizedPCSCF) != 2 {
		t.Fatalf("deprioritized P-CSCFs = %v, want one complete candidate round", status.DeprioritizedPCSCF)
	}
	if status.DeprioritizedPCSCF["pcscf-a.example:5060"].Before(firstUntil) {
		t.Fatal("existing P-CSCF penalty was shortened")
	}
	select {
	case err := <-service.RegistrationErrors():
		if err == nil || !strings.Contains(err.Error(), "fresh runtime required") {
			t.Fatalf("runtime recovery error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing runtime recovery request")
	}
}

func TestVodafoneUKFailedAlternateRegisterHonorsRetryAfter(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	now := time.Unix(1_700_000_000, 0)
	err := registerResponseErrorWithRetryAfter(t, "3600")

	if got := service.failedRegisterUnavailableUntil(err, now); !got.Equal(now.Add(time.Hour)) {
		t.Fatalf("Retry-After penalty = %s, want %s", got, now.Add(time.Hour))
	}
}

func TestVodafoneUKFailedAlternateRegisterDoesNotLetRetryAfterShortenBackoff(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	now := time.Unix(1_700_000_000, 0)
	err := registerResponseErrorWithRetryAfter(t, "1")

	if got := service.failedRegisterUnavailableUntil(err, now); !got.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("short Retry-After penalty = %s, want %s", got, now.Add(30*time.Second))
	}
}

func TestVodafoneUKFailedAlternateRegisterUsesRFC5626Penalty(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.portSRecoveryJitter = func(upper time.Duration) time.Duration { return upper / 2 }
	now := time.Unix(1_700_000_000, 0)
	err := &registerResponseError{statusCode: 503, message: "Service Unavailable"}

	want := now.Add(30 * time.Second)
	if got := service.failedRegisterUnavailableUntil(err, now); !got.Equal(want) {
		t.Fatalf("RFC 5626 penalty = %s, want %s", got, want)
	}
}

func newPortSSessionTestService(t *testing.T, preset string) *Service {
	t.Helper()
	config := registerTransportTestConfig("udp", "pcscf-a.example:5060;pcscf-b.example:5060")
	config.CarrierPresetID = preset
	service, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.StopCurrent)
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{"pcscf-a.example:5060", "pcscf-b.example:5060"}
	service.mu.Unlock()
	return service
}

func syscallConnectionReset() error {
	return &net.OpError{Op: "read", Net: "tcp", Err: fmt.Errorf("connection reset by peer")}
}
