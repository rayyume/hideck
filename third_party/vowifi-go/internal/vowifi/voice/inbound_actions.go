package voice

import (
	"errors"
	"fmt"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

// AnswerWithSDP answers an inbound call using the local client's real RTP endpoint.
func (a *Agent) AnswerWithSDP(callID, clientSDP string) (InboundAnswer, error) {
	call := a.callByID(callID)
	if call == nil {
		return InboundAnswer{}, errors.New("voice: call not found")
	}
	call.inboundDecisionMu.Lock()
	defer call.inboundDecisionMu.Unlock()
	if call.CallDirection() != callstate.DirectionInbound || call.CallState() != callstate.StateRinging {
		return InboundAnswer{}, errors.New("voice: inbound call is not alerting")
	}
	responder := call.inboundResponseWriter()
	if responder == nil && !call.hasServerInvite() {
		return InboundAnswer{}, errors.New("voice: inbound INVITE response context is unavailable")
	}
	answer, err := a.applyInboundAnswer(call, clientSDP)
	if err != nil {
		return InboundAnswer{}, err
	}
	a.enableMediaMonitor(call)
	if err := call.StartMediaCurrent(); err != nil {
		a.releaseInboundCall(call, err, false)
		return InboundAnswer{}, err
	}
	_, imsAnswer := call.localSDPs()
	structured, err := a.answerStoredServerInvite(call, imsAnswer)
	if err != nil {
		a.releaseInboundCall(call, err, false)
		return InboundAnswer{}, err
	}
	if !structured {
		if err := responder.Respond(a.voiceSDPResponse(call, 200, imsAnswer)); err != nil {
			a.releaseInboundCall(call, err, false)
			return InboundAnswer{}, err
		}
	}
	call.StopOutboundNoAnswerTimer()
	call.stopTUECWTimer()
	a.startVoiceSessionTimer(call)
	_ = a.SwitchCall(call.CallID())
	answer.State = call.CallState().String()
	a.emitCallAnswered(call)
	return answer, nil
}

// Reject sends a final failure response for a pending inbound call.
func (a *Agent) Reject(callID string, statusCode int) error {
	call := a.callByID(callID)
	if call == nil {
		return errors.New("voice: call not found")
	}
	if statusCode < 300 || statusCode > 699 {
		return fmt.Errorf("voice: reject status must be 300-699, got %d", statusCode)
	}
	call.inboundDecisionMu.Lock()
	defer call.inboundDecisionMu.Unlock()
	return a.rejectInboundCall(call, statusCode)
}

func (a *Agent) rejectInboundCall(call *Call, statusCode int) error {
	if call.CallDirection() != callstate.DirectionInbound {
		return errors.New("voice: call is not inbound")
	}
	if call.CallState() != callstate.StateRinging {
		return errors.New("voice: inbound call is not alerting")
	}
	responder := call.inboundResponseWriter()
	if responder == nil && !call.hasServerInvite() {
		return errors.New("voice: inbound INVITE response context is unavailable")
	}
	structured, err := a.rejectStoredServerInvite(call, statusCode)
	if err != nil {
		return err
	}
	if !structured {
		if err := responder.Respond(imscore.InboundVoiceResponse{StatusCode: statusCode}); err != nil {
			return err
		}
	}
	a.releaseInboundCall(call, fmt.Errorf("voice: inbound call rejected with %d", statusCode), false)
	return nil
}

func (a *Agent) voiceSDPResponse(call *Call, status int, sdp string) imscore.InboundVoiceResponse {
	response := imscore.InboundVoiceResponse{StatusCode: status, ContentType: "application/sdp", Body: []byte(sdp)}
	if expires := formatSessionExpiresHeader(call); expires != "" {
		response.SessionExpires = expires
	}
	if profile, err := a.registeredDialogProfile(); err == nil {
		response.Contact = profile.ContactURI
	}
	return response
}

func (a *Agent) notifyIncomingCall(call *Call) {
	a.mu.RLock()
	handler := a.incomingHandler
	a.mu.RUnlock()
	if handler != nil {
		handler(call.incomingSnapshot())
	}
}

func (a *Agent) startInboundNoAnswerTimer(call *Call) {
	call.mu.Lock()
	call.noAnswerTimer = time.AfterFunc(inboundClientWaitTimeout, func() {
		call.inboundDecisionMu.Lock()
		defer call.inboundDecisionMu.Unlock()
		if call.CallState() != callstate.StateRinging {
			return
		}
		cause := a.sendInboundTimeout(call)
		a.releaseInboundCall(call, cause, false)
	})
	call.mu.Unlock()
}

func (a *Agent) sendInboundTimeout(call *Call) error {
	cause := errors.New("voice: inbound call timed out")
	if call != nil && call.waitingIndication {
		cause = errors.New("voice: call waiting timed out")
	}
	if err := a.sendStatusResponseResult(480, "Temporarily Unavailable", call); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (a *Agent) releaseInboundCall(call *Call, cause error, canceled bool) {
	if call == nil || !call.claimTerminalFinalization() {
		return
	}
	call.stopTUECWTimer()
	_ = call.TransitionChecked(callstate.StateTerminating)
	cleanupErr := a.closeCallDialogForCleanup(call)
	if canceled {
		reason := "remote_cancel"
		if cause != nil {
			reason = cause.Error()
		}
		a.emitCallCanceled(call, reason)
	} else if cause != nil {
		a.emitCallFailed(call, cause.Error())
	}
	_ = call.TransitionChecked(callstate.StateTerminated)
	cleanupErr = errors.Join(cleanupErr, a.finalizeActiveCall(call))
	a.reportCallCleanupError(call, cleanupErr)
}

func (a *Agent) handleInboundBye(call *Call) (imscore.InboundVoiceResult, error) {
	if call == nil {
		return voiceResult(481), nil
	}
	call.stopTUECWTimer()
	call.inboundDecisionMu.Lock()
	defer call.inboundDecisionMu.Unlock()
	if call.CallState() != callstate.StateConnected {
		return voiceResult(481), nil
	}
	return voiceResult(200), a.finishRemoteBye(call)
}

func voiceResult(status int) imscore.InboundVoiceResult {
	return imscore.InboundVoiceResult{Handled: true, StatusCode: status}
}
