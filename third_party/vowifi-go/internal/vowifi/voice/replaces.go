package voice

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

type replacesSpec struct {
	CallID    string
	ToTag     string
	FromTag   string
	EarlyOnly bool
}

func parseReplaces(value string) (replacesSpec, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return replacesSpec{}, errors.New("voice: empty Replaces")
	}
	callID, rest, _ := strings.Cut(value, ";")
	callID = strings.TrimSpace(strings.Trim(callID, `"`))
	if callID == "" {
		return replacesSpec{}, errors.New("voice: Replaces missing call-id")
	}
	spec := replacesSpec{CallID: callID}
	for _, part := range strings.Split(rest, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, raw, hasValue := strings.Cut(part, "=")
		name = strings.ToLower(strings.TrimSpace(name))
		if !hasValue {
			if name == "early-only" {
				spec.EarlyOnly = true
			}
			continue
		}
		token := strings.TrimSpace(strings.Trim(raw, `"`))
		switch name {
		case "to-tag":
			spec.ToTag = token
		case "from-tag":
			spec.FromTag = token
		}
	}
	if spec.ToTag == "" || spec.FromTag == "" {
		return replacesSpec{}, fmt.Errorf("voice: Replaces missing tags")
	}
	return spec, nil
}

func inboundReplacesHeader(request imscore.InboundVoiceRequest) string {
	if value := strings.TrimSpace(request.Replaces); value != "" {
		return value
	}
	if request.Request != nil {
		return strings.TrimSpace(requestHeaderValue(request.Request, "Replaces"))
	}
	return ""
}

func (a *Agent) lookupReplacedCall(spec replacesSpec) (*Call, int) {
	if a == nil {
		return nil, 500
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	var match *Call
	for _, call := range a.calls {
		if call == nil || call.IsTerminalState() {
			continue
		}
		if !replacesMatchesCall(call, spec) {
			continue
		}
		if match != nil {
			return nil, 481
		}
		match = call
	}
	if match == nil {
		return nil, 481
	}
	if spec.EarlyOnly && match.CallState() == callstate.StateConnected {
		return nil, 486
	}
	return match, 0
}

func replacesMatchesCall(call *Call, spec replacesSpec) bool {
	callID, fromTag, toTag := call.replacesDialogID()
	if !strings.EqualFold(callID, spec.CallID) {
		return false
	}
	return tagsEqual(fromTag, spec.FromTag) && tagsEqual(toTag, spec.ToTag) ||
		tagsEqual(fromTag, spec.ToTag) && tagsEqual(toTag, spec.FromTag)
}

func (c *Call) replacesDialogID() (callID, fromTag, toTag string) {
	if c == nil {
		return "", "", ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	callID = firstNonEmpty(c.DialogState.IMSCallID, c.DialogState.CallID, c.callID)
	fromTag = firstNonEmpty(c.DialogState.IMSFromTag, c.DialogState.FromTag)
	toTag = firstNonEmpty(c.DialogState.IMSToTag, c.DialogState.ToTag)
	return callID, fromTag, toTag
}

func tagsEqual(left, right string) bool {
	return strings.TrimSpace(left) != "" && strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func (a *Agent) replacedCallForInvite(request imscore.InboundVoiceRequest) (*Call, int) {
	raw := inboundReplacesHeader(request)
	if raw == "" {
		return nil, 0
	}
	spec, err := parseReplaces(raw)
	if err != nil {
		return nil, 400
	}
	return a.lookupReplacedCall(spec)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (a *Agent) terminateReplacedCall(call *Call) {
	if a == nil || call == nil || call.IsTerminalState() {
		return
	}
	if call.CallState() != callstate.StateConnected {
		_ = a.Reject(call.CallID(), 487)
		return
	}
	_ = a.HangupCurrent(call.CallID())
}
