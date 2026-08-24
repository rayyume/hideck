package volte

import (
	"fmt"
	"strings"
)

var ErrNoUniqueProfile = fmt.Errorf("volte: no unique MBN profile for this PLMN")

// UniqueMBN maps a serving PLMN onto one firmware MBN name that is actually
// present on the module. Candidates are tried in order. Unknown operators
// fail closed; ROW_Generic is never used as a guess.
func UniqueMBN(mcc, mnc string, entries []MBNEntry) (string, error) {
	wants := profileNamesForPLMN(mcc, mnc)
	if len(wants) == 0 {
		return "", ErrNoUniqueProfile
	}
	for _, want := range wants {
		for _, e := range entries {
			if strings.EqualFold(strings.TrimSpace(e.Name), want) {
				return e.Name, nil
			}
		}
	}
	return "", fmt.Errorf("%w: none of %s in module list", ErrNoUniqueProfile, strings.Join(wants, ","))
}

func profileNamesForPLMN(mcc, mnc string) []string {
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
			return []string{"Volte_OpenMkt-Commercial-CMCC"}
		case "03", "05", "11":
			return []string{"VoLTE_OPNMKT_CT"}
		case "01", "06", "09":
			return []string{"CU-VoLTE"}
		case "15":
			// 中国广电。固件若带专用画像优先用；QDC507 当前清单没有，
			// 广电 5G 与移动共建共享，退到移动 VoLTE 画像。
			return []string{
				"Volte_OpenMkt-Commercial-CBN",
				"VoLTE_OpenMkt-Commercial-CBN",
				"VoLTE_OPNMKT_CBN",
				"CBN-VoLTE",
				"VoLTE-CBN",
				"Volte_OpenMkt-Commercial-CMCC",
			}
		}
	}
	return nil
}

func NormalizePLMN(mcc, mnc string) string {
	mcc = strings.TrimSpace(mcc)
	mnc = strings.TrimSpace(mnc)
	if mcc == "" || mnc == "" {
		return ""
	}
	return mcc + "-" + mnc
}
