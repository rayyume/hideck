package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

// ErrHoldNotAligned is kept for the phone/API mapping of a hold that the
// live IMS dialog cannot send. Originating hold/resume now sends the
// TS 24.610 re-INVITE via setCallHold.
var ErrHoldNotAligned = errors.New("voice: hold is not aligned to 24.229/24.610 on the live network")

// HoldCall sends a mid-dialog hold re-INVITE (a=sendonly).
func (a *Agent) HoldCall(ctx context.Context, callID string) error {
	return a.setCallHold(ctx, callID, true)
}

// ResumeCall sends a mid-dialog resume re-INVITE (a=sendrecv).
func (a *Agent) ResumeCall(ctx context.Context, callID string) error {
	return a.setCallHold(ctx, callID, false)
}

func (a *Agent) setCallHold(ctx context.Context, callID string, hold bool) error {
	if a == nil {
		return errors.New("voice: nil agent")
	}
	call := a.callByID(callID)
	if call == nil {
		return errors.New("voice: call not found")
	}
	if call.CallState() != callstate.StateConnected {
		return errors.New("voice: call is not connected")
	}
	if call.localHoldValue() == hold {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return a.sendLocalHoldOffer(ctx, call, hold)
}

func (a *Agent) sendLocalHoldOffer(ctx context.Context, call *Call, hold bool) error {
	direction := sdpDirectionSendRecv
	if hold {
		direction = sdpDirectionSendOnly
	}
	sdp := rewriteSDPDirection(
		bumpSDPOriginVersion(advertiseEstablishedSessionQoS(call.imsLocalSDPValue())),
		direction,
	)
	if strings.TrimSpace(sdp) == "" {
		return errors.New("voice: local hold offer SDP is unavailable")
	}
	response, err := a.sendCallDialogInvite(ctx, call, buildIMSReinvite(a, call, sdp))
	if err != nil {
		return fmt.Errorf("voice: hold re-INVITE failed: %w", err)
	}
	call.learnVoiceDialog(response)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if _, ackErr := a.sendCallDialogRequest(ctx, call, buildIMSACKForStatus(a, call, response.StatusCode)); ackErr != nil {
			return errors.Join(fmt.Errorf("voice: hold re-INVITE rejected: %d %s", response.StatusCode, response.Reason), ackErr)
		}
		return fmt.Errorf("voice: hold re-INVITE rejected: %d %s", response.StatusCode, response.Reason)
	}
	if _, err := a.sendCallDialogRequest(ctx, call, BuildIMSACK(a, call)); err != nil {
		return fmt.Errorf("voice: ACK hold re-INVITE: %w", err)
	}
	call.MarkACKSent()
	call.applyVoiceSessionExpires(voiceResponseHeader(response.Headers, "Session-Expires"))
	call.applySessionMinSE(parseMinSEHeader(voiceResponseHeader(response.Headers, "Min-SE")))
	call.setLocalHold(hold)
	call.setLocalSDP(call.clientLocalSDPValue(), sdp)
	a.applyCallMediaDirection(call)
	a.startVoiceSessionTimer(call)
	a.emitCallMediaUpdated(call)
	return nil
}

func (a *Agent) applyCallMediaDirection(call *Call) {
	if call == nil {
		return
	}
	_, imsLocal := call.localSDPs()
	direction := sdpMediaDirection(imsLocal)
	if relay := call.RTPRelay(); relay != nil {
		relay.SetSendEnabled(localDirectionAllowsSend(direction))
	}
}

func (c *Call) setLocalHold(hold bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.localHold = hold
	c.mu.Unlock()
}

func (c *Call) setRemoteHold(hold bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.remoteHold = hold
	c.mu.Unlock()
}

func (c *Call) localHoldValue() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.localHold
}

func (c *Call) Held() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.localHold || c.remoteHold
}

func (c *Call) clientLocalSDPValue() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientLocalSDP
}
