package voice

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestInboundREFERAcceptsConnectedDialog(t *testing.T) {
	agent := startedVoiceAgent(t)
	peer := listenVoiceUDP(t)
	responder := &capturedVoiceResponder{localTag: "local-tag"}
	invite := inboundAgentInvite("refer-dialog", peer, responder)
	if _, err := agent.HandleInboundVoiceRequest(invite); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(invite.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	referResponder := &capturedVoiceResponder{localTag: "local-tag"}
	result, err := agent.HandleInboundVoiceRequest(imscore.InboundVoiceRequest{
		Method: "REFER", CallID: invite.CallID, ReferTo: "<sip:+447700900999@ims.example>",
		Responder: referResponder,
	})
	if err != nil || result.StatusCode != 0 {
		t.Fatalf("REFER result=%+v err=%v", result, err)
	}
	if got := fmt.Sprint(referResponder.statuses()); got != "[202]" {
		t.Fatalf("REFER responses = %v", referResponder.statuses())
	}
}

func TestInboundREFERWithoutDialogIs481(t *testing.T) {
	agent := startedVoiceAgent(t)
	result, err := agent.HandleInboundVoiceRequest(imscore.InboundVoiceRequest{
		Method: "REFER", CallID: "missing", ReferTo: "<sip:+447700900999@ims.example>",
	})
	if err != nil || result.StatusCode != 481 {
		t.Fatalf("REFER result=%+v err=%v", result, err)
	}
}

func TestBuildIMSReferNotifyUsesSipfrag(t *testing.T) {
	agent := &Agent{}
	call := NewCall(agent, callstate.DirectionInbound, "refer-notify", "+447700900123")
	call.setVoiceDialog(&voiceSIPDialog{
		localURI: "sip:local@ims.example", remoteURI: "sip:peer@ims.example",
		remoteTarget: "sip:peer@ims.example", localAddress: "192.0.2.10:5060",
		transport: "tcp", localTag: "local", remoteTag: "remote", cseq: 4,
	})
	got := buildIMSReferNotify(agent, call, "SIP/2.0 100 Trying\r\n", false)
	for _, want := range []string{
		"NOTIFY sip:peer@ims.example SIP/2.0",
		"Event: refer",
		"Subscription-State: active;expires=60",
		"Content-Type: message/sipfrag;version=2.0",
		"SIP/2.0 100 Trying",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("NOTIFY missing %q: %s", want, got)
		}
	}
	final := buildIMSReferNotify(agent, call, "SIP/2.0 200 OK\r\n", true)
	if !strings.Contains(final, "Subscription-State: terminated;reason=noresource") {
		t.Fatalf("final NOTIFY = %s", final)
	}
}
