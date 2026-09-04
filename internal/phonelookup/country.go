package phonelookup

import "strings"

func matchCountry(normalized string) (string, bool) {
	s := strings.TrimPrefix(normalized, "+")
	if !strings.HasPrefix(normalized, "+") {
		if isCNNational(s) || cnServiceNumbers[s].carrier != "" {
			return "中国", true
		}
		return "", false
	}
	info := lookupIntl(normalized)
	if info.Country != "" {
		return info.Country, true
	}
	return "", false
}
