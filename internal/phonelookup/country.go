package phonelookup

import "strings"

func matchCountry(normalized string) (string, bool) {
	if !strings.HasPrefix(normalized, "+") {
		return "", false
	}
	info := lookupIntl(normalized)
	if info.Country != "" {
		return info.Country, true
	}
	return "", false
}
