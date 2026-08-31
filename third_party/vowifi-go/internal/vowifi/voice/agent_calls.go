package voice

import (
	"errors"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

const maxConcurrentCalls = 2

func (a *Agent) liveCallsLocked() []*Call {
	if a == nil {
		return nil
	}
	out := make([]*Call, 0, len(a.calls))
	for _, call := range a.calls {
		if call != nil && !call.IsTerminalState() {
			out = append(out, call)
		}
	}
	return out
}

func (a *Agent) cannotAddCallLocked() bool {
	live := a.liveCallsLocked()
	if len(live) >= maxConcurrentCalls {
		return true
	}
	if len(live) == 0 {
		return false
	}
	for _, call := range live {
		if call.CallState() == callstate.StateConnected {
			return false
		}
	}
	return true
}

func (a *Agent) registerLiveCallLocked(call *Call, waiting bool) {
	if a == nil || call == nil {
		return
	}
	a.calls[call.CallID()] = call
	if waiting && a.activeCall != nil && !a.activeCall.IsTerminalState() {
		a.waitingCall = call
		return
	}
	a.activeCall = call
	if a.waitingCall == call {
		a.waitingCall = nil
	}
}

func (a *Agent) promoteLiveCallLocked() {
	if a == nil {
		return
	}
	if a.activeCall != nil && !a.activeCall.IsTerminalState() {
		return
	}
	a.activeCall = nil
	for _, call := range a.calls {
		if call != nil && !call.IsTerminalState() {
			a.activeCall = call
			if a.waitingCall == call {
				a.waitingCall = nil
			}
			return
		}
	}
}

// SwitchCall makes callID the current focus without dropping the other live call.
func (a *Agent) SwitchCall(callID string) error {
	if a == nil {
		return errors.New("voice: nil agent")
	}
	callID = strings.TrimSpace(callID)
	a.mu.Lock()
	defer a.mu.Unlock()
	call := a.calls[callID]
	if call == nil || call.IsTerminalState() {
		return errors.New("voice: call not found")
	}
	a.activeCall = call
	if a.waitingCall == call {
		a.waitingCall = nil
	}
	return nil
}
