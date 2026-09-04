package phonelookup

import "strings"

type callingCode struct {
	code    string
	country string
}

func matchCountry(normalized string) (string, bool) {
	s := strings.TrimPrefix(normalized, "+")
	if !strings.HasPrefix(normalized, "+") {
		if isCNNational(s) || cnServiceNumbers[s].carrier != "" {
			return "中国", true
		}
		return "", false
	}
	best, name := "", ""
	for _, item := range countryCallingCodes {
		if strings.HasPrefix(s, item.code) && len(item.code) >= len(best) {
			best, name = item.code, item.country
		}
	}
	if best == "" {
		return "", false
	}
	return name, true
}

// 按码长大致从长到短，matchCountry 仍会取最长前缀。
var countryCallingCodes = []callingCode{
	{"86", "中国"},
	{"852", "香港"},
	{"853", "澳门"},
	{"886", "台湾"},
	{"81", "日本"},
	{"82", "韩国"},
	{"84", "越南"},
	{"66", "泰国"},
	{"65", "新加坡"},
	{"60", "马来西亚"},
	{"62", "印度尼西亚"},
	{"63", "菲律宾"},
	{"91", "印度"},
	{"92", "巴基斯坦"},
	{"880", "孟加拉"},
	{"94", "斯里兰卡"},
	{"95", "缅甸"},
	{"855", "柬埔寨"},
	{"856", "老挝"},
	{"976", "蒙古"},
	{"7", "俄罗斯"},
	{"90", "土耳其"},
	{"98", "伊朗"},
	{"966", "沙特阿拉伯"},
	{"971", "阿联酋"},
	{"972", "以色列"},
	{"974", "卡塔尔"},
	{"20", "埃及"},
	{"27", "南非"},
	{"234", "尼日利亚"},
	{"254", "肯尼亚"},
	{"212", "摩洛哥"},
	{"1", "美国/加拿大"},
	{"44", "英国"},
	{"353", "爱尔兰"},
	{"33", "法国"},
	{"49", "德国"},
	{"39", "意大利"},
	{"34", "西班牙"},
	{"31", "荷兰"},
	{"32", "比利时"},
	{"41", "瑞士"},
	{"43", "奥地利"},
	{"46", "瑞典"},
	{"47", "挪威"},
	{"45", "丹麦"},
	{"358", "芬兰"},
	{"48", "波兰"},
	{"420", "捷克"},
	{"36", "匈牙利"},
	{"30", "希腊"},
	{"351", "葡萄牙"},
	{"40", "罗马尼亚"},
	{"380", "乌克兰"},
	{"61", "澳大利亚"},
	{"64", "新西兰"},
	{"55", "巴西"},
	{"52", "墨西哥"},
	{"54", "阿根廷"},
	{"56", "智利"},
	{"57", "哥伦比亚"},
}
