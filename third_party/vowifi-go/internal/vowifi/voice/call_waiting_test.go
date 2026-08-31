package voice

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
)

func TestWaitingCall180CarriesCallWaitingAlertInfo(t *testing.T) {
	agent := startedVoiceAgent(t)
	firstPeer := listenVoiceUDP(t)
	firstResponder := &capturedVoiceResponder{localTag: "first-tag"}
	first := inboundAgentInvite("cw-active", firstPeer, firstResponder)
	if _, err := agent.HandleInboundVoiceRequest(first); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(first.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	if got := firstResponder.lastResponse(); got.StatusCode == 180 && strings.TrimSpace(got.AlertInfo) != "" {
		t.Fatalf("non-waiting 180 Alert-Info = %q", got.AlertInfo)
	}
	secondPeer := listenVoiceUDP(t)
	secondResponder := &capturedVoiceResponder{localTag: "wait-tag"}
	second := inboundAgentInvite("cw-waiting", secondPeer, secondResponder)
	if _, err := agent.HandleInboundVoiceRequest(second); err != nil {
		t.Fatal(err)
	}
	got := secondResponder.lastResponse()
	if got.StatusCode != 180 {
		t.Fatalf("waiting status=%d, want 180", got.StatusCode)
	}
	if got.AlertInfo != "<urn:alert:service:call-waiting>" {
		t.Fatalf("waiting Alert-Info = %q", got.AlertInfo)
	}
}

func TestNonWaiting180OmitsAlertInfo(t *testing.T) {
	agent := startedVoiceAgent(t)
	peer := listenVoiceUDP(t)
	responder := &capturedVoiceResponder{localTag: "local-tag"}
	invite := inboundAgentInvite("cw-single", peer, responder)
	if _, err := agent.HandleInboundVoiceRequest(invite); err != nil {
		t.Fatal(err)
	}
	got := responder.lastResponse()
	if got.StatusCode != 180 {
		t.Fatalf("status=%d, want 180", got.StatusCode)
	}
	if strings.TrimSpace(got.AlertInfo) != "" {
		t.Fatalf("non-waiting 180 Alert-Info = %q", got.AlertInfo)
	}
}

func TestTUECWTimeoutSends480Cause19(t *testing.T) {
	agent := startedVoiceAgent(t)
	agent.cwTimeout = 40 * time.Millisecond
	firstPeer := listenVoiceUDP(t)
	first := inboundAgentInvite("cw-to-active", firstPeer, &capturedVoiceResponder{localTag: "t1"})
	if _, err := agent.HandleInboundVoiceRequest(first); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(first.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	secondPeer := listenVoiceUDP(t)
	secondResponder := &capturedVoiceResponder{localTag: "t2"}
	second := inboundAgentInvite("cw-to-wait", secondPeer, secondResponder)
	if _, err := agent.HandleInboundVoiceRequest(second); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	var last imscore.InboundVoiceResponse
	for time.Now().Before(deadline) {
		last = secondResponder.lastResponse()
		if last.StatusCode == 480 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if last.StatusCode != 480 {
		t.Fatalf("timeout status=%d responses=%v", last.StatusCode, secondResponder.statuses())
	}
	if !strings.Contains(last.Reason, "cause=19") {
		t.Fatalf("480 Reason = %q, want cause=19", last.Reason)
	}
	if agent.callByID(second.CallID) != nil {
		t.Fatal("waiting call remained after TUE-CW timeout")
	}
}

func TestCANCELStopsTUECWTimer(t *testing.T) {
	agent := startedVoiceAgent(t)
	agent.cwTimeout = 80 * time.Millisecond
	firstPeer := listenVoiceUDP(t)
	first := inboundAgentInvite("cw-cancel-active", firstPeer, &capturedVoiceResponder{localTag: "c1"})
	if _, err := agent.HandleInboundVoiceRequest(first); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(first.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	secondPeer := listenVoiceUDP(t)
	secondResponder := &capturedVoiceResponder{localTag: "c2"}
	second := inboundAgentInvite("cw-cancel-wait", secondPeer, secondResponder)
	if _, err := agent.HandleInboundVoiceRequest(second); err != nil {
		t.Fatal(err)
	}
	cancelResponder := &capturedVoiceResponder{localTag: "cancel-tag"}
	result, err := agent.HandleInboundVoiceRequest(imscore.InboundVoiceRequest{
		Method: "CANCEL", CallID: second.CallID, Responder: cancelResponder,
	})
	if err != nil || result.StatusCode != 0 {
		t.Fatalf("CANCEL result=%+v err=%v", result, err)
	}
	time.Sleep(150 * time.Millisecond)
	if fmt.Sprint(secondResponder.statuses()) != "[180 487]" {
		t.Fatalf("waiting responses = %v, want [180 487]", secondResponder.statuses())
	}
	if secondResponder.lastResponse().StatusCode == 480 {
		t.Fatal("TUE-CW 480 arrived after CANCEL")
	}
}
