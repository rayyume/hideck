package device

import (
	"testing"

	"github.com/yibaiba/hideck/internal/config"
)

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
