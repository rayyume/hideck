package voice

import (
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
