package voice

import (
	"context"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestInitialInviteOmitsSessionExpiresPerIR92(t *testing.T) {
	agent := newTestAgent(t)
	call := NewCall(agent, callstate.DirectionOutbound, "call-se-omit", "+447700900001")
	invite := BuildIMSInvite(agent, call)
	if !strings.Contains(invite, "Supported: "+voiceInviteSupported) {
		t.Fatalf("initial INVITE missing Supported timer: %s", invite)
	}
	if voiceTestHeader(invite, "Session-Expires") != "" {
		t.Fatalf("IR.92 2.2.8 allows omitting Session-Expires; got %q", voiceTestHeader(invite, "Session-Expires"))
	}
}

func TestInboundInviteWithoutSessionExpiresUsesIR92Default(t *testing.T) {
	agent := startedVoiceAgent(t)
	peer := listenVoiceUDP(t)
	responder := &capturedVoiceResponder{localTag: "local-tag"}
	invite := inboundAgentInvite("call-in-se-default", peer, responder)
	invite.Supported = "100rel, timer"
	if _, err := agent.HandleInboundVoiceRequest(invite); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(invite.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	call := agent.callByID(invite.CallID)
	response := responder.lastResponse()
	if response.SessionExpires != "1800;refresher=uac" {
		t.Fatalf("IR.92 inbound 2xx Session-Expires = %q", response.SessionExpires)
	}
	if call.weAreSessionRefresher() || call.voiceSessionExpires() != 1800*time.Second {
		t.Fatalf("inbound default refresher=%t expires=%s", call.weAreSessionRefresher(), call.voiceSessionExpires())
	}
}

func TestInboundInviteWithoutTimerDoesNotInventSessionExpires(t *testing.T) {
	call := NewCall(nil, callstate.DirectionInbound, "call-no-timer", "+447700900001")
	call.applyInboundSessionTimer("100rel", "", "")
	if got := formatSessionExpiresHeader(call); got != "" {
		t.Fatalf("invented Session-Expires = %q", got)
	}
}

func TestInboundInviteWithoutSessionExpiresHonorsMinSE(t *testing.T) {
	call := NewCall(nil, callstate.DirectionInbound, "call-minse-default", "+447700900001")
	call.applyInboundSessionTimer("timer", "", "2400")
	if got := formatSessionExpiresHeader(call); got != "2400;refresher=uac" {
		t.Fatalf("greater of 1800 and Min-SE = %q", got)
	}
}

func TestWeAreSessionRefresher(t *testing.T) {
	tests := []struct {
		direction callstate.Direction
		refresher string
		want      bool
	}{
		{direction: callstate.DirectionOutbound, refresher: sessionRefresherUAC, want: true},
		{direction: callstate.DirectionOutbound, refresher: sessionRefresherUAS, want: false},
		{direction: callstate.DirectionInbound, refresher: sessionRefresherUAS, want: true},
		{direction: callstate.DirectionInbound, refresher: sessionRefresherUAC, want: false},
		{direction: callstate.DirectionOutbound, want: true},
	}
	for _, test := range tests {
		call := NewCall(nil, test.direction, "call-refresher", "+447700900001")
		if test.refresher == "" {
			call.applyVoiceSessionExpires("1800")
		} else {
			call.applyVoiceSessionExpires("1800;refresher=" + test.refresher)
		}
		if got := call.weAreSessionRefresher(); got != test.want {
			t.Fatalf("dir=%s refresher=%q got %t want %t", test.direction, test.refresher, got, test.want)
		}
	}
}

func TestFormatSessionExpiresIncludesRefresherAndMinSE(t *testing.T) {
	call := NewCall(nil, callstate.DirectionOutbound, "call-expires", "+447700900001")
	call.applyVoiceSessionExpires("120;refresher=uac")
	call.applySessionMinSE(90 * time.Second)
	if got := formatSessionExpiresHeader(call); got != "120;refresher=uac" {
		t.Fatalf("Session-Expires = %q", got)
	}
	headers := formatSessionTimerHeaders(call)
	if !strings.Contains(headers, "Supported: timer\r\n") ||
		!strings.Contains(headers, "Session-Expires: 120;refresher=uac\r\n") ||
		!strings.Contains(headers, "Min-SE: 90\r\n") {
		t.Fatalf("session timer headers = %q", headers)
	}
}

func TestApplySessionMinSERaisesExpiry(t *testing.T) {
	call := NewCall(nil, callstate.DirectionOutbound, "call-minse", "+447700900001")
	call.applyVoiceSessionExpires("30;refresher=uac")
	call.applySessionMinSE(90 * time.Second)
	if call.voiceSessionExpires() != 90*time.Second {
		t.Fatalf("expires = %s", call.voiceSessionExpires())
	}
}

func TestSessionRefreshRetries422Once(t *testing.T) {
	var updates atomic.Int32
	registrar := startScriptedVoiceRegistrar(t, func(request string) (int, string, string) {
		switch {
		case strings.HasPrefix(request, "UPDATE "):
			if updates.Add(1) == 1 {
				return 422, "Min-SE: 90\r\n", ""
			}
			if got := voiceTestHeader(request, "Session-Expires"); got != "90;refresher=uac" {
				t.Errorf("retried UPDATE Session-Expires = %q", got)
			}
			return 200, "", ""
		case strings.HasPrefix(request, "INVITE "):
			return 200, "To: <sip:callee@ims.example.com>;tag=remote\r\nContact: <sip:callee@ims.example.com>\r\nContent-Type: application/sdp\r\n", testIMSAnswerSDP
		default:
			return 200, "", ""
		}
	})
	agent := newVoiceTestAgent(t, registrar)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })
	call, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatal(err)
	}
	call.applyVoiceSessionExpires("30;refresher=uac")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := agent.refreshVoiceSession(ctx, call); err != nil {
		t.Fatal(err)
	}
	if updates.Load() != 2 {
		t.Fatalf("UPDATE count = %d", updates.Load())
	}
	if call.voiceSessionExpires() != 90*time.Second {
		t.Fatalf("expires after 422 = %s", call.voiceSessionExpires())
	}
}

func TestSessionRefreshFallsBackToReinvite(t *testing.T) {
	var invites, updates atomic.Int32
	registrar := startScriptedVoiceRegistrar(t, func(request string) (int, string, string) {
		switch {
		case strings.HasPrefix(request, "UPDATE "):
			updates.Add(1)
			return 405, "", ""
		case strings.HasPrefix(request, "INVITE "):
			invites.Add(1)
			return 200, "To: <sip:callee@ims.example.com>;tag=remote\r\nContact: <sip:callee@ims.example.com>\r\nContent-Type: application/sdp\r\n", testIMSAnswerSDP
		default:
			return 200, "", ""
		}
	})
	agent := newVoiceTestAgent(t, registrar)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })
	call, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatal(err)
	}
	call.applyVoiceSessionExpires("120;refresher=uac")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := agent.refreshVoiceSession(ctx, call); err != nil {
		t.Fatal(err)
	}
	if updates.Load() != 1 || invites.Load() != 2 {
		t.Fatalf("UPDATE=%d INVITE=%d", updates.Load(), invites.Load())
	}
}

func TestInboundSessionUpdateEchoesRefresherAndDoesNotRefresh(t *testing.T) {
	agent := startedVoiceAgent(t)
	peer := listenVoiceUDP(t)
	responder := &capturedVoiceResponder{localTag: "local-tag"}
	invite := inboundAgentInvite("call-in-timer", peer, responder)
	invite.SessionExpires = "120;refresher=uac"
	if _, err := agent.HandleInboundVoiceRequest(invite); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(invite.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	call := agent.callByID(invite.CallID)
	if call.weAreSessionRefresher() {
		t.Fatal("inbound UAC refresher made us the refresher")
	}
	updateResponder := &capturedVoiceResponder{localTag: "local-tag"}
	result, err := agent.HandleInboundVoiceRequest(imscore.InboundVoiceRequest{
		Method: "UPDATE", CallID: invite.CallID, SessionExpires: "1800;refresher=uac",
		Responder: updateResponder,
	})
	if err != nil || result.StatusCode != 0 {
		t.Fatalf("inbound UPDATE result=%+v err=%v", result, err)
	}
	response := updateResponder.lastResponse()
	if response.StatusCode != 200 || response.SessionExpires != "1800;refresher=uac" {
		t.Fatalf("UPDATE 200 = %+v", response)
	}
	if call.weAreSessionRefresher() || call.voiceSessionExpires() != 1800*time.Second {
		t.Fatalf("refresher=%t expires=%s", call.weAreSessionRefresher(), call.voiceSessionExpires())
	}
}
