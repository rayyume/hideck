package policy

import "strings"

const XCAPSessionSlot = "xcap"

// PDNSpec names an extra SWu PDN that reuses the IMS ePDG.
type PDNSpec struct {
	Slot string
	APN  string
}

// AdditionalPDNs returns the XCAP/Ut PDN when a distinct APN is configured.
// A single IMS APN keeps the historical one-session-per-device behavior.
func AdditionalPDNs(config EffectiveCarrierConfig) []PDNSpec {
	apn := strings.TrimSpace(config.XCAPAPN)
	ims := strings.TrimSpace(config.APN)
	if apn == "" || strings.EqualFold(apn, ims) {
		return nil
	}
	return []PDNSpec{{Slot: XCAPSessionSlot, APN: apn}}
}
