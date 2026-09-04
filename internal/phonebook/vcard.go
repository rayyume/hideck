package phonebook

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/quotedprintable"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

func parseVCard(text string) []Contact {
	var out []Contact
	for _, card := range splitVCards(unfoldVCard(text)) {
		name := vcardName(card)
		for _, number := range vcardNumbers(card) {
			if name == "" {
				name = number
			}
			out = append(out, Contact{Name: name, Number: number})
		}
	}
	return uniqueContacts(out)
}

func unfoldVCard(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.ReplaceAll(text, "\n ", "")
	text = strings.ReplaceAll(text, "\n\t", "")
	return text
}

func splitVCards(text string) [][]string {
	var cards [][]string
	var current []string
	inCard := false
	for _, line := range strings.Split(text, "\n") {
		upper := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(upper, "BEGIN:VCARD"):
			current = nil
			inCard = true
		case strings.HasPrefix(upper, "END:VCARD"):
			if inCard && len(current) > 0 {
				cards = append(cards, current)
			}
			current = nil
			inCard = false
		case inCard && strings.TrimSpace(line) != "":
			current = append(current, line)
		}
	}
	return cards
}

func vcardName(card []string) string {
	if fn := vcardValue(card, "FN"); fn != "" {
		return fn
	}
	if nick := vcardValue(card, "NICKNAME"); nick != "" {
		return nick
	}
	n := vcardValue(card, "N")
	if n != "" {
		parts := strings.Split(n, ";")
		family, given := "", ""
		if len(parts) > 0 {
			family = strings.TrimSpace(parts[0])
		}
		if len(parts) > 1 {
			given = strings.TrimSpace(parts[1])
		}
		if name := joinPersonName(family, given); name != "" {
			return name
		}
	}
	return vcardValue(card, "ORG")
}

func vcardNumbers(card []string) []string {
	var out []string
	for _, line := range card {
		name, params, value := splitVCardLine(line)
		if !isVCardPhoneProperty(name) || strings.TrimSpace(value) == "" {
			continue
		}
		if vcardHasType(params, "FAX") || vcardHasType(params, "PAGER") {
			continue
		}
		decoded := normalizeImportedNumber(decodeVCardValue(params, value))
		if looksLikePhone(decoded) {
			out = append(out, decoded)
		}
	}
	return out
}

func isVCardPhoneProperty(name string) bool {
	switch strings.ToUpper(strings.TrimSpace(name)) {
	case "TEL", "X-MOBILE", "X-IPHONE", "X-ANDROID-CUSTOM-TEL", "CELL":
		return true
	default:
		return false
	}
}

func vcardValue(card []string, want string) string {
	for _, line := range card {
		name, params, value := splitVCardLine(line)
		if strings.EqualFold(name, want) {
			return decodeVCardValue(params, value)
		}
	}
	return ""
}

func splitVCardLine(line string) (name string, params []string, value string) {
	colon := strings.IndexByte(line, ':')
	if colon < 0 {
		return "", nil, strings.TrimSpace(line)
	}
	head := line[:colon]
	value = line[colon+1:]
	parts := strings.Split(head, ";")
	name = strings.TrimSpace(parts[0])
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	if len(parts) > 1 {
		params = parts[1:]
	}
	return name, params, value
}

func vcardHasType(params []string, want string) bool {
	want = strings.ToUpper(want)
	for _, got := range vcardTypes(params) {
		if got == want {
			return true
		}
	}
	return false
}

func vcardTypes(params []string) []string {
	var types []string
	for _, param := range params {
		p := strings.TrimSpace(param)
		if p == "" {
			continue
		}
		upper := strings.ToUpper(p)
		eq := strings.Index(upper, "TYPE=")
		if eq >= 0 {
			val := strings.Trim(p[eq+5:], `"`)
			for _, item := range strings.Split(val, ",") {
				item = strings.ToUpper(strings.TrimSpace(item))
				if item != "" {
					types = append(types, item)
				}
			}
			continue
		}
		if !strings.Contains(upper, "=") {
			types = append(types, upper)
		}
	}
	return types
}

func decodeVCardValue(params []string, value string) string {
	charset := "utf-8"
	encoding := ""
	for _, param := range params {
		upper := strings.ToUpper(strings.TrimSpace(param))
		switch {
		case strings.HasPrefix(upper, "CHARSET="):
			charset = strings.ToLower(strings.TrimPrefix(upper, "CHARSET="))
		case strings.HasPrefix(upper, "ENCODING="):
			encoding = strings.TrimPrefix(upper, "ENCODING=")
		case upper == "QUOTED-PRINTABLE":
			encoding = "QUOTED-PRINTABLE"
		case upper == "BASE64" || upper == "B":
			encoding = "BASE64"
		}
	}
	raw := []byte(value)
	switch encoding {
	case "QUOTED-PRINTABLE":
		decoded, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(strings.ReplaceAll(value, "=\n", ""))))
		if err == nil {
			raw = decoded
		}
	case "BASE64", "B":
		compact := make([]byte, 0, len(value))
		for _, r := range value {
			if !unicode.IsSpace(r) {
				compact = append(compact, byte(r))
			}
		}
		decoded, err := base64.StdEncoding.DecodeString(string(compact))
		if err == nil {
			raw = decoded
		}
	}
	switch charset {
	case "gbk", "gb2312", "gb18030":
		if out, err := io.ReadAll(transform.NewReader(bytes.NewReader(raw), simplifiedchinese.GBK.NewDecoder())); err == nil {
			return vcardUnescape(strings.TrimSpace(string(out)))
		}
	}
	return vcardUnescape(strings.TrimSpace(string(raw)))
}

func vcardUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n', 'N':
				b.WriteByte('\n')
			case ',', ';', '\\':
				b.WriteByte(s[i+1])
			default:
				b.WriteByte(s[i+1])
			}
			i++
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func ExportVCard(contacts []Contact) []byte {
	groups := map[string][]string{}
	order := make([]string, 0, len(contacts))
	for _, item := range contacts {
		name := strings.TrimSpace(item.Name)
		number := strings.TrimSpace(item.Number)
		if name == "" || number == "" {
			continue
		}
		if _, ok := groups[name]; !ok {
			order = append(order, name)
		}
		groups[name] = append(groups[name], number)
	}
	var b strings.Builder
	for _, name := range order {
		b.WriteString("BEGIN:VCARD\r\nVERSION:3.0\r\n")
		b.WriteString("FN:" + vcardEscape(name) + "\r\n")
		b.WriteString("N:" + vcardEscape(name) + ";;;;\r\n")
		for _, number := range groups[name] {
			b.WriteString("TEL;TYPE=CELL;TYPE=VOICE:" + vcardEscape(number) + "\r\n")
		}
		b.WriteString("END:VCARD\r\n")
	}
	return []byte(b.String())
}

func vcardEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, ",", `\,`)
	s = strings.ReplaceAll(s, ";", `\;`)
	return s
}
