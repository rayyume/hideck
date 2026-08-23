package volte

const (
	PhaseIdle       = "idle"
	PhaseEnabling   = "enabling"
	PhaseRegistered = "registered"
	PhaseUnverified = "ims_enabled_unverified"
	PhaseFailed     = "failed"
)

type Status struct {
	DeviceID          string `json:"device_id"`
	Phase             string `json:"phase"`
	IMSEnabled        bool   `json:"ims_enabled"`
	VoLTEEnabled      bool   `json:"volte_enabled"`
	IMSRegistered     bool   `json:"ims_registered"`
	VoiceAvailable    bool   `json:"voice_available"`
	UACEnabled        bool   `json:"uac_enabled"`
	AudioDevice       string `json:"audio_device,omitempty"`
	RebootRequired    bool   `json:"reboot_required"`
	QMIIMSUnavailable bool   `json:"qmi_ims_unavailable"`
	LastError         string `json:"last_error,omitempty"`
}

func (s Status) Ready() bool {
	return s.IMSRegistered || (s.IMSEnabled && s.VoLTEEnabled && s.Phase == PhaseUnverified)
}
