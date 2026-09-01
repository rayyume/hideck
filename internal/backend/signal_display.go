package backend

// QMI / 高通 NAS 在 LTE 上经常把 GSM RSSI 填成这些占位值，不是真实覆盖。
const (
	qmiUnavailableRSSI = -125
	qmiMinInt8RSSI     = -128
	legacyInvalidRSSI  = -999
)

// IsPlaceholderRSSI 判断 RSSI 是否为“未测量/不可用”，不能当信号格用。
func IsPlaceholderRSSI(rssi int) bool {
	switch rssi {
	case 0, qmiUnavailableRSSI, qmiMinInt8RSSI, legacyInvalidRSSI:
		return true
	default:
		return false
	}
}

// DisplaySignalDBM 给页面/API 的主信号：有效 RSSI 优先，否则用 LTE RSRP。
func DisplaySignalDBM(rssi, rsrp int) int {
	if !IsPlaceholderRSSI(rssi) {
		return rssi
	}
	if !IsPlaceholderRSSI(rsrp) {
		return rsrp
	}
	return 0
}

func normalizeSignalInfo(info *SignalInfo) *SignalInfo {
	if info == nil {
		return nil
	}
	info.RSSI = DisplaySignalDBM(info.RSSI, info.RSRP)
	return info
}
