package voice

import (
	"net"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestSecondOutboundCallAllowedWhenFirstIsConnected(t *testing.T) {
	agent := newTestAgent(t)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })
	first, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	secondSDP := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=client\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 32010 RTP/AVP 0\r\n"
	second, err := agent.HandleClientInvite("+8613800000001", secondSDP)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	if first.CallID() == second.CallID() {
		t.Fatal("second call reused the first call id")
	}
	if first.CallState() != callstate.StateConnected || second.CallState() != callstate.StateConnected {
		t.Fatalf("states first=%s second=%s", first.CallState(), second.CallState())
	}
	if agent.ActiveCall() != second {
		t.Fatal("new outbound call should become the current focus")
	}
	if err := agent.SwitchCall(first.CallID()); err != nil {
		t.Fatalf("switch: %v", err)
	}
	if agent.ActiveCall() != first {
		t.Fatal("switch did not move the current call")
	}
	if err := agent.HangupCurrent(second.CallID()); err != nil {
		t.Fatalf("hangup second: %v", err)
	}
	if agent.callByID(first.CallID()) == nil || first.CallState() != callstate.StateConnected {
		t.Fatalf("first call dropped after second hangup: state=%s", first.CallState())
	}
	if agent.ActiveCall() != first {
		t.Fatal("remaining call was not promoted")
	}
}

func TestAnswerSecondInboundCallAndHangupOne(t *testing.T) {
	agent := startedVoiceAgent(t)
	firstPeer := listenVoiceUDP(t)
	firstResponder := &capturedVoiceResponder{localTag: "first-tag"}
	first := inboundAgentInvite("call-active", firstPeer, firstResponder)
	if _, err := agent.HandleInboundVoiceRequest(first); err != nil {
		t.Fatal(err)
	}
	client1 := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(first.CallID, voiceSDP(client1.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	secondPeer := listenVoiceUDP(t)
	secondResponder := &capturedVoiceResponder{localTag: "wait-tag"}
	second := inboundAgentInvite("call-waiting", secondPeer, secondResponder)
	if _, err := agent.HandleInboundVoiceRequest(second); err != nil {
		t.Fatal(err)
	}
	client2 := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(second.CallID, voiceSDP(client2.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatalf("answer waiting: %v", err)
	}
	firstCall := agent.callByID(first.CallID)
	secondCall := agent.callByID(second.CallID)
	if firstCall == nil || secondCall == nil {
		t.Fatal("both calls should stay registered")
	}
	if firstCall.CallState() != callstate.StateConnected || secondCall.CallState() != callstate.StateConnected {
		t.Fatalf("states first=%s second=%s", firstCall.CallState(), secondCall.CallState())
	}
	if agent.ActiveCall() != secondCall {
		t.Fatal("answered waiting call should become current")
	}
	if err := agent.SwitchCall(first.CallID); err != nil {
		t.Fatalf("switch back: %v", err)
	}
	if agent.ActiveCall() != firstCall || agent.callByID(second.CallID) == nil {
		t.Fatal("switching dropped the other live call")
	}
}

func TestThirdCallIsBusyWhileTwoAreLive(t *testing.T) {
	agent := startedVoiceAgent(t)
	firstPeer := listenVoiceUDP(t)
	first := inboundAgentInvite("call-1", firstPeer, &capturedVoiceResponder{localTag: "t1"})
	if _, err := agent.HandleInboundVoiceRequest(first); err != nil {
		t.Fatal(err)
	}
	client1 := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(first.CallID, voiceSDP(client1.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	secondPeer := listenVoiceUDP(t)
	second := inboundAgentInvite("call-2", secondPeer, &capturedVoiceResponder{localTag: "t2"})
	if _, err := agent.HandleInboundVoiceRequest(second); err != nil {
		t.Fatal(err)
	}
	client2 := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(second.CallID, voiceSDP(client2.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	thirdPeer := listenVoiceUDP(t)
	thirdResponder := &capturedVoiceResponder{localTag: "t3"}
	third := inboundAgentInvite("call-3", thirdPeer, thirdResponder)
	result, err := agent.HandleInboundVoiceRequest(third)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != 486 {
		t.Fatalf("third INVITE status=%d, want 486", result.StatusCode)
	}
	if _, err := agent.HandleClientInvite("+8613800000002", "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=c\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 32100 RTP/AVP 0\r\n"); err == nil {
		t.Fatal("third outbound should be busy")
	}
}

func TestSingleCallStillConnectsAndClears(t *testing.T) {
	agent := newTestAgent(t)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })
	call, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatal(err)
	}
	if !agent.IsBusy() || agent.ActiveCall() != call {
		t.Fatalf("single-call occupancy busy=%t active=%v", agent.IsBusy(), agent.ActiveCall())
	}
	if err := agent.HangupCurrent(call.CallID()); err != nil {
		t.Fatal(err)
	}
	if agent.IsBusy() || agent.ActiveCall() != nil {
		t.Fatal("single-call hangup left occupancy")
	}
}

func TestSwitchMissingCallFails(t *testing.T) {
	agent := newTestAgent(t)
	if err := agent.SwitchCall("missing"); err == nil {
		t.Fatal("switch missing call succeeded")
	}
}
