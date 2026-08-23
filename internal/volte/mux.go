package volte

import (
	"context"
	"strings"

	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
)

type VoiceBackend interface {
	SubscribeIncomingCalls(func(voicehost.IncomingCall)) func()
	SubscribeCallEvents(func(voicehost.CallEvent)) func()
	BeginCall(context.Context, voicehost.BeginCallRequest) (voicehost.CallSnapshot, error)
	ActiveCall(deviceID string) *voicehost.CallSnapshot
	AnswerIncomingCall(context.Context, voicehost.AnswerRequest) (voicehost.AnswerResult, error)
	RejectIncomingCall(voicehost.RejectRequest) error
	HangupCall(context.Context, string, string) error
	SendCallDTMF(string, string, string) error
	StartCallCapture(string, string, string) error
	DeviceStatus(deviceID string) map[string]interface{}
}

type Mux struct {
	IMS      VoiceBackend
	Native   VoiceBackend
	IsNative func(deviceID string) bool
}

func (m *Mux) native(deviceID string) bool {
	if m == nil || m.IsNative == nil {
		return false
	}
	return m.IsNative(strings.TrimSpace(deviceID))
}

func (m *Mux) pick(deviceID string) VoiceBackend {
	if m.native(deviceID) && m.Native != nil {
		return m.Native
	}
	return m.IMS
}

func (m *Mux) SubscribeIncomingCalls(handler func(voicehost.IncomingCall)) func() {
	var unsubs []func()
	if m != nil && m.IMS != nil {
		unsubs = append(unsubs, m.IMS.SubscribeIncomingCalls(handler))
	}
	if m != nil && m.Native != nil {
		unsubs = append(unsubs, m.Native.SubscribeIncomingCalls(handler))
	}
	return func() {
		for _, u := range unsubs {
			if u != nil {
				u()
			}
		}
	}
}

func (m *Mux) SubscribeCallEvents(handler func(voicehost.CallEvent)) func() {
	var unsubs []func()
	if m != nil && m.IMS != nil {
		unsubs = append(unsubs, m.IMS.SubscribeCallEvents(handler))
	}
	if m != nil && m.Native != nil {
		unsubs = append(unsubs, m.Native.SubscribeCallEvents(handler))
	}
	return func() {
		for _, u := range unsubs {
			if u != nil {
				u()
			}
		}
	}
}

func (m *Mux) BeginCall(ctx context.Context, request voicehost.BeginCallRequest) (voicehost.CallSnapshot, error) {
	return m.pick(request.DeviceID).BeginCall(ctx, request)
}

func (m *Mux) ActiveCall(deviceID string) *voicehost.CallSnapshot {
	return m.pick(deviceID).ActiveCall(deviceID)
}

func (m *Mux) AnswerIncomingCall(ctx context.Context, request voicehost.AnswerRequest) (voicehost.AnswerResult, error) {
	return m.pick(request.DeviceID).AnswerIncomingCall(ctx, request)
}

func (m *Mux) RejectIncomingCall(request voicehost.RejectRequest) error {
	return m.pick(request.DeviceID).RejectIncomingCall(request)
}

func (m *Mux) HangupCall(ctx context.Context, deviceID, callID string) error {
	return m.pick(deviceID).HangupCall(ctx, deviceID, callID)
}

func (m *Mux) SendCallDTMF(deviceID, callID, digit string) error {
	return m.pick(deviceID).SendCallDTMF(deviceID, callID, digit)
}

func (m *Mux) StartCallCapture(deviceID, callID, basePath string) error {
	return m.pick(deviceID).StartCallCapture(deviceID, callID, basePath)
}

func (m *Mux) DeviceStatus(deviceID string) map[string]interface{} {
	backend := m.pick(deviceID)
	if backend == nil {
		return map[string]interface{}{"device_id": deviceID, "ready": false}
	}
	return backend.DeviceStatus(deviceID)
}
