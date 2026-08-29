package voice

import (
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

// sdpQoSReservedLocal is the RFC 3312 / 3GPP 24.229 originating offer once the
// access bearer is already up. GSMA IR.92 requires the precondition option
// tag and qos attributes; VoWiFi has SWu/IPsec before INVITE, so current
// local status must be sendrecv. Advertising "local none" plus "des
// mandatory" makes the callee wait for a segmented UPDATE that this stack
// does not send. Desired local stays mandatory (already satisfied). Remote
// stays optional. Do not put Require: precondition on the INVITE.
const sdpQoSReservedLocal = "" +
	"a=curr:qos local sendrecv\r\n" +
	"a=curr:qos remote none\r\n" +
	"a=des:qos mandatory local sendrecv\r\n" +
	"a=des:qos optional remote sendrecv\r\n"

// sdpQoSEstablishedRemote is the RFC 3312 current remote status after the
// initial session is already up. TS 24.610 Table A.1.3-1 hold re-INVITE uses
// "curr:qos remote sendrecv". Replaying the first-offer "remote none" on a
// mid-dialog hold tells the peer mandatory/segmented preconditions are still
// open (RFC 3312 §6); IR.92 2.4.1 then expects a SIP UPDATE we do not send.
const sdpQoSEstablishedRemote = "a=curr:qos remote sendrecv"

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

func advertiseEstablishedSessionQoS(sdp string) string {
	if strings.TrimSpace(sdp) == "" || !sdpHasPreconditions(sdp) {
		return sdp
	}
	var rewritten strings.Builder
	for _, source := range splitSDPTextLines(sdp) {
		line := strings.TrimRight(source, "\r")
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(trimmed, "a=curr:qos remote none") {
			line = sdpQoSEstablishedRemote
		}
		rewritten.WriteString(line)
		rewritten.WriteString("\r\n")
	}
	return rewritten.String()
}

func markLocalSDPSessionEstablished(call *Call) {
	if call == nil {
		return
	}
	client, ims := call.localSDPs()
	updated := advertiseEstablishedSessionQoS(ims)
	if updated == ims {
		return
	}
	call.setLocalSDP(client, updated)
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
