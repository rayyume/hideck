package volte

import (
	"fmt"
	"strings"
)

var ErrNoUniqueProfile = fmt.Errorf("volte: no unique MBN profile for this PLMN")

// UniqueMBN maps a serving PLMN onto one firmware MBN name that is actually
// present on the module. Unknown operators and missing names fail closed;
// ROW_Generic is never used as a guess.
func UniqueMBN(mcc, mnc string, entries []MBNEntry) (string, error) {
	want := profileNameForPLMN(mcc, mnc)
	if want == "" {
		return "", ErrNoUniqueProfile
	}
	for _, e := range entries {
		if strings.EqualFold(strings.TrimSpace(e.Name), want) {
			return e.Name, nil
		}
	}
	return "", fmt.Errorf("%w: %s not in module list", ErrNoUniqueProfile, want)
}

func profileNameForPLMN(mcc, mnc string) string {
	mcc = strings.TrimSpace(mcc)
	mnc = strings.TrimLeft(strings.TrimSpace(mnc), "0")
	if mnc == "" {
		mnc = "0"
	}
	if len(mnc) == 1 {
		mnc = "0" + mnc
	}
	switch mcc {
	case "460":
		switch mnc {
		case "00", "02", "04", "07", "08":
			return "Volte_OpenMkt-Commercial-CMCC"
		case "03", "05", "11":
			return "VoLTE_OPNMKT_CT"
		case "01", "06", "09":
			return "CU-VoLTE"
		}
	}
	return ""
}

func NormalizePLMN(mcc, mnc string) string {
	mcc = strings.TrimSpace(mcc)
	mnc = strings.TrimSpace(mnc)
	if mcc == "" || mnc == "" {
		return ""
	}
	return mcc + "-" + mnc
}
