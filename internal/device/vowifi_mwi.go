package device

import (
	"strings"
	"time"
)

// VoWiFiMWIState is the host snapshot of RFC 3842 message-waiting indication.
// Known is false until a NOTIFY has been observed; counts stay zero.
type VoWiFiMWIState struct {
	Known           bool
	MessagesWaiting bool
	VoiceNew        int
	VoiceOld        int
	Account         string
	UpdatedAt       time.Time
}

// RecordVoWiFiMWI stores the latest MWI snapshot for a device.
func (p *Pool) RecordVoWiFiMWI(deviceID string, state VoWiFiMWIState) {
	deviceID = strings.TrimSpace(deviceID)
	if p == nil || deviceID == "" {
		return
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = time.Now()
	}
	state.Known = true
	p.vowifiMWIMu.Lock()
	if p.vowifiMWI == nil {
		p.vowifiMWI = make(map[string]VoWiFiMWIState)
	}
	p.vowifiMWI[deviceID] = state
	p.vowifiMWIMu.Unlock()
}

// GetVoWiFiMWI returns the last MWI snapshot, or a zero empty state.
func (p *Pool) GetVoWiFiMWI(deviceID string) VoWiFiMWIState {
	if p == nil {
		return VoWiFiMWIState{}
	}
	p.vowifiMWIMu.RLock()
	defer p.vowifiMWIMu.RUnlock()
	if p.vowifiMWI == nil {
		return VoWiFiMWIState{}
	}
	return p.vowifiMWI[strings.TrimSpace(deviceID)]
}

// ClearVoWiFiMWI drops MWI state when the VoWiFi runtime stops.
func (p *Pool) ClearVoWiFiMWI(deviceID string) {
	if p == nil {
		return
	}
	p.vowifiMWIMu.Lock()
	delete(p.vowifiMWI, strings.TrimSpace(deviceID))
	p.vowifiMWIMu.Unlock()
}
