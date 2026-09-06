package imscore

import (
	"context"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/smscodec"
)

func TestRPReportQueueKeepsSenderAndRejectionOnSamePath(t *testing.T) {
	s, _, _ := newInboundSMSTestService(t)
	s.mu.Lock()
	s.registrar = "pcscf-a.example:5060"
	s.mu.Unlock()
	sent := make(chan string, 2)
	s.transport.SetSendFn(func(raw string) error { sent <- raw; return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	firstDone := make(chan error, 1)
	go func() {
		_, err := s.dispatchOutboundMESSAGE(ctx, "test", parsedDispatchRequest(t, "block-message-queue", 1), time.Second)
		firstDone <- err
	}()
	first := waitForOutboundSMSControl(t, sent)
	raw := inboundSMSRequest(t, imsSMSContentType, inboundRPData(t, 0x45, "+447700900123", "queued report"))
	reportDone := make(chan error, 1)
	go func() {
		reportDone <- s.sendRPReport(rpReportRequest{Inbound: raw, Body: smscodec.BuildRPAck(0x45), RPMR: 0x45})
	}()
	deadline := time.Now().Add(time.Second)
	for len(s.outboundMsgCh) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(s.outboundMsgCh) == 0 {
		t.Fatal("report was not queued")
	}
	s.mu.Lock()
	s.registrar = "pcscf-b.example:5060"
	s.mu.Unlock()
	replacement := make(chan string, 1)
	s.transport.SetSendFn(func(raw string) error { replacement <- raw; return nil })
	s.transport.DeliverResponse(registerResponseForRequest(first, 200, nil))
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	var report string
	select {
	case report = <-sent:
	case <-replacement:
		t.Fatal("report used replacement sender but retained original rejection path")
	case <-ctx.Done():
		t.Fatal("queued report was not sent")
	}
	s.transport.DeliverResponse(registerResponseForRequest(report, 488, nil))
	err := <-reportDone
	if rpReportRejectStatus(err) != 488 || rpReportRejectRegistrar(err) != "pcscf-a.example:5060" {
		t.Fatalf("report rejection path = %q, error = %v", rpReportRejectRegistrar(err), err)
	}
}
