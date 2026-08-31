package voice

import (
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestReservedLocalOfferStartsWithSatisfiedLocalQoS(t *testing.T) {
	if !sdpPreconditionsSatisfied(sdpQoSReservedLocal) {
		t.Fatal("reserved-local first offer marks available Wi-Fi resources as unavailable")
	}
	if strings.Contains(sdpQoSReservedLocal, "a=curr:qos local none") {
		t.Fatal("reserved-local offer advertised unmet local qos")
	}
}

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
	agent.applyCallPreconditions(call, "a=curr:qos local none\r\na=curr:qos remote none\r\n"+
		"a=des:qos mandatory local sendrecv\r\n")
	if call.CallState() != callstate.StatePreconditionWait || call.preconditionMetValue() {
		t.Fatalf("state=%s met=%v", call.CallState(), call.preconditionMetValue())
	}
	agent.applyCallPreconditions(call, "a=curr:qos remote sendrecv\r\n"+
		"a=des:qos mandatory remote sendrecv\r\n")
	if call.CallState() != callstate.StateEarlyMedia || !call.preconditionMetValue() {
		t.Fatalf("after met state=%s met=%v", call.CallState(), call.preconditionMetValue())
	}
}

func TestSDPPreconditionsRequireEveryMandatoryStatus(t *testing.T) {
	unmet := "a=curr:qos local sendrecv\r\na=curr:qos remote none\r\n" +
		"a=des:qos mandatory local sendrecv\r\na=des:qos mandatory remote sendrecv\r\n"
	if sdpPreconditionsSatisfied(unmet) {
		t.Fatal("one satisfied mandatory segment hid an unmet segment")
	}
	optionalRemote := strings.Replace(unmet, "mandatory remote", "optional remote", 1)
	if !sdpPreconditionsSatisfied(optionalRemote) {
		t.Fatal("optional remote reservation blocked session establishment")
	}
}

func TestEnsureOriginatingPreconditionsAddsAudioAttributes(t *testing.T) {
	raw := "v=0\r\nm=audio 12000 RTP/AVP 104\r\na=rtpmap:104 AMR-WB/16000\r\n" +
		"m=video 12002 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\n"
	got := ensureOriginatingPreconditions(raw)
	audioQoS := strings.Index(got, "a=curr:qos local sendrecv")
	video := strings.Index(got, "m=video")
	if audioQoS < 0 || video < audioQoS {
		t.Fatalf("audio preconditions were not inserted before the next media section: %q", got)
	}
	if again := ensureOriginatingPreconditions(got); again != got {
		t.Fatal("precondition insertion is not idempotent")
	}
}
