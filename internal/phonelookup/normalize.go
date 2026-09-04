package phonelookup

import (
	"strconv"
	"strings"
	"unicode"

	"github.com/nyaruka/phonenumbers/v2"
)

func Canonical(raw string) string {
	return CanonicalWithRegion(raw, "")
}

func CanonicalWithRegion(raw, region string) string {
	normalized := NormalizeWithRegion(raw, region)
	if normalized == "" {
		return ""
	}
	_, national := nationalDigits(normalized)
	if _, ok := cnServiceNumbers[national]; ok {
		return national
	}
	digits := digitsOnly(normalized)
	if _, ok := cnServiceNumbers[digits]; ok {
		return digits
	}
	return normalized
}

func Normalize(raw string) string {
	return NormalizeWithRegion(raw, "")
}

func NormalizeWithRegion(raw, region string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, ";>"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, "<"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.Index(strings.ToLower(s), "sip:"); i >= 0 {
		s = s[i+4:]
	}
	if i := strings.Index(strings.ToLower(s), "tel:"); i >= 0 {
		s = s[i+4:]
	}
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	b.Grow(len(s) + 1)
	for _, r := range s {
		if r == '+' && b.Len() == 0 {
			b.WriteByte('+')
			continue
		}
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if strings.HasPrefix(out, "00") {
		out = "+" + strings.TrimPrefix(out, "00")
	}
	if strings.HasPrefix(out, "+") || normalizeRegion(region) == "" {
		return out
	}
	if _, ok := cnServiceNumbers[out]; ok {
		return out
	}
	number, err := phonenumbers.Parse(out, normalizeRegion(region))
	if err != nil || !phonenumbers.IsPossibleNumber(number) {
		return out
	}
	return phonenumbers.Format(number, phonenumbers.E164)
}

func normalizeRegion(region string) string {
	region = strings.ToUpper(strings.TrimSpace(region))
	if len(region) != 2 || phonenumbers.GetCountryCodeForRegion(region) == 0 {
		return ""
	}
	return region
}

func nationalDigits(normalized string) (cc, national string) {
	s := strings.TrimPrefix(strings.TrimSpace(normalized), "+")
	if s == "" {
		return "", ""
	}
	if strings.HasPrefix(normalized, "+") {
		if cc, rest := callingCodePrefix(normalized); cc != 0 {
			return strconv.Itoa(cc), rest
		}
		if len(s) > 1 {
			return s[:1], s[1:]
		}
		return s, ""
	}
	return "", s
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
