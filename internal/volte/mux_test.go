package volte

import (
	"context"
	"testing"

	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
)

type stubBackend struct {
	name string
	last string
}

func (s *stubBackend) SubscribeIncomingCalls(func(voicehost.IncomingCall)) func() {
	return func() {}
}
func (s *stubBackend) SubscribeCallEvents(func(voicehost.CallEvent)) func() { return func() {} }
func (s *stubBackend) BeginCall(_ context.Context, req voicehost.BeginCallRequest) (voicehost.CallSnapshot, error) {
	s.last = req.DeviceID
	return voicehost.CallSnapshot{CallID: s.name + "-" + req.DeviceID, DeviceID: req.DeviceID}, nil
}
func (s *stubBackend) ActiveCall(string) *voicehost.CallSnapshot { return nil }
func (s *stubBackend) AnswerIncomingCall(context.Context, voicehost.AnswerRequest) (voicehost.AnswerResult, error) {
	return voicehost.AnswerResult{}, nil
}
func (s *stubBackend) RejectIncomingCall(voicehost.RejectRequest) error { return nil }
func (s *stubBackend) HangupCall(context.Context, string, string) error { return nil }
func (s *stubBackend) SendCallDTMF(string, string, string) error        { return nil }
func (s *stubBackend) StartCallCapture(string, string, string) error    { return nil }
func (s *stubBackend) DeviceStatus(string) map[string]interface{} {
	return map[string]interface{}{"backend": s.name}
}

func TestMuxRoutesByMode(t *testing.T) {
	ims := &stubBackend{name: "ims"}
	native := &stubBackend{name: "native"}
	mux := &Mux{IMS: ims, Native: native, IsNative: func(id string) bool { return id == "wwan1" }}

	got, err := mux.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan0"})
	if err != nil || got.CallID != "ims-wwan0" {
		t.Fatalf("wifi path: %+v err=%v", got, err)
	}
	got, err = mux.BeginCall(context.Background(), voicehost.BeginCallRequest{DeviceID: "wwan1"})
	if err != nil || got.CallID != "native-wwan1" {
		t.Fatalf("volte path: %+v err=%v", got, err)
	}
}

func TestMapQMIState(t *testing.T) {
	state, event := mapQMIState(qmiCallConversation)
	if state != "connected" || event != "CallAnswered" {
		t.Fatalf("conversation -> %s %s", state, event)
	}
	state, event = mapQMIState(qmiCallEnd)
	if state != "completed" || event != "CallEnded" {
		t.Fatalf("end -> %s %s", state, event)
	}
	state, event = mapQMIState(qmiCallOriginating)
	if state != "calling" || event != "CallRinging" {
		t.Fatalf("originating -> %s %s", state, event)
	}
}
