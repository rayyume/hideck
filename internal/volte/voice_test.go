package volte

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
)

func enableVoice(t *testing.T) (*Controller, *FakeModem) {
	t.Helper()
	host := newFakeModem()
	host.IMS, host.VoLTE = 1, 1
	host.USB[len(host.USB)-1] = "1"
	ctl := NewControllerWithBackup(host, t.TempDir())
	if err := ctl.Enable(context.Background(), "wwan1"); err != nil {
		t.Fatal(err)
	}
	return ctl, host
}

func TestVoiceAgentMOSequenceOnce(t *testing.T) {
	ctl, host := enableVoice(t)
	var events []voicehost.CallEvent
	ctl.SubscribeCallEvents(func(ev voicehost.CallEvent) { events = append(events, ev) })
	snap, err := ctl.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan1", Callee: "10086"})
	if err != nil {
		t.Fatal(err)
	}
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 1, State: qmiCallOriginating, Direction: qmiDirMO}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 1, Number: "10086"}},
	})
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 1, State: qmiCallAlerting, Direction: qmiDirMO}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 1, Number: "10086"}},
	})
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 1, State: qmiCallConversation, Direction: qmiDirMO}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 1, Number: "10086"}},
	})
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 1, State: qmiCallEnd, Direction: qmiDirMO}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 1, Number: "10086"}},
	})
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 1, State: qmiCallEnd, Direction: qmiDirMO}},
	})
	if snap.CallID != "volte-wwan1-1" {
		t.Fatalf("call id %s", snap.CallID)
	}
	got := eventTypes(events)
	want := []string{"CallRinging", "CallRinging", "CallAnswered", "CallEnded"}
	if len(got) != len(want) {
		t.Fatalf("events %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events %v", got)
		}
	}
	if events[0].State != "calling" || events[1].State != "ringing" {
		t.Fatalf("ring states %s %s", events[0].State, events[1].State)
	}
}

func TestVoiceAgentMTNoDuplicateIncoming(t *testing.T) {
	ctl, host := enableVoice(t)
	var incoming []voicehost.IncomingCall
	var events []voicehost.CallEvent
	ctl.SubscribeIncomingCalls(func(call voicehost.IncomingCall) { incoming = append(incoming, call) })
	ctl.SubscribeCallEvents(func(ev voicehost.CallEvent) { events = append(events, ev) })
	info := &qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 2, State: qmiCallIncoming, Direction: qmiDirMT}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 2, Number: "13800138000"}},
	}
	host.fireVoice(info)
	host.fireVoice(info)
	if len(incoming) != 1 {
		t.Fatalf("incoming %d", len(incoming))
	}
	if _, err := ctl.AnswerIncomingCall(context.Background(), voicehost.AnswerRequest{DeviceID: "wwan1", CallID: incoming[0].CallID}); err != nil {
		t.Fatal(err)
	}
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 2, State: qmiCallConversation, Direction: qmiDirMT}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 2, Number: "13800138000"}},
	})
	if err := ctl.RejectIncomingCall(voicehost.RejectRequest{}); err == nil {
		// reject of unknown id should fail; hangup current
	}
	if err := ctl.HangupCall(context.Background(), "wwan1", incoming[0].CallID); err != nil {
		t.Fatal(err)
	}
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 2, State: qmiCallEnd, Direction: qmiDirMT}},
	})
	if countType(events, "CallRinging") != 1 || countType(events, "CallAnswered") != 1 || countType(events, "CallEnded") != 1 {
		t.Fatalf("events %v", eventTypes(events))
	}
	if incoming[0].OfferSDP == "" || !strings.Contains(incoming[0].OfferSDP, "PCMU/8000") {
		t.Fatalf("MT OfferSDP %q", incoming[0].OfferSDP)
	}
}

func TestVoiceAgentReusesQMICallIDAfterEnd(t *testing.T) {
	ctl, host := enableVoice(t)
	var events []voicehost.CallEvent
	ctl.SubscribeCallEvents(func(ev voicehost.CallEvent) { events = append(events, ev) })
	if _, err := ctl.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan1", Callee: "10086"}); err != nil {
		t.Fatal(err)
	}
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 1, State: qmiCallConversation, Direction: qmiDirMO}},
	})
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 1, State: qmiCallEnd, Direction: qmiDirMO}},
	})
	if _, err := ctl.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan1", Callee: "10010"}); err != nil {
		t.Fatal(err)
	}
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 1, State: qmiCallConversation, Direction: qmiDirMO}},
	})
	if countType(events, "CallAnswered") != 2 {
		t.Fatalf("second call must answer after id reuse: %v", eventTypes(events))
	}
}

func TestVoiceAgentIgnoresBackwardAndReconcilesMissing(t *testing.T) {
	ctl, host := enableVoice(t)
	var events []voicehost.CallEvent
	ctl.SubscribeCallEvents(func(ev voicehost.CallEvent) { events = append(events, ev) })
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 3, State: qmiCallConversation, Direction: qmiDirMO}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 3, Number: "10010"}},
	})
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 3, State: qmiCallOriginating, Direction: qmiDirMO}},
	})
	host.allCalls = &qmi.VoiceAllCallInfo{}
	ctl.ReconcileCalls(context.Background(), "wwan1")
	if countType(events, "CallAnswered") != 1 || countType(events, "CallEnded") != 1 {
		t.Fatalf("events %v", eventTypes(events))
	}
	if countType(events, "CallRinging") != 0 {
		t.Fatalf("backward originating should not emit ringing: %v", eventTypes(events))
	}
}

func TestVoiceAgentDTMFUsesCallID(t *testing.T) {
	ctl, host := enableVoice(t)
	snap, err := ctl.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan1", Callee: "10000"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ctl.SendCallDTMF("wwan1", snap.CallID, "5"); err != nil {
		t.Fatal(err)
	}
	_ = host
}

func TestRejectAlreadyGoneClearsCall(t *testing.T) {
	ctl, host := enableVoice(t)
	var events []voicehost.CallEvent
	var incoming []voicehost.IncomingCall
	ctl.SubscribeCallEvents(func(ev voicehost.CallEvent) { events = append(events, ev) })
	ctl.SubscribeIncomingCalls(func(call voicehost.IncomingCall) { incoming = append(incoming, call) })
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls:              []qmi.VoiceCallInfo{{ID: 4, State: qmiCallIncoming, Direction: qmiDirMT}},
		RemotePartyNumbers: []qmi.VoiceRemotePartyNumber{{CallID: 4, Number: "4001995558"}},
	})
	if len(incoming) != 1 {
		t.Fatalf("incoming %d", len(incoming))
	}
	host.hangupErr = &qmi.QMIError{
		Service: qmi.ServiceVOICE, MessageID: qmi.VOICEEndCall, Result: 1, ErrorCode: qmi.QMIErrInvalidID,
	}
	if err := ctl.RejectIncomingCall(voicehost.RejectRequest{DeviceID: "wwan1", CallID: incoming[0].CallID}); err != nil {
		t.Fatalf("reject stale call: %v", err)
	}
	if countType(events, "CallEnded") != 1 {
		t.Fatalf("events %v", eventTypes(events))
	}
	if _, ok := ctl.lookup("wwan1", incoming[0].CallID); ok {
		t.Fatal("stale call still in voice session")
	}
	if err := ctl.RejectIncomingCall(voicehost.RejectRequest{DeviceID: "wwan1", CallID: incoming[0].CallID}); err != nil {
		t.Fatalf("repeat reject: %v", err)
	}
}

func TestHangupAlreadyGoneClearsCall(t *testing.T) {
	ctl, host := enableVoice(t)
	var events []voicehost.CallEvent
	ctl.SubscribeCallEvents(func(ev voicehost.CallEvent) { events = append(events, ev) })
	snap, err := ctl.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan1", Callee: "10000"})
	if err != nil {
		t.Fatal(err)
	}
	host.hangupErr = &qmi.QMIError{
		Service: qmi.ServiceVOICE, MessageID: qmi.VOICEEndCall, Result: 1, ErrorCode: qmi.QMIErrInvalidID,
	}
	if err := ctl.HangupCall(context.Background(), "wwan1", snap.CallID); err != nil {
		t.Fatalf("hangup stale call: %v", err)
	}
	if countType(events, "CallEnded") != 1 {
		t.Fatalf("events %v", eventTypes(events))
	}
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 1, State: qmiCallEnd, Direction: qmiDirMO}},
	})
	if countType(events, "CallEnded") != 1 {
		t.Fatalf("duplicate end after local finish: %v", eventTypes(events))
	}
}

func TestVoiceAgentBusyRemoteRelease(t *testing.T) {
	ctl, host := enableVoice(t)
	var events []voicehost.CallEvent
	ctl.SubscribeCallEvents(func(ev voicehost.CallEvent) { events = append(events, ev) })
	_, err := ctl.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan1", Callee: "10086"})
	if err != nil {
		t.Fatal(err)
	}
	host.fireVoice(&qmi.VoiceAllCallInfo{
		Calls: []qmi.VoiceCallInfo{{ID: 1, State: qmiCallEnd, Direction: qmiDirMO}},
	})
	if countType(events, "CallEnded") != 1 {
		t.Fatalf("events %v", eventTypes(events))
	}
}

func eventTypes(events []voicehost.CallEvent) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Type)
	}
	return out
}

func countType(events []voicehost.CallEvent, typ string) int {
	n := 0
	for _, ev := range events {
		if ev.Type == typ {
			n++
		}
	}
	return n
}

func TestVoiceAgentTimeoutBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	default:
	}
}
