package phonebook

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	unicodeenc "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

type Contact struct {
	ContactID string
	Name      string
	Number    string
}

func Parse(data []byte, filename string) []Contact {
	text := decodeImportText(data)
	name := strings.ToLower(strings.TrimSpace(filename))
	switch {
	case strings.Contains(strings.ToUpper(text), "BEGIN:VCARD"):
		return parseVCard(text)
	case strings.HasSuffix(name, ".vcf"), strings.HasSuffix(name, ".vcard"):
		return parseVCard(text)
	default:
		return parseCSV(text)
	}
}

func decodeImportText(raw []byte) string {
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if len(raw) == 0 {
		return ""
	}
	switch {
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}):
		return decodeUTF16(raw, unicodeenc.LittleEndian, unicodeenc.UseBOM)
	case bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		return decodeUTF16(raw, unicodeenc.BigEndian, unicodeenc.UseBOM)
	case looksLikeUTF16LE(raw):
		return decodeUTF16(raw, unicodeenc.LittleEndian, unicodeenc.IgnoreBOM)
	}
	if utf8.Valid(raw) {
		return string(raw)
	}
	if out, err := io.ReadAll(transform.NewReader(bytes.NewReader(raw), simplifiedchinese.GBK.NewDecoder())); err == nil {
		return string(out)
	}
	return string(raw)
}

func decodeUTF16(raw []byte, endian unicodeenc.Endianness, bom unicodeenc.BOMPolicy) string {
	out, err := io.ReadAll(transform.NewReader(bytes.NewReader(raw), unicodeenc.UTF16(endian, bom).NewDecoder()))
	if err != nil || len(out) == 0 {
		return string(raw)
	}
	return string(out)
}

func looksLikeUTF16LE(raw []byte) bool {
	if len(raw) < 8 || len(raw)%2 != 0 {
		return false
	}
	nuls := 0
	limit := len(raw)
	if limit > 64 {
		limit = 64
	}
	for i := 1; i < limit; i += 2 {
		if raw[i] == 0 {
			nuls++
		}
	}
	return nuls >= 8
}

func normalizeImportedNumber(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'`)
	s = strings.TrimPrefix(s, "'")
	if len(s) >= 4 && strings.EqualFold(s[:4], "tel:") {
		s = strings.TrimSpace(s[4:])
	}
	var normalized strings.Builder
	digits := 0
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= '0' && r <= '9':
			normalized.WriteRune(r)
			digits++
		case r == '+' && normalized.Len() == 0:
			normalized.WriteRune(r)
		case r == ' ' || r == '\t' || r == '-' || r == '(' || r == ')' || r == '.':
			continue
		default:
			return ""
		}
	}
	if digits < 3 || digits > 32 {
		return ""
	}
	return normalized.String()
}

func joinPersonName(family, given string) string {
	family = strings.TrimSpace(family)
	given = strings.TrimSpace(given)
	if family == "" {
		return given
	}
	if given == "" {
		return family
	}
	if isCJKName(family) || isCJKName(given) {
		return family + given
	}
	return given + " " + family
}

func isCJKName(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func uniqueContacts(in []Contact) []Contact {
	seen := map[string]int{}
	out := make([]Contact, 0, len(in))
	for _, item := range in {
		number := normalizeImportedNumber(item.Number)
		name := strings.TrimSpace(item.Name)
		if number == "" || name == "" {
			continue
		}
		if i, ok := seen[number]; ok {
			if len(name) > len(out[i].Name) {
				out[i].Name = name
			}
			continue
		}
		seen[number] = len(out)
		out = append(out, Contact{ContactID: item.ContactID, Name: name, Number: number})
	}
	return out
}

func looksLikePhone(s string) bool {
	return normalizeImportedNumber(s) != ""
}

type contactGroup struct {
	ID      string
	Name    string
	Numbers []string
}

func groupContacts(contacts []Contact) []contactGroup {
	groups := make([]contactGroup, 0, len(contacts))
	indexes := make(map[string]int)
	for _, item := range contacts {
		name := strings.TrimSpace(item.Name)
		number := normalizeImportedNumber(item.Number)
		if name == "" || number == "" {
			continue
		}
		key := strings.TrimSpace(item.ContactID)
		if key == "" {
			key = "number:" + number
		}
		if index, ok := indexes[key]; ok {
			groups[index].Numbers = append(groups[index].Numbers, number)
			continue
		}
		indexes[key] = len(groups)
		groups = append(groups, contactGroup{ID: key, Name: name, Numbers: []string{number}})
	}
	return groups
}

func sourceContactID(kind string, index int) string {
	return fmt.Sprintf("%s:%d", kind, index)
}
