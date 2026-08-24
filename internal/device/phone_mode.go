package device

import (
	"strings"

	"github.com/yibaiba/hideck/internal/config"
)

const (
	PhoneModeWiFi     = "wifi"
	PhoneModeCellular = "cellular"
	PhoneModeVoLTE    = "volte"
)

func IsNativeVoLTEMode(mode string) bool {
	return strings.TrimSpace(mode) == PhoneModeVoLTE
}

func PhoneModeCampsOnCell(mode string) bool {
	switch strings.TrimSpace(mode) {
	case PhoneModeCellular, PhoneModeVoLTE:
		return true
	default:
		return false
	}
}

// PhoneServiceEnabled reports the persisted "phone on" bit.
// The card-policy column remains VoWiFiEnabled; native VoLTE and
// cellular software IMS reuse it as the phone-service switch.
func PhoneServiceEnabled(cfg config.DeviceConfig) bool {
	return cfg.VoWiFiEnabled
}

func cellularAlwaysData(cfg config.DeviceConfig) bool {
	return strings.TrimSpace(cfg.PhoneMode) == PhoneModeCellular && strings.TrimSpace(cfg.DataStrategy) == "always"
}

// applyPhoneRadioPolicy applies RF side-effects of turning phone service on.
// Camped modes (cellular, volte) keep RF on; WiFi calling takes the radio.
func applyPhoneRadioPolicy(cfg *config.DeviceConfig) {
	if cfg == nil || !PhoneServiceEnabled(*cfg) {
		return
	}
	if PhoneModeCampsOnCell(cfg.PhoneMode) {
		cfg.AirplaneEnabled = false
		if cellularAlwaysData(*cfg) {
			cfg.NetworkEnabled = true
		}
		return
	}
	cfg.AirplaneEnabled = true
	cfg.NetworkEnabled = false
}
