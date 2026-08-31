package voice

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestBuildConsultativeReferContainsReplaces(t *testing.T) {
	call := NewCall(&Agent{}, callstate.DirectionOutbound, "consult-id@ims.example", "sip:+447700900999@ims.example")
	call.setVoiceDialog(&voiceSIPDialog{
		localURI: "sip:local@ims.example", remoteURI: "sip:+447700900999@ims.example",
		localTag: "from-a", remoteTag: "to-c",
	})
	got, err := formatConsultativeReferTo(call)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Replaces=") || !strings.Contains(got, "to-tag%3D") || !strings.Contains(got, "from-tag%3D") {
		t.Fatalf("Refer-To missing Replaces encoding: %s", got)
	}
	if !strings.Contains(got, "method=INVITE") {
		t.Fatalf("Refer-To missing method: %s", got)
	}
}

func TestTransferConsultativeFailsWithoutReplaces(t *testing.T) {
	agent := startedVoiceAgent(t)
	first, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatal(err)
	}
	secondSDP := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=client\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 32010 RTP/AVP 0\r\n"
	second, err := agent.HandleClientInvite("+8613800000001", secondSDP)
	if err != nil {
		t.Fatal(err)
	}
	second.setVoiceDialog(&voiceSIPDialog{
		localURI: "sip:local@ims.example", remoteURI: "sip:peer@ims.example",
	})
	err = agent.TransferConsultative(context.Background(), first.CallID(), second.CallID())
	if !errors.Is(err, ErrECTRequiresReplaces) {
		t.Fatalf("err=%v", err)
	}
}

func TestTransferConsultativeSendsREFERAndReleases(t *testing.T) {
	var mu sync.Mutex
	var refers []string
	referSeen := make(chan struct{})
	registrar := startScriptedVoiceRegistrar(t, func(request string) (int, string, string) {
		if strings.HasPrefix(request, "INVITE ") {
			extra := "To: <sip:peer@ims.example.com>;tag=remote\r\nContact: <sip:peer@ims.example.com>\r\nContent-Type: application/sdp\r\n"
			return 200, extra, testIMSAnswerSDP
		}
		if strings.HasPrefix(request, "REFER ") {
			mu.Lock()
			refers = append(refers, request)
			mu.Unlock()
			select {
			case <-referSeen:
			default:
				close(referSeen)
			}
			return 202, "To: <sip:peer@ims.example.com>;tag=remote\r\n", ""
		}
		return 200, "", ""
	})
	agent := newVoiceTestAgent(t, registrar)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })
	first, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatal(err)
	}
	secondSDP := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=client\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 32010 RTP/AVP 0\r\n"
	second, err := agent.HandleClientInvite("+8613800000001", secondSDP)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		done <- agent.TransferConsultative(ctx, first.CallID(), second.CallID())
	}()
	select {
	case <-referSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("consultative REFER was not sent")
	}
	if _, err := agent.HandleInboundVoiceRequest(imscore.InboundVoiceRequest{
		Method: "NOTIFY", CallID: first.CallID(), Event: "refer",
		Body:      []byte("SIP/2.0 200 OK\r\n"),
		Responder: &capturedVoiceResponder{localTag: "notify"},
	}); err != nil {
		t.Fatalf("NOTIFY: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("transfer: %v", err)
	}
	mu.Lock()
	dump := append([]string(nil), refers...)
	mu.Unlock()
	if len(dump) == 0 || !strings.Contains(dump[0], "Replaces=") {
		t.Fatalf("REFER missing Replaces: %v", dump)
	}
	if agent.callByID(first.CallID()) != nil || agent.callByID(second.CallID()) != nil {
		t.Fatal("local dialogs were not released after ECT")
	}
}

func TestParseSipfragStatus(t *testing.T) {
	if got := parseSipfragStatus("SIP/2.0 200 OK\r\n"); got != 200 {
		t.Fatalf("status=%d", got)
	}
	if got := parseSipfragStatus("SIP/2.0 603 Decline\r\n"); got != 603 {
		t.Fatalf("status=%d", got)
	}
}
