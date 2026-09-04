package phonelookup

import (
	"strings"
	"unicode"
)

func Canonical(raw string) string {
	normalized := Normalize(raw)
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
	digits := strings.TrimPrefix(out, "+")
	if !strings.HasPrefix(out, "+") {
		switch {
		case len(digits) == 11 && digits[0] == '1':
			out = "+86" + digits
		case len(digits) >= 10 && digits[0] == '0':
			out = "+86" + strings.TrimPrefix(digits, "0")
		default:
			out = digits
		}
	}
	return out
}

func nationalDigits(normalized string) (cc, national string) {
	s := strings.TrimPrefix(strings.TrimSpace(normalized), "+")
	if s == "" {
		return "", ""
	}
	if strings.HasPrefix(normalized, "+") {
		for _, n := range countryCallingCodes {
			if strings.HasPrefix(s, n.code) {
				rest := strings.TrimPrefix(s, n.code)
				if rest != "" || len(n.code) >= 3 {
					return n.code, rest
				}
			}
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
