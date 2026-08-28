package voice

import (
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func sdpHasPreconditions(sdp string) bool {
	for _, line := range strings.Split(sdp, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if strings.HasPrefix(strings.ToLower(line), "a=curr:qos") ||
			strings.HasPrefix(strings.ToLower(line), "a=des:qos") {
			return true
		}
	}
	return false
}

func sdpQoSCurrent(sdp, side string) string {
	prefix := "a=curr:qos " + strings.ToLower(strings.TrimSpace(side)) + " "
	for _, line := range strings.Split(sdp, "\n") {
		line = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(line, "\r")))
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func sdpPreconditionsSatisfied(remoteSDP string) bool {
	if !sdpHasPreconditions(remoteSDP) {
		return true
	}
	remote := sdpQoSCurrent(remoteSDP, "remote")
	local := sdpQoSCurrent(remoteSDP, "local")
	return remote == "sendrecv" || local == "sendrecv"
}

func (c *Call) setPreconditionMet(met bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.MediaState.PreconditionMet = met
	c.mu.Unlock()
}

func (c *Call) preconditionMetValue() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.MediaState.PreconditionMet
}

func (a *Agent) applyCallPreconditions(call *Call, remoteSDP string) {
	if call == nil {
		return
	}
	met := sdpPreconditionsSatisfied(remoteSDP)
	call.setPreconditionMet(met)
	if met {
		if call.CallState() == callstate.StatePreconditionWait {
			_ = call.TransitionChecked(callstate.StateEarlyMedia)
		}
		return
	}
	if call.CallState() == callstate.StateEarlyMedia {
		_ = call.TransitionChecked(callstate.StatePreconditionWait)
	}
}
