package device

import (
	"strings"

	"github.com/iniwex5/vowifi-go/runtimehost/carrier"
	"github.com/yibaiba/hideck/internal/cardpolicy"
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

// WorkerSoftwareIMSBlocked reports China MCC 460/461, which has no ePDG
// software IMS path in this stack. Phone service must use native VoLTE.
func WorkerSoftwareIMSBlocked(w *Worker) bool {
	if w == nil {
		return false
	}
	mcc := strings.TrimSpace(w.state.Identity.NativeMCC)
	if carrier.IsVoWiFiBlockedMCC(mcc) {
		return true
	}
	imsi := strings.TrimSpace(w.state.Identity.IMSI)
	if len(imsi) >= 3 && carrier.IsVoWiFiBlockedMCC(imsi[:3]) {
		return true
	}
	return false
}

func forceSoftwareIMSBlockedToVoLTE(w *Worker, pol cardpolicy.Policy) {
	if w == nil || !PhoneServiceEnabled(w.Config) || !WorkerSoftwareIMSBlocked(w) {
		return
	}
	w.Config.PhoneMode = PhoneModeVoLTE
	if !pol.AirplaneEnabled {
		w.Config.AirplaneEnabled = false
	}
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
