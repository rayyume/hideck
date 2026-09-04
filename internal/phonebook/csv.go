package phonebook

import (
	"encoding/csv"
	"fmt"
	"strings"
	"unicode"
)

func parseCSV(text string) []Contact {
	text = strings.TrimPrefix(text, "\ufeff")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	if strings.Count(text, "\t") > strings.Count(text, ",") && strings.Count(text, "\t") > strings.Count(text, ";") {
		reader.Comma = '\t'
	} else if strings.Count(text, ";") > strings.Count(text, ",") {
		reader.Comma = ';'
	}
	rows, err := reader.ReadAll()
	if err != nil || len(rows) == 0 {
		return parseLooseLines(text)
	}
	cols := csvColumns{nameIdx: 0, givenIdx: -1, familyIdx: -1, phoneIdxs: []int{1}}
	start := 0
	scanAll := false
	hasHeader := looksLikeHeader(rows[0])
	if hasHeader {
		cols = headerColumns(rows[0])
		start = 1
		if len(cols.phoneIdxs) == 0 {
			return nil
		}
	} else {
		scanAll = true
		cols.phoneIdxs = nil
	}
	var out []Contact
	for rowIndex, row := range rows[start:] {
		name := rowName(row, cols)
		var numbers []string
		if scanAll {
			skip := map[int]struct{}{cols.nameIdx: {}, cols.givenIdx: {}, cols.familyIdx: {}}
			for i, cell := range row {
				if _, ok := skip[i]; ok {
					continue
				}
				numbers = append(numbers, splitPhones(cell)...)
			}
		} else {
			for _, i := range cols.phoneIdxs {
				if i >= 0 && i < len(row) {
					numbers = append(numbers, splitPhones(row[i])...)
				}
			}
		}
		if name == "" && len(numbers) > 0 {
			name = numbers[0]
		}
		if looksLikePhone(name) && len(numbers) == 0 {
			numbers = []string{name}
		}
		for _, number := range numbers {
			if name == "" || !looksLikePhone(number) {
				continue
			}
			out = append(out, Contact{
				ContactID: sourceContactID("csv", rowIndex), Name: name, Number: number,
			})
		}
	}
	if len(out) == 0 && !hasHeader {
		return parseLooseLines(text)
	}
	return uniqueContacts(out)
}

type csvColumns struct {
	nameIdx   int
	givenIdx  int
	familyIdx int
	phoneIdxs []int
}

func looksLikeHeader(row []string) bool {
	joined := strings.ToLower(strings.Join(row, " "))
	return strings.Contains(joined, "name") || strings.Contains(joined, "姓名") ||
		strings.Contains(joined, "phone") || strings.Contains(joined, "tel") ||
		strings.Contains(joined, "mobile") || strings.Contains(joined, "手机") ||
		strings.Contains(joined, "号码") || strings.Contains(joined, "이름") ||
		strings.Contains(joined, "휴대폰")
}

func headerColumns(row []string) csvColumns {
	cols := csvColumns{nameIdx: -1, givenIdx: -1, familyIdx: -1}
	for i, col := range row {
		key := headerKey(col)
		switch {
		case isFullNameHeader(key):
			if cols.nameIdx < 0 {
				cols.nameIdx = i
			}
		case isGivenNameHeader(key):
			cols.givenIdx = i
		case isFamilyNameHeader(key):
			cols.familyIdx = i
		case isPhoneHeader(key):
			cols.phoneIdxs = append(cols.phoneIdxs, i)
		}
	}
	if cols.nameIdx < 0 {
		if cols.givenIdx >= 0 || cols.familyIdx >= 0 {
			cols.nameIdx = -1
		} else {
			cols.nameIdx = 0
		}
	}
	return cols
}

func rowName(row []string, cols csvColumns) string {
	if cols.nameIdx >= 0 && cols.nameIdx < len(row) {
		if name := strings.TrimSpace(row[cols.nameIdx]); name != "" {
			return name
		}
	}
	given, family := "", ""
	if cols.givenIdx >= 0 && cols.givenIdx < len(row) {
		given = strings.TrimSpace(row[cols.givenIdx])
	}
	if cols.familyIdx >= 0 && cols.familyIdx < len(row) {
		family = strings.TrimSpace(row[cols.familyIdx])
	}
	return joinPersonName(family, given)
}

func headerKey(col string) string {
	key := strings.ToLower(strings.TrimSpace(col))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, key)
}

func isFullNameHeader(key string) bool {
	return key == "name" || key == "姓名" || key == "名字" || key == "名称" || key == "昵称" ||
		key == "이름" || key == "성명" || key == "displayname" || key == "display-name" ||
		key == "formattedname" || key == "fileas" || key == "file-as" || key == "nickname"
}

func isGivenNameHeader(key string) bool {
	return key == "firstname" || key == "givenname" || key == "first" || key == "名"
}

func isFamilyNameHeader(key string) bool {
	return key == "lastname" || key == "familyname" || key == "surname" || key == "last" || key == "姓"
}

func isPhoneHeader(key string) bool {
	if strings.Contains(key, "fax") || strings.Contains(key, "传真") || strings.Contains(key, "팩스") ||
		strings.Contains(key, "邮箱") || strings.Contains(key, "email") || strings.Contains(key, "mail") ||
		strings.Contains(key, "地址") || strings.Contains(key, "address") ||
		strings.Contains(key, "生日") || strings.Contains(key, "birth") ||
		strings.Contains(key, "公司") || strings.Contains(key, "单位") || strings.Contains(key, "职位") ||
		strings.Contains(key, "company") || strings.Contains(key, "org") ||
		strings.Contains(key, "备注") || strings.Contains(key, "note") || strings.Contains(key, "comment") ||
		strings.Contains(key, "type") || strings.Contains(key, "label") ||
		strings.Contains(key, "类型") || strings.Contains(key, "标签") {
		return false
	}
	return strings.Contains(key, "手机") || strings.Contains(key, "电话") || strings.Contains(key, "号码") ||
		strings.Contains(key, "phone") || strings.Contains(key, "mobile") || strings.Contains(key, "tel") ||
		strings.Contains(key, "cell") || strings.Contains(key, "座机") || strings.Contains(key, "iphone") ||
		strings.Contains(key, "휴대폰") || strings.Contains(key, "휴대전화") || strings.Contains(key, "집전화") ||
		strings.Contains(key, "회사전화") ||
		key == "住宅" || key == "办公" || key == "家庭" || key == "其他" || key == "工作" ||
		key == "number" || key == "home" || key == "work" || key == "other" || key == "office" || key == "main"
}

func splitPhones(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, ":::", "/")
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '/' || r == ';' || r == '|' || r == '、' || r == '，'
	})
	var out []string
	for _, part := range parts {
		part = normalizeImportedNumber(part)
		if looksLikePhone(part) {
			out = append(out, part)
		}
	}
	return out
}

func parseLooseLines(text string) []Contact {
	var out []Contact
	for lineIndex, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		fields := strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ';' || r == '\t' || r == '|'
		})
		if len(fields) < 2 {
			continue
		}
		name := strings.TrimSpace(fields[0])
		number := normalizeImportedNumber(fields[1])
		if candidate := normalizeImportedNumber(name); candidate != "" && number == "" {
			name, number = strings.TrimSpace(fields[1]), candidate
		}
		if name == "" || number == "" {
			continue
		}
		out = append(out, Contact{
			ContactID: sourceContactID("line", lineIndex), Name: name, Number: number,
		})
	}
	return uniqueContacts(out)
}

func ExportCSV(contacts []Contact) []byte {
	groups := groupContacts(contacts)
	maxNumbers := 0
	for _, group := range groups {
		if len(group.Numbers) > maxNumbers {
			maxNumbers = len(group.Numbers)
		}
	}
	var b strings.Builder
	b.WriteString("\ufeff")
	w := csv.NewWriter(&b)
	_ = w.Write(csvExportHeader(maxNumbers))
	for _, group := range groups {
		row := []string{group.Name}
		for _, number := range group.Numbers {
			row = append(row, "Mobile", number)
		}
		_ = w.Write(row)
	}
	w.Flush()
	return []byte(b.String())
}

func csvExportHeader(numberCount int) []string {
	header := []string{"Name"}
	for index := 1; index <= numberCount; index++ {
		header = append(header,
			fmt.Sprintf("Phone %d - Type", index),
			fmt.Sprintf("Phone %d - Value", index),
		)
	}
	return header
}
