package imscore

import (
	"context"
	"fmt"
	"io"
	"net"
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

func TestOnlyVodafoneUKEarlyPeerResetArmsPCSCFFailover(t *testing.T) {
	tests := []struct {
		name   string
		preset string
		err    error
		age    time.Duration
		want   bool
	}{
		{name: "VOXI early reset", preset: vodafoneUKCarrierPresetID, err: syscallConnectionReset(), age: time.Minute, want: true},
		{name: "VOXI EOF", preset: vodafoneUKCarrierPresetID, err: io.EOF, age: time.Minute},
		{name: "VOXI late reset", preset: vodafoneUKCarrierPresetID, err: syscallConnectionReset(), age: 3 * time.Minute},
		{name: "other carrier reset", preset: "2degrees_nz", err: syscallConnectionReset(), age: time.Minute},
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
			_, _, got := service.pendingPortSResetFailover()
			if got != test.want {
				t.Fatalf("failover pending = %t, want %t", got, test.want)
			}
		})
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
