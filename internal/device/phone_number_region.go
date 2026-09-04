package device

import (
	"strings"

	"github.com/yibaiba/hideck/internal/phonelookup"
	"github.com/yibaiba/hideck/internal/upstreamproxy"
)

func (p *Pool) PhoneNumberRegion(deviceID string) string {
	if p == nil {
		return ""
	}
	worker := p.GetWorker(strings.TrimSpace(deviceID))
	if worker == nil {
		return ""
	}
	status := worker.GetCachedDeviceStatus()
	mcc := strings.TrimSpace(status.NativeMCC)
	if mcc == "" {
		imsi := strings.TrimSpace(status.IMSI)
		if len(imsi) >= 3 {
			mcc = imsi[:3]
		}
	}
	if region, ok := upstreamproxy.CountryCodeFromHomeMCC(mcc); ok {
		return region
	}
	return phonelookup.RegionForMCC(mcc)
}
