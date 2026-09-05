package imscore

import (
	"bufio"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/smscodec"
)

func TestVodafoneUKMTReport488RequestsFreshPCSCFPath(t *testing.T) {
	service, _, _ := newInboundSMSTestService(t)
	service.cfg.CarrierPresetID = vodafoneUKCarrierPresetID
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{"pcscf-a.example:5060", "pcscf-b.example:5060"}
	service.mu.Unlock()
	service.transport.SetSendFn(func(request string) error {
		service.transport.DeliverResponse(registerResponseForRequest(request, 488, nil))
		return nil
	})

	raw := inboundSMSRequest(t, imsSMSContentType,
		inboundRPData(t, 0x42, "+447700900123", "rejected report"))
	service.sendRPReportWithRetry(rpReportRequest{
		Inbound: raw, Body: smscodec.BuildRPAck(0x42), RPMR: 0x42,
	})

	select {
	case err := <-service.RegistrationErrors():
		if err == nil || !strings.Contains(err.Error(), "MT SMS RP report") ||
			!strings.Contains(err.Error(), "fresh runtime required") {
			t.Fatalf("runtime recovery error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing runtime recovery request")
	}
	status := service.StatusCurrent()
	until := status.DeprioritizedPCSCF["pcscf-a.example:5060"]
	if until.Before(time.Now().Add(29 * time.Minute)) {
		t.Fatalf("rejected P-CSCF penalty expires too early: %s", until)
	}
	if status.LastSIPCode != 488 || service.RegState() == regRegistered {
		t.Fatalf("rejected P-CSCF state = %+v", status)
	}
}

func TestMTReport488RecoveryIsCarrierScoped(t *testing.T) {
	service := newPortSSessionTestService(t, "2degrees_nz")
	service.mu.Lock()
	service.regState = regRegistered
	service.signalingReady = true
	service.mu.Unlock()

	service.triggerMTReportPCSCFRecovery(&rpReportRejectError{
		Status: 488, Registrar: "pcscf-a.example:5060",
	})

	select {
	case err := <-service.RegistrationErrors():
		t.Fatalf("other carrier requested runtime recovery: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	status := service.StatusCurrent()
	if len(status.DeprioritizedPCSCF) != 0 || service.RegState() != regRegistered {
		t.Fatalf("other carrier state changed = %+v", status)
	}
}

func TestVodafoneUKMTReportNon488DoesNotChangePCSCFPath(t *testing.T) {
	service := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	service.mu.Lock()
	service.regState = regRegistered
	service.signalingReady = true
	service.mu.Unlock()

	service.triggerMTReportPCSCFRecovery(&rpReportRejectError{
		Status: 503, Registrar: "pcscf-a.example:5060",
	})

	select {
	case err := <-service.RegistrationErrors():
		t.Fatalf("non-488 report requested runtime recovery: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	status := service.StatusCurrent()
	if len(status.DeprioritizedPCSCF) != 0 || service.RegState() != regRegistered {
		t.Fatalf("non-488 report changed P-CSCF state = %+v", status)
	}
}

func TestMTReport488DoesNotTearDownAChangedPCSCFPath(t *testing.T) {
	service, _, _ := newInboundSMSTestService(t)
	service.cfg.CarrierPresetID = vodafoneUKCarrierPresetID
	service.mu.Lock()
	service.registrar = "pcscf-a.example:5060"
	service.registrarCandidates = []string{"pcscf-a.example:5060", "pcscf-b.example:5060"}
	service.mu.Unlock()
	serviceConn, networkConn := net.Pipe()
	t.Cleanup(func() { _ = serviceConn.Close() })
	t.Cleanup(func() { _ = networkConn.Close() })
	service.recordPortSOpened(serviceConn, time.Now())

	service.mu.Lock()
	service.registrar = "pcscf-b.example:5060"
	service.registrarIndex = 1
	service.mu.Unlock()
	readResult := make(chan error, 1)
	go func() {
		request, err := readSIPStreamMessage(bufio.NewReader(networkConn))
		if err == nil {
			service.transport.DeliverResponse(registerResponseForRequest(request, 488, nil))
		}
		readResult <- err
	}()
	raw := inboundSMSRequest(t, imsSMSContentType,
		inboundRPData(t, 0x43, "+447700900123", "stale path"))
	service.sendRPReportWithRetry(rpReportRequest{
		Inbound: raw, Body: smscodec.BuildRPAck(0x43), PeerConn: serviceConn, RPMR: 0x43,
	})
	if err := <-readResult; err != nil {
		t.Fatalf("read RP report: %v", err)
	}

	deadline := time.Now().Add(time.Second)
	for service.pcscfRecoveryPending.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.pcscfRecoveryPending.Load() {
		t.Fatal("stale report recovery did not finish")
	}
	select {
	case err := <-service.RegistrationErrors():
		t.Fatalf("stale report recovery tore down the current path: %v", err)
	default:
	}
	status := service.StatusCurrent()
	if _, penalized := status.DeprioritizedPCSCF["pcscf-a.example:5060"]; !penalized {
		t.Fatal("the P-CSCF that returned 488 was not penalized")
	}
	if _, penalized := status.DeprioritizedPCSCF["pcscf-b.example:5060"]; penalized {
		t.Fatal("the replacement P-CSCF was incorrectly penalized")
	}
	if service.RegState() != regRegistered {
		t.Fatalf("replacement P-CSCF registration state = %s", service.RegState())
	}
}
