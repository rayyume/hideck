package voice

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

const (
	minimumHalfSessionRefresh  = 90 * time.Second
	shortSessionRefreshLead    = 10 * time.Second
	voiceSessionRefreshTimeout = 5 * time.Second
	sessionRefresherUAC        = "uac"
	sessionRefresherUAS        = "uas"
)

type sessionExpiresOffer struct {
	Expires   time.Duration
	Refresher string
	OK        bool
}

func parseVoiceSessionExpires(value string) (time.Duration, bool) {
	offer := parseSessionExpiresOffer(value)
	return offer.Expires, offer.OK
}

func parseSessionExpiresOffer(value string) sessionExpiresOffer {
	secondsText, rest, _ := strings.Cut(strings.TrimSpace(value), ";")
	if secondsText == "" {
		return sessionExpiresOffer{}
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(secondsText))
	if err != nil || seconds <= 0 {
		return sessionExpiresOffer{}
	}
	offer := sessionExpiresOffer{Expires: time.Duration(seconds) * time.Second, OK: true}
	for _, param := range strings.Split(rest, ";") {
		key, raw, _ := strings.Cut(strings.TrimSpace(param), "=")
		if !strings.EqualFold(key, "refresher") {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(raw)) {
		case sessionRefresherUAC, sessionRefresherUAS:
			offer.Refresher = strings.ToLower(strings.TrimSpace(raw))
		}
	}
	return offer
}

func parseMinSEHeader(value string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func (c *Call) applyVoiceSessionExpires(value string) {
	offer := parseSessionExpiresOffer(value)
	if strings.TrimSpace(value) != "" && !offer.OK {
		logging.WarnRate("ims-voice-session-expires-invalid", "IMS 会话过期时间无效", "value", value)
		return
	}
	if !offer.OK {
		return
	}
	c.SessionTimerMu.Lock()
	c.SessionExpires = int(offer.Expires / time.Second)
	if offer.Refresher != "" {
		c.sessionRefresher = offer.Refresher
	} else if c.sessionRefresher == "" {
		c.sessionRefresher = sessionRefresherUAC
	}
	c.SessionTimerMu.Unlock()
}

func (c *Call) applySessionMinSE(minSE time.Duration) {
	if c == nil || minSE <= 0 {
		return
	}
	c.SessionTimerMu.Lock()
	c.sessionMinSE = int(minSE / time.Second)
	if c.SessionExpires > 0 && c.SessionExpires < c.sessionMinSE {
		c.SessionExpires = c.sessionMinSE
	}
	c.SessionTimerMu.Unlock()
}

func (c *Call) sessionRefresherValue() string {
	if c == nil {
		return ""
	}
	c.SessionTimerMu.Lock()
	defer c.SessionTimerMu.Unlock()
	if c.sessionRefresher == "" {
		return sessionRefresherUAC
	}
	return c.sessionRefresher
}

func (c *Call) sessionMinSEValue() time.Duration {
	if c == nil {
		return 0
	}
	c.SessionTimerMu.Lock()
	defer c.SessionTimerMu.Unlock()
	return time.Duration(c.sessionMinSE) * time.Second
}

func (c *Call) weAreSessionRefresher() bool {
	refresher := c.sessionRefresherValue()
	if c.CallDirection() == callstate.DirectionInbound {
		return refresher == sessionRefresherUAS
	}
	return refresher == sessionRefresherUAC
}

func formatSessionExpiresHeader(call *Call) string {
	if call == nil {
		return ""
	}
	expires := call.voiceSessionExpires()
	if expires <= 0 {
		return ""
	}
	header := strconv.FormatInt(int64(expires/time.Second), 10)
	if refresher := call.sessionRefresherValue(); refresher != "" {
		header += ";refresher=" + refresher
	}
	return header
}

func (c *Call) voiceSessionExpires() time.Duration {
	if c == nil {
		return 0
	}
	c.SessionTimerMu.Lock()
	defer c.SessionTimerMu.Unlock()
	return time.Duration(c.SessionExpires) * time.Second
}

func sessionRefreshDelay(expires time.Duration) time.Duration {
	half := expires / 2
	if half >= minimumHalfSessionRefresh {
		return half
	}
	if expires > shortSessionRefreshLead {
		return expires - shortSessionRefreshLead
	}
	return expires
}

// StartSessionTimer retains the v1.5.5 callback-based timer API.
func (c *Call) StartSessionTimer(callback func()) {
	if c == nil {
		return
	}
	c.SessionTimerMu.Lock()
	defer c.SessionTimerMu.Unlock()
	if c.SessionExpires < 1 {
		return
	}
	if c.SessionTimer != nil {
		c.SessionTimer.Stop()
	}
	delay := sessionRefreshDelay(time.Duration(c.SessionExpires) * time.Second)
	c.SessionTimer = time.AfterFunc(delay, func() {
		if callback != nil {
			callback()
		}
	})
}

// StartSessionTimerCurrent retains the additive duration-based API.
func (c *Call) StartSessionTimerCurrent(expires time.Duration) error {
	if c == nil {
		return errors.New("voice: nil call")
	}
	if expires > 0 {
		c.SessionTimerMu.Lock()
		c.SessionExpires = int(expires / time.Second)
		c.SessionTimerMu.Unlock()
	}
	if c.voiceSessionExpires() <= 0 {
		return c.stopSessionTimer()
	}
	if c.agent == nil || c.agent.ims == nil {
		return errors.New("voice: session timer has no IMS agent")
	}
	c.agent.startVoiceSessionTimer(c)
	return nil
}

func (a *Agent) startVoiceSessionTimer(call *Call) {
	if a == nil || call == nil {
		return
	}
	if call.weAreSessionRefresher() {
		call.StartSessionTimer(func() {
			a.runCallTask(call, "session_update", func() { a.sendIMSSessionUpdate(call) })
		})
		return
	}
	expires := call.voiceSessionExpires()
	if expires <= 0 {
		return
	}
	call.StartSessionTimerAfter(expires, func() {
		a.runCallTask(call, "session_expire", func() { a.expireVoiceSession(call) })
	})
}

func (c *Call) StartSessionTimerAfter(delay time.Duration, callback func()) {
	if c == nil || delay <= 0 {
		return
	}
	c.SessionTimerMu.Lock()
	defer c.SessionTimerMu.Unlock()
	if c.SessionTimer != nil {
		c.SessionTimer.Stop()
	}
	c.SessionTimer = time.AfterFunc(delay, func() {
		if callback != nil {
			callback()
		}
	})
}

func (a *Agent) sendIMSSessionUpdate(call *Call) {
	if a == nil || call == nil || call.CallState() != callstate.StateConnected {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), voiceSessionRefreshTimeout)
	defer cancel()
	err := a.refreshVoiceSession(ctx, call)
	if err != nil {
		logging.WarnRate("ims-voice-session-refresh-failed:"+call.CallID(),
			"IMS 会话刷新失败", "device", a.deviceID, "call_id", call.CallID(), "err", err)
	}
	if call.CallState() == callstate.StateConnected {
		a.startVoiceSessionTimer(call)
	}
}

func (a *Agent) expireVoiceSession(call *Call) {
	if a == nil || call == nil || call.CallState() != callstate.StateConnected {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), voiceHangupTimeout)
	defer cancel()
	if err := a.HangupContext(ctx, call.CallID()); err != nil && !call.IsTerminalState() {
		a.forceReleaseCall(call, errors.Join(errors.New("voice: session timer expired"), err))
	}
}
