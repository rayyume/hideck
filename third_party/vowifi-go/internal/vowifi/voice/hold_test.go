package voice

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestBuildIMSReinviteUsesHoldDirection(t *testing.T) {
	agent := newTestAgent(t)
	call := NewCall(agent, callstate.DirectionOutbound, "call-reinvite", "+8613800000000")
	call.setVoiceDialog(&voiceSIPDialog{
		localURI: "sip:local@ims.example", remoteURI: "sip:peer@ims.example",
		remoteTarget: "sip:peer@edge.example", localAddress: "192.0.2.10:5060",
		transport: "tcp", localTag: "local", remoteTag: "remote", cseq: 7, inviteCSeq: 7,
	})
	call.applyVoiceSessionExpires("120;refresher=uac")
	sdp := rewriteSDPDirection(testClientSDP, sdpDirectionSendOnly)
	request := buildIMSReinvite(agent, call, sdp)
	if !strings.HasPrefix(request, "INVITE sip:peer@edge.example SIP/2.0") {
		t.Fatalf("request line = %q", strings.Split(request, "\r\n")[0])
	}
	if voiceTestHeader(request, "CSeq") != "8 INVITE" || voiceTestHeader(request, "Session-Expires") != "120;refresher=uac" {
		t.Fatalf("re-INVITE headers = CSeq %q Session-Expires %q", voiceTestHeader(request, "CSeq"), voiceTestHeader(request, "Session-Expires"))
	}
	if !strings.Contains(request, "a=sendonly") {
		t.Fatalf("re-INVITE omitted sendonly: %q", request)
	}
	if !strings.Contains(request, "Supported: "+voiceInviteSupported) {
		t.Fatalf("re-INVITE omitted Supported: %q", request)
	}
	if strings.Count(request, "Supported:") != 1 {
		t.Fatalf("re-INVITE duplicated Supported: %q", request)
	}
}

func TestOutboundHoldAndResumeRewriteLocalSDP(t *testing.T) {
	agent := newTestAgent(t)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })
	call, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := agent.HoldCall(ctx, call.CallID()); err != nil {
		t.Fatal(err)
	}
	if !call.Held() || sdpMediaDirection(call.imsLocalSDPValue()) != sdpDirectionSendOnly {
		t.Fatalf("held=%t direction=%s sdp=%q", call.Held(), sdpMediaDirection(call.imsLocalSDPValue()), call.imsLocalSDPValue())
	}
	if sdpHasPreconditions(call.imsLocalSDPValue()) && strings.Contains(call.imsLocalSDPValue(), "a=curr:qos remote none") {
		t.Fatalf("hold SDP kept first-offer remote none: %q", call.imsLocalSDPValue())
	}
	if err := agent.ResumeCall(ctx, call.CallID()); err != nil {
		t.Fatal(err)
	}
	if call.Held() || sdpMediaDirection(call.imsLocalSDPValue()) != sdpDirectionSendRecv {
		t.Fatalf("resumed held=%t direction=%s", call.Held(), sdpMediaDirection(call.imsLocalSDPValue()))
	}
}

func TestSimulatedHoldReinviteUses24610RemoteQoS(t *testing.T) {
	holdInvites := make(chan string, 2)
	registrar := startScriptedVoiceRegistrar(t, func(request string) (int, string, string) {
		if strings.HasPrefix(request, "INVITE ") {
			if strings.Contains(request, "a=sendonly") {
				holdInvites <- request
			}
			extra := "To: <sip:callee@ims.example.com>;tag=remote\r\nContact: <sip:callee@ims.example.com>\r\nContent-Type: application/sdp\r\n"
			return 200, extra, testIMSAnswerSDP
		}
		return 200, "", ""
	})
	agent := newVoiceTestAgent(t, registrar)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	call, err := agent.DialContext(ctx, "+447000000001")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	if strings.Contains(call.imsLocalSDPValue(), "a=curr:qos remote none") {
		t.Fatalf("connected local SDP still first-offer remote none: %q", call.imsLocalSDPValue())
	}
	if err := agent.HoldCall(ctx, call.CallID()); err != nil {
		t.Fatal(err)
	}
	select {
	case invite := <-holdInvites:
		if !strings.Contains(invite, "a=sendonly") {
			t.Fatalf("hold re-INVITE omitted sendonly: %q", invite)
		}
		if !strings.Contains(invite, "a=curr:qos remote sendrecv") || strings.Contains(invite, "a=curr:qos remote none") {
			t.Fatalf("hold re-INVITE must follow TS 24.610 curr remote sendrecv: %q", invite)
		}
		if !strings.Contains(invite, "a=des:qos mandatory local sendrecv") ||
			!strings.Contains(invite, "a=des:qos optional remote sendrecv") {
			t.Fatalf("hold re-INVITE dropped 24.610 qos desired: %q", invite)
		}
	case <-ctx.Done():
		t.Fatal("hold re-INVITE was not observed")
	}
}

func TestHoldReinvitePRACKsReliableProvisional(t *testing.T) {
	registrar := startReliableProvisionalRegistrar(t)
	agent := newVoiceTestAgent(t, registrar.conn)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	call, err := agent.dialContext(ctx, "+447942985429", testClientSDP)
	if err != nil {
		t.Fatalf("dialContext: %v", err)
	}
	initialInviteCSeq := call.voiceDialogSnapshot().inviteCSeq
	assertReliableProvisionalPRACK(t, <-registrar.prack, registrar.conn.LocalAddr().(*net.UDPAddr).Port, initialInviteCSeq)
	<-registrar.ack

	if err := agent.HoldCall(ctx, call.CallID()); err != nil {
		t.Fatal(err)
	}
	if !call.Held() || sdpMediaDirection(call.imsLocalSDPValue()) != sdpDirectionSendOnly {
		t.Fatalf("held=%t direction=%s", call.Held(), sdpMediaDirection(call.imsLocalSDPValue()))
	}
	holdInviteCSeq := call.voiceDialogSnapshot().inviteCSeq
	if holdInviteCSeq == initialInviteCSeq {
		t.Fatal("hold re-INVITE did not advance INVITE CSeq")
	}
	if got := voiceTestHeader(<-registrar.prack, "RAck"); got != fmt.Sprintf("42 %d INVITE", holdInviteCSeq) {
		t.Fatalf("hold RAck = %q", got)
	}

	if err := agent.ResumeCall(ctx, call.CallID()); err != nil {
		t.Fatal(err)
	}
	if call.Held() || sdpMediaDirection(call.imsLocalSDPValue()) != sdpDirectionSendRecv {
		t.Fatalf("resumed held=%t direction=%s", call.Held(), sdpMediaDirection(call.imsLocalSDPValue()))
	}
	if got := voiceTestHeader(<-registrar.prack, "RAck"); got != fmt.Sprintf("43 %d INVITE", call.voiceDialogSnapshot().inviteCSeq) {
		t.Fatalf("resume RAck = %q", got)
	}
}

func TestInboundHoldAnswersRecvonlyAndStopsLANForward(t *testing.T) {
	agent := startedVoiceAgent(t)
	firstPeer := listenVoiceUDP(t)
	initialResponder := &capturedVoiceResponder{localTag: "local-tag"}
	request := inboundAgentInvite("call-in-hold", firstPeer, initialResponder)
	if _, err := agent.HandleInboundVoiceRequest(request); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(request.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	holdPeer := listenVoiceUDP(t)
	holdResponder := &capturedVoiceResponder{localTag: "local-tag"}
	hold := inboundAgentInvite(request.CallID, holdPeer, holdResponder)
	hold.Body = []byte(voiceSDP(holdPeer.LocalAddr().(*net.UDPAddr).Port) + "a=sendonly\r\n")
	result, err := agent.HandleInboundVoiceRequest(hold)
	if err != nil || result.StatusCode != 0 || fmt.Sprint(holdResponder.statuses()) != "[200]" {
		t.Fatalf("hold re-INVITE result=%+v statuses=%v err=%v", result, holdResponder.statuses(), err)
	}
	call := agent.callByID(request.CallID)
	answer := string(holdResponder.lastResponse().Body)
	if sdpMediaDirection(answer) != sdpDirectionRecvOnly || !call.Held() {
		t.Fatalf("hold answer dir=%s held=%t body=%q", sdpMediaDirection(answer), call.Held(), answer)
	}
	clientOffer, _ := ParseSDP([]byte(call.ClientSDP()))
	writeVoicePacket(t, client, clientOffer.GetMediaPort(), []byte("held-rtp"))
	assertNoVoicePacket(t, holdPeer)
}

func assertNoVoicePacket(t *testing.T, conn *net.UDPConn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 256)
	if n, _, err := conn.ReadFromUDP(buffer); err == nil {
		t.Fatalf("unexpected packet %q", buffer[:n])
	}
}
