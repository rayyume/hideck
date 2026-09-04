package phonebook

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestParseXiaomiQuotedPrintableVCard(t *testing.T) {
	raw := "BEGIN:VCARD\r\nVERSION:2.1\r\nN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:=E5=BC=A0;=E4=B8=89;;;\r\nFN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:=E5=BC=A0=E4=B8=89\r\nTEL;CELL:13800138000\r\nEND:VCARD\r\n"
	got := Parse([]byte(raw), "xiaomi.vcf")
	if len(got) != 1 || got[0].Name != "张三" || got[0].Number != "13800138000" {
		t.Fatalf("%+v", got)
	}
}

func TestParseHuaweiVCard30(t *testing.T) {
	raw := "BEGIN:VCARD\nVERSION:3.0\nFN:中国联通\nTEL;TYPE=CELL,VOICE:10010\nTEL;TYPE=FAX:01088888888\nEND:VCARD\n"
	got := Parse([]byte(raw), "huawei.vcf")
	if len(got) != 1 || got[0].Name != "中国联通" || got[0].Number != "10010" {
		t.Fatalf("%+v", got)
	}
}

func TestParseOPPOCSV(t *testing.T) {
	raw := "姓名,手机\n李四,18600001111\n"
	got := Parse([]byte(raw), "oppo.csv")
	if len(got) != 1 || got[0].Name != "李四" || got[0].Number != "18600001111" {
		t.Fatalf("%+v", got)
	}
}

func TestParseVCardMultipleNumbers(t *testing.T) {
	raw := "BEGIN:VCARD\nVERSION:3.0\nFN:张三\nTEL;TYPE=CELL:13800138000\nTEL;TYPE=CELL:18600001111\nEND:VCARD\n"
	got := Parse([]byte(raw), "multi.vcf")
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	if got[0].Name != "张三" || got[1].Name != "张三" {
		t.Fatalf("%+v", got)
	}
}

func TestParseXiaomiVCardMultipleTEL(t *testing.T) {
	raw := "BEGIN:VCARD\r\nVERSION:2.1\r\nFN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:=E5=BC=A0=E4=B8=89\r\nTEL;CELL:13800138000\r\nTEL;HOME:01088888888\r\nTEL;WORK:18600001111\r\nEND:VCARD\r\n"
	got := Parse([]byte(raw), "xiaomi.vcf")
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	for _, item := range got {
		if item.Name != "张三" {
			t.Fatalf("%+v", got)
		}
	}
}

func TestExportVCardRoundTrip(t *testing.T) {
	in := []Contact{{Name: "张三", Number: "+8613800138000"}}
	got := Parse(ExportVCard(in), "export.vcf")
	if len(got) != 1 || got[0].Name != "张三" || got[0].Number != "+8613800138000" {
		t.Fatalf("%+v", got)
	}
}

func TestExportVCardGroupsMultipleNumbers(t *testing.T) {
	in := []Contact{
		{Name: "张三", Number: "13800138000"},
		{Name: "张三", Number: "18600001111"},
	}
	raw := string(ExportVCard(in))
	if strings.Count(raw, "BEGIN:VCARD") != 1 || strings.Count(raw, "TEL;TYPE=CELL;TYPE=VOICE:") != 2 {
		t.Fatalf("%s", raw)
	}
	got := Parse([]byte(raw), "export.vcf")
	if len(got) != 2 || got[0].Name != "张三" || got[1].Name != "张三" {
		t.Fatalf("%+v", got)
	}
}

func TestParseCSVMultiplePhoneColumns(t *testing.T) {
	raw := "姓名,手机号码,住宅电话,办公电话,传真\n张三,13800138000,01088888888,18600001111,01000000000\n"
	got := Parse([]byte(raw), "huawei.csv")
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	for _, item := range got {
		if item.Name != "张三" {
			t.Fatalf("%+v", got)
		}
	}
}

func TestParseCSVSlashNumbers(t *testing.T) {
	raw := "姓名,手机\n李四,13800138000/18600001111\n"
	got := Parse([]byte(raw), "oppo.csv")
	if len(got) != 2 || got[0].Name != "李四" || got[1].Name != "李四" {
		t.Fatalf("%+v", got)
	}
}

func TestParseIOSVCard(t *testing.T) {
	raw := "BEGIN:VCARD\nVERSION:3.0\nPRODID:-//Apple Inc.//iPhone OS 18.0//EN\nN:张;三;;;\nFN:张三\nTEL;type=CELL;type=VOICE;type=pref:13800138000\nitem1.TEL;type=pref:tel:+86 186 0000 1111\nitem1.X-ABLabel:iPhone\nTEL;TYPE=\"HOME,VOICE\":01088888888\nEND:VCARD\n"
	got := Parse([]byte(raw), "ios.vcf")
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	for _, item := range got {
		if item.Name != "张三" {
			t.Fatalf("%+v", got)
		}
	}
}

func TestParseGoogleCSV(t *testing.T) {
	raw := "Name,Given Name,Family Name,Phone 1 - Type,Phone 1 - Value,Phone 2 - Type,Phone 2 - Value\nZhang San,San,Zhang,Mobile,+8613800138000 ::: +8618600001111,Home,01088888888\n"
	got := Parse([]byte(raw), "google.csv")
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	for _, item := range got {
		if item.Name != "Zhang San" {
			t.Fatalf("%+v", got)
		}
	}
}

func TestParseOutlookCSV(t *testing.T) {
	raw := "First Name,Last Name,Mobile Phone,Home Phone,Business Phone\nJohn,Smith,13800138000,01088888888,18600001111\n"
	got := Parse([]byte(raw), "outlook.csv")
	if len(got) != 3 || got[0].Name != "John Smith" {
		t.Fatalf("%+v", got)
	}
}

func TestParseSamsungCSV(t *testing.T) {
	raw := "Name,Mobile Phone,Home Phone,Work Phone\nMin Park,01012345678,0211112222,0313334444\n"
	got := Parse([]byte(raw), "samsung.csv")
	if len(got) != 3 || got[0].Name != "Min Park" {
		t.Fatalf("%+v", got)
	}
}

func TestParseSamsungKoreanCSV(t *testing.T) {
	raw := "이름,휴대폰,집전화\n홍길동,01012345678,0211112222\n"
	got := Parse([]byte(raw), "samsung-kr.csv")
	if len(got) != 2 || got[0].Name != "홍길동" {
		t.Fatalf("%+v", got)
	}
}

func TestParseVivoVCard(t *testing.T) {
	raw := "BEGIN:VCARD\r\nVERSION:2.1\r\nFN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:=E7=8E=8B=E4=BA=94\r\nTEL;CELL:13900001111\r\nTEL;WORK:075512345678\r\nEND:VCARD\r\n"
	got := Parse([]byte(raw), "vivo.vcf")
	if len(got) != 2 || got[0].Name != "王五" || got[1].Name != "王五" {
		t.Fatalf("%+v", got)
	}
}

func TestParseSamsungVCard(t *testing.T) {
	raw := "BEGIN:VCARD\nVERSION:2.1\nN;CHARSET=UTF-8:Park;Min;;;\nFN;CHARSET=UTF-8:Min Park\nTEL;CELL:+821012345678\nTEL;VOICE:021234567\nX-SAMSUNG-NICKNAME:\nEND:VCARD\n"
	got := Parse([]byte(raw), "samsung.vcf")
	if len(got) != 2 || got[0].Name != "Min Park" {
		t.Fatalf("%+v", got)
	}
}

func TestParseUTF16LECSV(t *testing.T) {
	text := "姓名,手机\n李四,18600001111\n"
	u := utf16.Encode([]rune(text))
	raw := []byte{0xFF, 0xFE}
	for _, r := range u {
		raw = append(raw, byte(r), byte(r>>8))
	}
	got := Parse(raw, "excel.csv")
	if len(got) != 1 || got[0].Name != "李四" || got[0].Number != "18600001111" {
		t.Fatalf("%+v", got)
	}
}

func TestExportCSVRoundTrip(t *testing.T) {
	in := []Contact{
		{Name: "张三", Number: "13800138000"},
		{Name: "张三", Number: "18600001111"},
	}
	got := Parse(ExportCSV(in), "export.csv")
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	for _, item := range got {
		if item.Name != "张三" {
			t.Fatalf("%+v", got)
		}
	}
}
