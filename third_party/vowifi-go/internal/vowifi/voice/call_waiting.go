package voice

import (
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

const defaultTUECWTimeout = 30 * time.Second

func (a *Agent) tueCWTimeout() time.Duration {
	if a != nil && a.cwTimeout > 0 {
		return a.cwTimeout
	}
	return defaultTUECWTimeout
}

func (a *Agent) startTUECWTimer(call *Call) {
	if a == nil || call == nil {
		return
	}
	timeout := a.tueCWTimeout()
	call.mu.Lock()
	if call.cwTimer != nil {
		call.cwTimer.Stop()
	}
	call.cwTimer = time.AfterFunc(timeout, func() {
		call.inboundDecisionMu.Lock()
		defer call.inboundDecisionMu.Unlock()
		if call.CallState() != callstate.StateRinging || !call.waitingIndication {
			return
		}
		cause := a.sendInboundTimeout(call)
		a.releaseInboundCall(call, cause, false)
	})
	call.mu.Unlock()
}

func (c *Call) stopTUECWTimer() {
	if c == nil {
		return
	}
	c.mu.Lock()
	timer := c.cwTimer
	c.cwTimer = nil
	c.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}
