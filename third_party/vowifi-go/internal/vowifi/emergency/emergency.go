package emergency

import (
	"errors"
	"fmt"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
)

const (
	// ServiceURN is the RFC 5031 top-level emergency service URN.
	ServiceURN = "urn:service:sos"
	// AnonymousIMPU is the RFC 3261 anonymous identity used when no IMPU is available.
	AnonymousIMPU = "sip:anonymous@anonymous.invalid"
)

// ErrOriginatingDisabled is returned when the UE detects an emergency destination
// but originating emergency sessions is not enabled.
var ErrOriginatingDisabled = errors.New("emergency originating is disabled")

var emergencyNumbers = map[string]struct{}{
	"112": {},
	"999": {},
	"911": {},
	"000": {},
	"110": {},
	"118": {},
	"119": {},
}

// IsEmergencyDestination reports whether value is an emergency service URN or number.
func IsEmergencyDestination(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if isEmergencyURN(value) {
		return true
	}
	digits := emergencyDigits(value)
	_, ok := emergencyNumbers[digits]
	return ok
}

// ServiceURNFor maps a dialled emergency number or URN to the Request-URI URN.
func ServiceURNFor(value string) string {
	value = strings.TrimSpace(value)
	if isEmergencyURN(value) {
		return strings.ToLower(value)
	}
	if IsEmergencyDestination(value) {
		return ServiceURN
	}
	return ""
}

// EmergencyEPDGAddr is the IR.51 / TS 23.003 emergency ePDG FQDN.
func EmergencyEPDGAddr(mcc, mnc string) string {
	return fmt.Sprintf("sos.epdg.epc.mnc%s.mcc%s.pub.3gppnetwork.org", common.Plmn3(mnc), common.Plmn3(mcc))
}

func isEmergencyURN(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return lower == ServiceURN || strings.HasPrefix(lower, ServiceURN+".")
}

func emergencyDigits(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.ToLower(value), "tel:")
	value = strings.TrimPrefix(value, "sip:")
	if at := strings.IndexByte(value, '@'); at >= 0 {
		value = value[:at]
	}
	var digits strings.Builder
	for _, character := range value {
		if character >= '0' && character <= '9' {
			digits.WriteRune(character)
		}
	}
	return digits.String()
}
