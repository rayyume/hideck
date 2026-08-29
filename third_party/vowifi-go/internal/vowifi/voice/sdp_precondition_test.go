package voice

import (
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestSDPPreconditionsSatisfied(t *testing.T) {
	if !sdpPreconditionsSatisfied("v=0\r\nm=audio 9 RTP/AVP 0\r\n") {
		t.Fatal("SDP without qos should be vacuously met")
	}
	unmet := "a=curr:qos local none\r\na=curr:qos remote none\r\na=des:qos mandatory local sendrecv\r\n"
	if sdpPreconditionsSatisfied(unmet) {
		t.Fatal("unmet qos was treated as satisfied")
	}
	met := "a=curr:qos local sendrecv\r\na=curr:qos remote sendrecv\r\n"
	if !sdpPreconditionsSatisfied(met) {
		t.Fatal("sendrecv qos was not satisfied")
	}
	if !sdpPreconditionsSatisfied(sdpQoSReservedLocal) {
		t.Fatal("VoWiFi reserved local qos must already be satisfied")
	}
}

func TestAdvertiseEstablishedSessionQoSMatches24610HoldOffer(t *testing.T) {
	got := advertiseEstablishedSessionQoS(sdpQoSReservedLocal + "a=sendrecv\r\n")
	if !strings.Contains(got, "a=curr:qos remote sendrecv") {
		t.Fatalf("established qos missing remote sendrecv: %q", got)
	}
	if strings.Contains(got, "a=curr:qos remote none") {
		t.Fatalf("established qos kept first-offer remote none: %q", got)
	}
	if !strings.Contains(got, "a=curr:qos local sendrecv") ||
		!strings.Contains(got, "a=des:qos mandatory local sendrecv") {
		t.Fatalf("established qos dropped local reservation: %q", got)
	}
	if again := advertiseEstablishedSessionQoS(got); again != got {
		t.Fatalf("established qos rewrite is not idempotent: %q / %q", got, again)
	}
}

func TestHoldOfferUses24610EstablishedRemoteQoS(t *testing.T) {
	agent := newTestAgent(t)
	call := NewCall(agent, callstate.DirectionOutbound, "call-hold-qos", "+447000000001")
	call.setLocalSDP("", string(buildBasicSDP("192.0.2.10", 12000, 1)))
	sdp := rewriteSDPDirection(
		bumpSDPOriginVersion(advertiseEstablishedSessionQoS(call.imsLocalSDPValue())),
		sdpDirectionSendOnly,
	)
	if !strings.Contains(sdp, "a=sendonly") {
		t.Fatalf("hold offer omitted sendonly: %q", sdp)
	}
	if !strings.Contains(sdp, "a=curr:qos remote sendrecv") || strings.Contains(sdp, "a=curr:qos remote none") {
		t.Fatalf("hold offer must follow TS 24.610 curr remote sendrecv: %q", sdp)
	}
}

func TestApplyCallPreconditionsMovesEarlyMediaToWait(t *testing.T) {
	agent := NewAgent("precond", nil, nil)
	call := NewCall(agent, callstate.DirectionOutbound, "precond-1", "+447700900000")
	if err := call.TransitionChecked(callstate.StateCalling); err != nil {
		t.Fatal(err)
	}
	if err := call.TransitionChecked(callstate.StateEarlyMedia); err != nil {
		t.Fatal(err)
	}
	agent.applyCallPreconditions(call, "a=curr:qos local none\r\na=curr:qos remote none\r\n")
	if call.CallState() != callstate.StatePreconditionWait || call.preconditionMetValue() {
		t.Fatalf("state=%s met=%v", call.CallState(), call.preconditionMetValue())
	}
	agent.applyCallPreconditions(call, "a=curr:qos remote sendrecv\r\n")
	if call.CallState() != callstate.StateEarlyMedia || !call.preconditionMetValue() {
		t.Fatalf("after met state=%s met=%v", call.CallState(), call.preconditionMetValue())
	}
}
