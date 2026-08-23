package volte

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
)

const (
	qmiCallIncoming     qmi.VoiceCallState     = 0x01
	qmiCallOriginating  qmi.VoiceCallState     = 0x02
	qmiCallAlerting     qmi.VoiceCallState     = 0x03
	qmiCallConversation qmi.VoiceCallState     = 0x04
	qmiCallEnd          qmi.VoiceCallState     = 0x08
	qmiDirMO            qmi.VoiceCallDirection = 0x01
	qmiDirMT            qmi.VoiceCallDirection = 0x02
)

type voiceSession struct {
	mu           sync.Mutex
	active       map[string]nativeCall
	emitted      map[string]map[string]bool
	incomingSeen map[string]bool
	attached     bool
	incoming     []func(voicehost.IncomingCall)
	events       []func(voicehost.CallEvent)
}

type nativeCall struct {
	ID        string
	QMI       uint8
	Direction string
	Peer      string
	State     string
	Start     time.Time
}

func newVoiceSession() *voiceSession {
	return &voiceSession{
		active:       make(map[string]nativeCall),
		emitted:      make(map[string]map[string]bool),
		incomingSeen: make(map[string]bool),
	}
}

func (c *Controller) attachVoice(deviceID string) {
	c.mu.Lock()
	s := c.sess[deviceID]
	if s == nil {
		c.mu.Unlock()
		return
	}
	if s.voice == nil {
		s.voice = newVoiceSession()
	}
	vs := s.voice
	first := !vs.attached
	vs.attached = true
	c.mu.Unlock()
	if first {
		_ = c.host.OnVoiceStatus(deviceID, func(info *qmi.VoiceAllCallInfo) {
			c.handleVoiceInfo(deviceID, vs, info)
		})
	}
	c.ReconcileCalls(context.Background(), deviceID)
}

func (c *Controller) ReconcileCalls(ctx context.Context, deviceID string) {
	if c == nil || c.host == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	info, err := c.host.VOICEGetAllCallInfo(ctx, deviceID)
	if err != nil {
		return
	}
	c.mu.Lock()
	s := c.sess[deviceID]
	var vs *voiceSession
	if s != nil {
		if s.voice == nil {
			s.voice = newVoiceSession()
		}
		vs = s.voice
	}
	c.mu.Unlock()
	if vs == nil {
		return
	}
	c.handleVoiceInfo(deviceID, vs, info)
}

func (c *Controller) SubscribeIncomingCalls(handler func(voicehost.IncomingCall)) func() {
	return c.addIncoming("", handler)
}

func (c *Controller) addIncoming(deviceID string, handler func(voicehost.IncomingCall)) func() {
	if c == nil || handler == nil {
		return func() {}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if deviceID == "" {
		c.globalIncoming = append(c.globalIncoming, handler)
		return func() {}
	}
	s := c.ensureLocked(deviceID)
	s.voice.incoming = append(s.voice.incoming, handler)
	return func() {}
}

func (c *Controller) SubscribeCallEvents(handler func(voicehost.CallEvent)) func() {
	if c == nil || handler == nil {
		return func() {}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.globalEvents = append(c.globalEvents, handler)
	return func() {}
}

func (c *Controller) ensureLocked(deviceID string) *session {
	s := c.sess[deviceID]
	if s == nil {
		s = &session{status: Status{DeviceID: deviceID, Phase: PhaseIdle}}
		c.sess[deviceID] = s
	}
	if s.voice == nil {
		s.voice = newVoiceSession()
	}
	return s
}

func (c *Controller) BeginCall(ctx context.Context, request voicehost.BeginCallRequest) (voicehost.CallSnapshot, error) {
	deviceID := strings.TrimSpace(request.DeviceID)
	if c == nil || c.host == nil {
		return voicehost.CallSnapshot{}, errors.New("volte: controller is not configured")
	}
	st := c.Status(deviceID)
	if !st.IMSEnabled || !st.VoLTEEnabled {
		return voicehost.CallSnapshot{}, errors.New("volte: native IMS is not enabled")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	qmiID, err := c.host.VOICEDial(ctx, deviceID, request.Callee)
	if err != nil {
		return voicehost.CallSnapshot{}, err
	}
	id := callID(deviceID, qmiID)
	now := time.Now()
	nc := nativeCall{ID: id, QMI: qmiID, Direction: "outbound", Peer: request.Callee, State: "calling", Start: now}
	c.storeCall(deviceID, nc)
	c.mu.Lock()
	vs := (*voiceSession)(nil)
	if s := c.sess[deviceID]; s != nil {
		vs = s.voice
	}
	c.mu.Unlock()
	if vs != nil {
		vs.markEmitted(id, rankKey("calling"))
	}
	c.emitEvent(deviceID, voicehost.CallEvent{
		Type: "CallRinging", DeviceID: deviceID, CallID: id, Callee: request.Callee,
		Direction: "outbound", State: "calling", Time: now,
		RecordingError: audioError(st),
	})
	return voicehost.CallSnapshot{
		CallID: id, DeviceID: deviceID, State: "calling", Direction: "outbound",
		Peer: request.Callee, StartTime: now,
	}, nil
}

func (c *Controller) ActiveCall(deviceID string) *voicehost.CallSnapshot {
	call, ok := c.lookupActive(strings.TrimSpace(deviceID))
	if !ok {
		return nil
	}
	snap := voicehost.CallSnapshot{
		CallID: call.ID, DeviceID: deviceID, State: call.State, Direction: call.Direction,
		Peer: call.Peer, StartTime: call.Start, Duration: time.Since(call.Start),
	}
	return &snap
}

func (c *Controller) HangupCall(ctx context.Context, deviceID, id string) error {
	call, ok := c.lookup(deviceID, id)
	if !ok {
		return fmt.Errorf("volte: call %s not found", id)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return c.host.VOICEHangup(ctx, deviceID, call.QMI)
}

func (c *Controller) AnswerIncomingCall(ctx context.Context, request voicehost.AnswerRequest) (voicehost.AnswerResult, error) {
	call, ok := c.lookup(request.DeviceID, request.CallID)
	if !ok {
		return voicehost.AnswerResult{}, fmt.Errorf("volte: call %s not found", request.CallID)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.host.VOICEAnswer(ctx, request.DeviceID, call.QMI); err != nil {
		return voicehost.AnswerResult{}, err
	}
	return voicehost.AnswerResult{CallID: call.ID, State: "connected"}, nil
}

func (c *Controller) RejectIncomingCall(request voicehost.RejectRequest) error {
	call, ok := c.lookup(request.DeviceID, request.CallID)
	if !ok {
		return fmt.Errorf("volte: call %s not found", request.CallID)
	}
	return c.host.VOICEHangup(context.Background(), request.DeviceID, call.QMI)
}

func (c *Controller) SendCallDTMF(deviceID, id, digit string) error {
	call, ok := c.lookup(deviceID, id)
	if !ok {
		return fmt.Errorf("volte: call %s not found", id)
	}
	return c.host.VOICEBurstDTMF(context.Background(), deviceID, call.QMI, digit)
}

func (c *Controller) StartCallCapture(string, string, string) error {
	return errors.New("volte: capture is unavailable until UAC audio is present")
}

func (c *Controller) DeviceStatus(deviceID string) map[string]interface{} {
	st := c.Status(deviceID)
	return map[string]interface{}{
		"device_id":  deviceID,
		"ready":      st.Ready(),
		"registered": st.IMSRegistered,
		"phase":      st.Phase,
		"backend":    "native_volte",
	}
}

func (c *Controller) handleVoiceInfo(deviceID string, vs *voiceSession, info *qmi.VoiceAllCallInfo) {
	if info == nil || vs == nil {
		return
	}
	now := time.Now()
	seen := make(map[string]bool, len(info.Calls))
	for _, item := range info.Calls {
		id := callID(deviceID, item.ID)
		seen[id] = true
		peer := remoteNumber(info, item.ID)
		state, eventType := mapQMIState(item.State)
		dir := "outbound"
		if item.Direction == qmiDirMT {
			dir = "inbound"
		}
		prev, existed := vs.get(id)
		if existed && stateRank(state) < stateRank(prev.State) && stateRank(state) != rankTerminal {
			continue
		}
		if peer == "" {
			peer = prev.Peer
		}
		if dir == "outbound" && prev.Direction != "" {
			dir = prev.Direction
		}
		next := nativeCall{ID: id, QMI: item.ID, Direction: dir, Peer: peer, State: state, Start: startedAt(prev, now)}
		vs.put(next)
		if dir == "inbound" && vs.markIncoming(id) {
			c.emitIncoming(deviceID, voicehost.IncomingCall{
				DeviceID: deviceID, CallID: id, Caller: peer, State: "ringing", ReceivedAt: now,
			})
		}
		if eventType == "" || !vs.markEmitted(id, rankKey(state)) {
			continue
		}
		c.emitEvent(deviceID, voicehost.CallEvent{
			Type: eventType, DeviceID: deviceID, CallID: id, Caller: peer, Callee: peer,
			Direction: dir, State: state, Time: now, RecordingError: audioError(c.Status(deviceID)),
		})
	}
	for _, call := range vs.list() {
		if seen[call.ID] || stateRank(call.State) == rankTerminal {
			continue
		}
		call.State = "completed"
		vs.put(call)
		if vs.markEmitted(call.ID, rankKey("completed")) {
			c.emitEvent(deviceID, voicehost.CallEvent{
				Type: "CallEnded", DeviceID: deviceID, CallID: call.ID, Caller: call.Peer, Callee: call.Peer,
				Direction: call.Direction, State: "completed", Time: now, RecordingError: audioError(c.Status(deviceID)),
			})
		}
	}
}

const rankTerminal = 4

func mapQMIState(state qmi.VoiceCallState) (string, string) {
	switch state {
	case qmiCallOriginating:
		return "calling", "CallRinging"
	case qmiCallIncoming, qmiCallAlerting:
		return "ringing", "CallRinging"
	case qmiCallConversation:
		return "connected", "CallAnswered"
	case qmiCallEnd:
		return "completed", "CallEnded"
	default:
		return "calling", ""
	}
}

func rankKey(state string) string {
	return fmt.Sprintf("%d", stateRank(state))
}

func stateRank(state string) int {
	switch state {
	case "calling":
		return 1
	case "ringing":
		return 2
	case "connected":
		return 3
	case "completed", "busy", "rejected", "failed":
		return rankTerminal
	default:
		return 0
	}
}

func callID(deviceID string, qmiID uint8) string {
	return fmt.Sprintf("volte-%s-%d", deviceID, qmiID)
}

func remoteNumber(info *qmi.VoiceAllCallInfo, id uint8) string {
	for _, n := range info.RemotePartyNumbers {
		if n.CallID == id {
			return n.Number
		}
	}
	return ""
}

func startedAt(prev nativeCall, now time.Time) time.Time {
	if !prev.Start.IsZero() {
		return prev.Start
	}
	return now
}

func audioError(st Status) string {
	if strings.TrimSpace(st.AudioDevice) != "" && st.UACEnabled && !st.RebootRequired {
		return ""
	}
	if st.RebootRequired {
		return "VoLTE 音频需要 UAC，模组可能要重启后才有声卡"
	}
	return "VoLTE 音频不可用：未检测到 UAC 声卡"
}

func (c *Controller) storeCall(deviceID string, call nativeCall) {
	c.mu.Lock()
	s := c.ensureLocked(deviceID)
	s.voice.put(call)
	c.mu.Unlock()
}

func (c *Controller) lookup(deviceID, id string) (nativeCall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sess[strings.TrimSpace(deviceID)]
	if s == nil || s.voice == nil {
		return nativeCall{}, false
	}
	return s.voice.get(id)
}

func (c *Controller) lookupActive(deviceID string) (nativeCall, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sess[deviceID]
	if s == nil || s.voice == nil {
		return nativeCall{}, false
	}
	for _, call := range s.voice.active {
		if call.State != "completed" {
			return call, true
		}
	}
	return nativeCall{}, false
}

func (c *Controller) emitEvent(deviceID string, event voicehost.CallEvent) {
	c.mu.Lock()
	handlers := append([]func(voicehost.CallEvent){}, c.globalEvents...)
	if s := c.sess[deviceID]; s != nil && s.voice != nil {
		handlers = append(handlers, s.voice.events...)
	}
	c.mu.Unlock()
	for _, h := range handlers {
		h(event)
	}
}

func (c *Controller) emitIncoming(deviceID string, call voicehost.IncomingCall) {
	c.mu.Lock()
	handlers := append([]func(voicehost.IncomingCall){}, c.globalIncoming...)
	if s := c.sess[deviceID]; s != nil && s.voice != nil {
		handlers = append(handlers, s.voice.incoming...)
	}
	c.mu.Unlock()
	for _, h := range handlers {
		h(call)
	}
}

func (vs *voiceSession) put(call nativeCall) {
	vs.mu.Lock()
	if vs.active == nil {
		vs.active = make(map[string]nativeCall)
	}
	vs.active[call.ID] = call
	vs.mu.Unlock()
}

func (vs *voiceSession) get(id string) (nativeCall, bool) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	call, ok := vs.active[id]
	return call, ok
}

func (vs *voiceSession) list() []nativeCall {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	out := make([]nativeCall, 0, len(vs.active))
	for _, call := range vs.active {
		out = append(out, call)
	}
	return out
}

func (vs *voiceSession) markEmitted(id, eventType string) bool {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.emitted == nil {
		vs.emitted = make(map[string]map[string]bool)
	}
	if vs.emitted[id] == nil {
		vs.emitted[id] = make(map[string]bool)
	}
	if vs.emitted[id][eventType] {
		return false
	}
	vs.emitted[id][eventType] = true
	return true
}

func (vs *voiceSession) markIncoming(id string) bool {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.incomingSeen == nil {
		vs.incomingSeen = make(map[string]bool)
	}
	if vs.incomingSeen[id] {
		return false
	}
	vs.incomingSeen[id] = true
	return true
}
