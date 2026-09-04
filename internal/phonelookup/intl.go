package phonelookup

import (
	"strconv"
	"strings"

	"github.com/nyaruka/phonenumbers/v2"
	"github.com/nyaruka/phonenumbers/v2/carrier"
	"github.com/nyaruka/phonenumbers/v2/geocoding"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// 来电条上过长的官方中文名改成常用短名。
var isoShortNames = map[string]string{
	"AE": "阿联酋",
	"AG": "安提瓜",
	"BA": "波黑",
	"BQ": "荷属加勒比",
	"CC": "科科斯群岛",
	"CD": "刚果（金）",
	"CG": "刚果（布）",
	"CZ": "捷克",
	"DO": "多米尼加",
	"FM": "密克罗尼西亚",
	"GB": "英国",
	"HK": "香港",
	"IO": "英属印度洋领地",
	"KG": "吉尔吉斯",
	"KN": "圣基茨",
	"KP": "朝鲜",
	"KR": "韩国",
	"MO": "澳门",
	"MP": "北马里亚纳",
	"NC": "新喀里多尼亚",
	"PF": "法属波利尼西亚",
	"PG": "巴新",
	"PM": "圣皮埃尔",
	"PS": "巴勒斯坦",
	"RU": "俄罗斯",
	"SJ": "斯瓦尔巴",
	"ST": "圣普",
	"TA": "特里斯坦",
	"TC": "特克斯和凯科斯",
	"TT": "特立尼达",
	"TW": "台湾",
	"US": "美国",
	"VC": "圣文森特",
	"VG": "英属维京",
	"VI": "美属维京",
	"WF": "瓦利斯和富图纳",
}

var nonGeoCallingCodes = map[int]string{
	800: "国际免费电话",
	808: "国际共享电话",
	870: "海事卫星电话",
	878: "通用个人号码",
	881: "全球卫星电话",
	882: "国际网络号码",
	883: "国际网络号码",
	888: "国际号码",
	979: "国际费率电话",
}

type intlInfo struct {
	ISO     string
	Country string
	Region  string
	Carrier string
	Kind    string
}

func lookupIntl(e164 string) intlInfo {
	if !strings.HasPrefix(e164, "+") {
		return intlInfo{}
	}
	num, err := phonenumbers.Parse(e164, "ZZ")
	cc := 0
	if err == nil && num != nil && num.GetCountryCode() != 0 {
		cc = int(num.GetCountryCode())
	} else {
		cc, _ = callingCodePrefix(e164)
		num = nil
	}
	if cc == 0 {
		return intlInfo{}
	}

	out := intlInfo{}
	if label, ok := nonGeoCallingCodes[cc]; ok {
		out.Country = label
		out.Kind = "international"
		if num != nil {
			if k := kindFromNumberType(phonenumbers.GetNumberType(num)); k != "" {
				out.Kind = k
			}
			out.Carrier = carrierName(num)
		}
		return out
	}

	iso := ""
	if num != nil {
		iso = phonenumbers.GetRegionCodeForNumber(num)
	}
	if iso == "" || iso == "ZZ" || iso == "001" {
		iso = phonenumbers.GetRegionCodeForCountryCode(cc)
	}
	out.ISO = iso
	out.Country = countryNameForISO(iso)
	if out.Country == "" {
		for _, region := range phonenumbers.GetRegionCodesForCountryCode(cc) {
			if name := countryNameForISO(region); name != "" {
				out.ISO = region
				out.Country = name
				break
			}
		}
	}
	if num != nil {
		out.Kind = kindFromNumberType(phonenumbers.GetNumberType(num))
		out.Region = geoArea(num, out.Country, out.ISO)
		out.Carrier = carrierName(num)
	}
	if out.Kind == "" && out.Country != "" {
		out.Kind = "international"
	}
	return out
}

func callingCodePrefix(e164 string) (cc int, rest string) {
	digits := strings.TrimPrefix(e164, "+")
	if digits == "" || digits == e164 && !strings.HasPrefix(e164, "+") {
		return 0, ""
	}
	codes := phonenumbers.GetSupportedCallingCodes()
	for n := 3; n >= 1; n-- {
		if len(digits) < n {
			continue
		}
		v, err := strconv.Atoi(digits[:n])
		if err != nil || !codes[v] {
			continue
		}
		return v, digits[n:]
	}
	return 0, ""
}

func carrierName(num *phonenumbers.PhoneNumber) string {
	if name, err := carrier.GetNameForNumber(num, "zh"); err == nil {
		if s := strings.TrimSpace(name); s != "" {
			return s
		}
	}
	if name, err := carrier.GetNameForNumber(num, "en"); err == nil {
		return strings.TrimSpace(name)
	}
	return ""
}

func countryNameForISO(iso string) string {
	iso = strings.ToUpper(strings.TrimSpace(iso))
	if iso == "" || iso == "ZZ" || iso == "001" {
		return ""
	}
	if name, ok := isoShortNames[iso]; ok {
		return name
	}
	reg, err := language.ParseRegion(iso)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(display.Regions(language.Chinese).Name(reg))
}

func geoArea(num *phonenumbers.PhoneNumber, country, iso string) string {
	for _, lang := range []string{"zh", "en"} {
		desc, err := geocoding.GetDescriptionForNumber(num, lang)
		if err != nil {
			continue
		}
		desc = strings.TrimSpace(desc)
		if desc == "" || isCountryLevelDescription(desc, country, iso) {
			continue
		}
		return desc
	}
	return ""
}

func isCountryLevelDescription(desc, country, iso string) bool {
	if desc == country || desc == iso {
		return true
	}
	if country != "" && (strings.Contains(desc, country) || strings.Contains(country, desc)) {
		return true
	}
	iso = strings.ToUpper(strings.TrimSpace(iso))
	if iso == "" || iso == "ZZ" {
		return false
	}
	reg, err := language.ParseRegion(iso)
	if err != nil {
		return false
	}
	en := display.Regions(language.English).Name(reg)
	zh := display.Regions(language.Chinese).Name(reg)
	return desc == en || desc == zh
}

func kindFromNumberType(t phonenumbers.PhoneNumberType) string {
	switch t {
	case phonenumbers.MOBILE, phonenumbers.FIXED_LINE_OR_MOBILE, phonenumbers.PAGER:
		return "mobile"
	case phonenumbers.FIXED_LINE:
		return "landline"
	case phonenumbers.TOLL_FREE, phonenumbers.SHARED_COST, phonenumbers.UAN:
		return "service"
	case phonenumbers.UNKNOWN:
		return ""
	default:
		return "international"
	}
}
