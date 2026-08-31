package voice

import (
	"context"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func (a *Agent) handleInboundRefer(request imscore.InboundVoiceRequest, call *Call) (imscore.InboundVoiceResult, error) {
	if call == nil {
		return voiceResult(481), nil
	}
	if call.CallState() != callstate.StateConnected {
		return voiceResult(603), nil
	}
	target := referToURI(request)
	if target == "" {
		return voiceResult(400), nil
	}
	if request.Responder != nil {
		if err := request.Responder.Respond(imscore.InboundVoiceResponse{StatusCode: 202}); err != nil {
			return voiceResult(0), err
		}
	}
	suppressNotify := referSubSuppressed(request)
	if !suppressNotify {
		_ = a.sendReferNotify(call, "SIP/2.0 100 Trying\r\n", false)
	}
	go a.completeReferTransfer(call, target, suppressNotify)
	if request.Responder != nil {
		return voiceResult(0), nil
	}
	return voiceResult(202), nil
}

func referToURI(request imscore.InboundVoiceRequest) string {
	if uri := voiceHeaderURI(request.ReferTo); uri != "" {
		return uri
	}
	if request.Request != nil {
		return voiceHeaderURI(requestHeaderValue(request.Request, "Refer-To"))
	}
	return ""
}

func referSubSuppressed(request imscore.InboundVoiceRequest) bool {
	value := strings.ToLower(strings.TrimSpace(request.ReferSub))
	if value == "" && request.Request != nil {
		value = strings.ToLower(strings.TrimSpace(requestHeaderValue(request.Request, "Refer-Sub")))
	}
	value, _, _ = strings.Cut(value, ";")
	return strings.TrimSpace(value) == "false"
}

func (a *Agent) completeReferTransfer(call *Call, target string, suppressNotify bool) {
	if a == nil || call == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sipfrag := "SIP/2.0 503 Service Unavailable\r\n"
	referred, err := a.startReferredOutboundCall(target)
	if err != nil {
		logging.Info("IMS REFER outbound INVITE skipped", "device", a.deviceID, "err", err)
	} else {
		_, err = a.executeOutboundCall(ctx, referred, "")
		if err == nil {
			sipfrag = "SIP/2.0 200 OK\r\n"
		} else {
			sipfrag = "SIP/2.0 603 Decline\r\n"
			logging.Info("IMS REFER outbound INVITE failed", "device", a.deviceID, "err", err)
		}
	}
	if !suppressNotify {
		_ = a.sendReferNotify(call, sipfrag, true)
	}
}

func (a *Agent) startReferredOutboundCall(target string) (*Call, error) {
	call := NewCall(a, callstate.DirectionOutbound, newVoiceCallID(), target)
	call.SetStartTime(time.Now())
	if err := a.prepareVoiceDialog(call, target); err != nil {
		return nil, err
	}
	if err := call.TransitionChecked(callstate.StateCalling); err != nil {
		_ = releaseUnregisteredCall(call)
		return nil, err
	}
	a.mu.Lock()
	if a.calls == nil {
		a.calls = make(map[string]*Call)
	}
	a.calls[call.CallID()] = call
	a.mu.Unlock()
	return call, nil
}

func (a *Agent) sendReferNotify(call *Call, sipfrag string, terminated bool) error {
	if a == nil || call == nil {
		return nil
	}
	request := buildIMSReferNotify(a, call, sipfrag, terminated)
	if strings.TrimSpace(request) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := a.sendCallDialogRequest(ctx, call, request)
	if err != nil {
		logging.Debug("IMS REFER NOTIFY skipped", "device", a.deviceID, "err", err)
	}
	return err
}

func buildIMSRefer(agent *Agent, call *Call, referTo string) string {
	if agent == nil || call == nil {
		return ""
	}
	ensureBuilderVoiceDialog(agent, call)
	dialog := call.advanceVoiceCSeq()
	request := buildVoiceRequest(dialog, call.CallID(), "REFER", voiceBranch(), "")
	extra := "Refer-To: " + strings.TrimSpace(referTo) + "\r\n"
	if strings.TrimSpace(dialog.localURI) != "" {
		extra += "Referred-By: <" + dialog.localURI + ">\r\n"
	}
	return strings.Replace(request, "Content-Length:", extra+"Content-Length:", 1)
}

func formatNameAddr(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return ""
	}
	if strings.HasPrefix(uri, "<") {
		return uri
	}
	return "<" + uri + ">"
}

func (c *Call) armReferSipfrag() <-chan string {
	if c == nil {
		ch := make(chan string)
		close(ch)
		return ch
	}
	ch := make(chan string, 1)
	c.mu.Lock()
	c.referSipfrag = ch
	c.mu.Unlock()
	return ch
}

func (c *Call) completeReferSipfrag(sipfrag string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	ch := c.referSipfrag
	c.referSipfrag = nil
	c.mu.Unlock()
	if ch == nil {
		return
	}
	select {
	case ch <- sipfrag:
	default:
	}
}

func buildIMSReferNotify(agent *Agent, call *Call, sipfrag string, terminated bool) string {
	if agent == nil || call == nil {
		return ""
	}
	ensureBuilderVoiceDialog(agent, call)
	dialog := call.advanceVoiceCSeq()
	body := sipfrag
	if !strings.HasSuffix(body, "\r\n") {
		body += "\r\n"
	}
	request := buildVoiceRequestWithContent(
		dialog, call.CallID(), "NOTIFY", voiceBranch(),
		"message/sipfrag;version=2.0", body,
	)
	extra := "Event: refer\r\n"
	if terminated {
		extra += "Subscription-State: terminated;reason=noresource\r\n"
	} else {
		extra += "Subscription-State: active;expires=60\r\n"
	}
	return strings.Replace(request, "Content-Length:", extra+"Content-Length:", 1)
}
