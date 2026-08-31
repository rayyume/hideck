package voice

import (
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

var (
	// ErrConferenceFactoryUnavailable is returned when the operator did not
	// provision a conference factory URI.
	ErrConferenceFactoryUnavailable = errors.New("voice: conference factory URI is unavailable")
	// ErrConferenceNeedsTwoCalls is returned when merge is requested without
	// two connected user calls.
	ErrConferenceNeedsTwoCalls = errors.New("voice: conference merge requires two connected calls")
)

// ConferenceInfo is a parsed RFC 4575 conference-info document.
type ConferenceInfo struct {
	Entity string
	Users  []ConferenceUser
}

// ConferenceUser is one conference-info user/endpoint.
type ConferenceUser struct {
	Entity string
	Status string
}

type conferenceInfoXML struct {
	XMLName xml.Name `xml:"conference-info"`
	Entity  string   `xml:"entity,attr"`
	Users   struct {
		User []struct {
			Entity   string `xml:"entity,attr"`
			Endpoint []struct {
				Status string `xml:"status"`
			} `xml:"endpoint"`
		} `xml:"user"`
	} `xml:"users"`
}

// SetConferenceFactoryURI stores the TS 24.605 conference factory URI.
func (a *Agent) SetConferenceFactoryURI(uri string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.conferenceFactoryURI = strings.TrimSpace(uri)
	a.mu.Unlock()
}

func (a *Agent) conferenceFactory() string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return strings.TrimSpace(a.conferenceFactoryURI)
}

// MergeConference INVITEs the conference factory and REFERs both connected
// user calls into the focus, then SUBSCRIBEs to conference-info.
func (a *Agent) MergeConference(ctx context.Context) (*Call, error) {
	if a == nil {
		return nil, errors.New("voice: nil agent")
	}
	factory := a.conferenceFactory()
	if factory == "" {
		return nil, ErrConferenceFactoryUnavailable
	}
	participants := a.connectedUserCalls()
	if len(participants) < 2 {
		return nil, ErrConferenceNeedsTwoCalls
	}
	if ctx == nil {
		ctx = context.Background()
	}
	conf, err := a.startConferenceOutboundCall(factory)
	if err != nil {
		return nil, err
	}
	response, err := a.executeOutboundCall(ctx, conf, "")
	if err != nil {
		return conf, err
	}
	focus := conferenceFocusURI(conf, response)
	if focus == "" {
		focus = factory
	}
	var joinErr error
	for _, participant := range participants {
		if err := a.referCallToURI(ctx, participant, focus); err != nil {
			joinErr = errors.Join(joinErr, err)
		}
	}
	if err := a.subscribeConference(ctx, conf); err != nil {
		joinErr = errors.Join(joinErr, err)
	}
	return conf, joinErr
}

func (a *Agent) connectedUserCalls() []*Call {
	if a == nil {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]*Call, 0, 2)
	for _, call := range a.calls {
		if call == nil || call.conference || call.IsTerminalState() {
			continue
		}
		if call.CallState() == callstate.StateConnected {
			out = append(out, call)
		}
	}
	return out
}

func (a *Agent) startConferenceOutboundCall(target string) (*Call, error) {
	call := NewCall(a, callstate.DirectionOutbound, newVoiceCallID(), target)
	call.conference = true
	call.SetStartTime(time.Now())
	if err := a.prepareVoiceDialog(call, target); err != nil {
		return nil, errors.Join(err, releaseUnregisteredCall(call))
	}
	if err := call.TransitionChecked(callstate.StateCalling); err != nil {
		return nil, errors.Join(err, releaseUnregisteredCall(call))
	}
	a.mu.Lock()
	if a.calls == nil {
		a.calls = make(map[string]*Call)
	}
	a.registerLiveCallLocked(call, false)
	a.mu.Unlock()
	return call, nil
}

func conferenceFocusURI(call *Call, response imscore.SIPResponse) string {
	if contact := voiceHeaderURI(voiceResponseHeader(response.Headers, "Contact")); contact != "" {
		return contact
	}
	if call == nil {
		return ""
	}
	dialog := call.voiceDialogSnapshot()
	if dialog.remoteTarget != "" {
		return dialog.remoteTarget
	}
	return dialog.remoteURI
}

func (a *Agent) referCallToURI(ctx context.Context, call *Call, uri string) error {
	if call == nil {
		return errors.New("voice: call not found")
	}
	referTo := formatNameAddr(uri)
	if referTo == "" {
		return errors.New("voice: conference Refer-To is empty")
	}
	response, err := a.sendCallDialogRequest(ctx, call, buildIMSRefer(a, call, referTo))
	if err != nil {
		return err
	}
	if response.StatusCode != 0 && (response.StatusCode < 200 || response.StatusCode >= 300) && response.StatusCode != 202 {
		return errors.New("voice: conference REFER rejected: " + imscore.SIPStatusText(response.StatusCode))
	}
	return nil
}

func (a *Agent) subscribeConference(ctx context.Context, call *Call) error {
	request := buildIMSConferenceSubscribe(a, call)
	if strings.TrimSpace(request) == "" {
		return errors.New("voice: conference SUBSCRIBE is empty")
	}
	response, err := a.sendCallDialogRequest(ctx, call, request)
	if err != nil {
		return err
	}
	if response.StatusCode != 0 && (response.StatusCode < 200 || response.StatusCode >= 300) {
		return errors.New("voice: conference SUBSCRIBE rejected: " + imscore.SIPStatusText(response.StatusCode))
	}
	return nil
}

func buildIMSConferenceSubscribe(agent *Agent, call *Call) string {
	if agent == nil || call == nil {
		return ""
	}
	ensureBuilderVoiceDialog(agent, call)
	dialog := call.advanceVoiceCSeq()
	request := buildVoiceRequest(dialog, call.CallID(), "SUBSCRIBE", voiceBranch(), "")
	extra := "Event: conference\r\nExpires: 3600\r\nAccept: application/conference-info+xml\r\n"
	return strings.Replace(request, "Content-Length:", extra+"Content-Length:", 1)
}

func parseConferenceInfo(body []byte) (ConferenceInfo, error) {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) == 0 {
		return ConferenceInfo{}, errors.New("voice: empty conference-info")
	}
	var parsed conferenceInfoXML
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return ConferenceInfo{}, err
	}
	info := ConferenceInfo{Entity: strings.TrimSpace(parsed.Entity)}
	for _, user := range parsed.Users.User {
		entry := ConferenceUser{Entity: strings.TrimSpace(user.Entity)}
		if len(user.Endpoint) > 0 {
			entry.Status = strings.TrimSpace(user.Endpoint[0].Status)
		}
		if entry.Entity != "" || entry.Status != "" {
			info.Users = append(info.Users, entry)
		}
	}
	return info, nil
}

func (c *Call) storeConferenceInfo(info ConferenceInfo) {
	if c == nil {
		return
	}
	c.mu.Lock()
	copy := info
	copy.Users = append([]ConferenceUser(nil), info.Users...)
	c.conferenceInfo = &copy
	c.mu.Unlock()
}

// ConferenceInfoSnapshot returns the last parsed conference-info document.
func (c *Call) ConferenceInfoSnapshot() ConferenceInfo {
	if c == nil {
		return ConferenceInfo{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.conferenceInfo == nil {
		return ConferenceInfo{}
	}
	out := *c.conferenceInfo
	out.Users = append([]ConferenceUser(nil), c.conferenceInfo.Users...)
	return out
}

func inboundEventPackage(request imscore.InboundVoiceRequest) string {
	value := strings.ToLower(strings.TrimSpace(request.Event))
	if value == "" && request.Request != nil {
		value = strings.ToLower(strings.TrimSpace(requestHeaderValue(request.Request, "Event")))
	}
	value, _, _ = strings.Cut(value, ";")
	return strings.TrimSpace(value)
}

func (a *Agent) handleInboundNotify(request imscore.InboundVoiceRequest, call *Call) (imscore.InboundVoiceResult, error) {
	if call == nil {
		return voiceResult(481), nil
	}
	event := inboundEventPackage(request)
	switch event {
	case "conference":
		info, err := parseConferenceInfo(request.Body)
		if err != nil {
			logging.Info("ignoring malformed conference-info", "device", a.deviceID, "err", err)
		} else {
			call.storeConferenceInfo(info)
		}
	case "refer":
		call.completeReferSipfrag(string(request.Body))
	}
	if request.Responder != nil {
		if err := request.Responder.Respond(imscore.InboundVoiceResponse{StatusCode: 200}); err != nil {
			return voiceResult(0), err
		}
		return voiceResult(0), nil
	}
	return voiceResult(200), nil
}
