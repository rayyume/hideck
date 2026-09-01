package device

import (
	"testing"

	"github.com/yibaiba/hideck/internal/config"
)

func TestWorkerSoftwareIMSBlockedChinaMCC(t *testing.T) {
	if WorkerSoftwareIMSBlocked(nil) {
		t.Fatal("nil worker")
	}
	china := &Worker{ID: "wwan1"}
	china.state.Identity.NativeMCC = "460"
	if !WorkerSoftwareIMSBlocked(china) {
		t.Fatal("MCC 460 should block software IMS")
	}
	imsiOnly := &Worker{ID: "wwan1"}
	imsiOnly.state.Identity.IMSI = "460011234567890"
	if !WorkerSoftwareIMSBlocked(imsiOnly) {
		t.Fatal("IMSI 460 should block software IMS")
	}
	vf := &Worker{ID: "wwan0"}
	vf.state.Identity.NativeMCC = "234"
	if WorkerSoftwareIMSBlocked(vf) {
		t.Fatal("MCC 234 must keep software IMS")
	}
}

func TestPhoneServiceEnabledReadsVoWiFiBit(t *testing.T) {
	if PhoneServiceEnabled(config.DeviceConfig{}) {
		t.Fatal("empty config is phone-off")
	}
	if !PhoneServiceEnabled(config.DeviceConfig{VoWiFiEnabled: true}) {
		t.Fatal("VoWiFiEnabled remains the persisted phone-on bit")
	}
}

func TestApplyPhoneRadioPolicyByMode(t *testing.T) {
	wifi := config.DeviceConfig{VoWiFiEnabled: true, PhoneMode: PhoneModeWiFi, NetworkEnabled: true}
	applyPhoneRadioPolicy(&wifi)
	if !wifi.AirplaneEnabled || wifi.NetworkEnabled {
		t.Fatalf("WiFi calling should take RF: %+v", wifi)
	}

	cell := config.DeviceConfig{VoWiFiEnabled: true, PhoneMode: PhoneModeCellular, DataStrategy: "always"}
	applyPhoneRadioPolicy(&cell)
	if cell.AirplaneEnabled || !cell.NetworkEnabled {
		t.Fatalf("cellular always should camp and open data: %+v", cell)
	}

	volte := config.DeviceConfig{VoWiFiEnabled: true, PhoneMode: PhoneModeVoLTE, DataStrategy: "always"}
	applyPhoneRadioPolicy(&volte)
	if volte.AirplaneEnabled || volte.NetworkEnabled {
		t.Fatalf("volte should camp without forcing data: %+v", volte)
	}
}
