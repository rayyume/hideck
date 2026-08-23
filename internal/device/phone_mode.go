package device

import "strings"

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
