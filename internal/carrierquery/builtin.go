package carrierquery

const (
	verifiedFree       = "verified_free"
	costUnknown        = "unknown"
	official           = "official"
	projectObservation = "project_observation"
)

var builtInRules = []Rule{
	smsRule("2degrees_nz_53024", "530", "24", "2degrees NZ", "233", "BAL", "NZD", verifiedFree,
		"https://www.2degrees.nz/help/account-and-billing/manage-account/ussd-shutdown", []string{"233", "2degrees"}),
	unsupportedRule("ais_th_52001", "520", "01", "AIS Thailand", "使用 AIS App / myAIS 查询",
		"https://aiscallcenter.ais.co.th/ikm/acc/index.php?kmid=KM1113048", "公开说明没有可审计的统一免费短信查询码；AIS Wi-Fi Calling 需账户开通"),
	unsupportedRule("ais_th_52003", "520", "03", "AIS Thailand", "使用 AIS App / myAIS 查询",
		"https://aiscallcenter.ais.co.th/ikm/acc/index.php?kmid=KM1113048", "公开说明没有可审计的统一免费短信查询码；520/03 与 520/01 同属 AIS"),
	ussdSMSRule("att_310280", "310", "280", "AT&T", "*777#", "USD",
		"https://www.att.com/support/article-modal/wireless/KM1048283/", []string{"AT&T"}, "USSD 请求后由运营商短信返回结果"),
	ussdSMSRule("att_310410", "310", "410", "AT&T", "*777#", "USD",
		"https://www.att.com/support/article-modal/wireless/KM1048283/", []string{"AT&T"}, "USSD 请求后由运营商短信返回结果"),
	unsupportedRule("dito_51566", "515", "66", "DITO Philippines", "使用 DITO App 或拨 *143# 查询",
		"https://dito.ph/", "公开说明没有可审计的统一免费短信查询码；*143# 是菜单不是单次余额回包"),
	unsupportedRule("docomo_44010", "440", "10", "NTT Docomo", "使用 dアカウント / My docomo 查询",
		"https://www.docomo.ne.jp/", "公开说明没有可审计的统一免费短信查询码；IIJmio 等 Docomo MVNO 通常没有 WiFi calling 开户"),
	unsupportedRule("cmhk_45412", "454", "12", "CMHK", "使用中国移动香港 App 或账户页查询",
		"https://www.hk.chinamobile.com/", "公开说明没有可审计的统一免费短信查询码"),
	unsupportedRule("cmhk_45413", "454", "13", "CMHK", "使用中国移动香港 App 或账户页查询",
		"https://www.hk.chinamobile.com/", "公开说明没有可审计的统一免费短信查询码；454/13 与 454/12 同属 CMHK"),
	unsupportedRule("csl_454000", "454", "000", "CSL Hong Kong", "不同预付卡产品使用 *109# 或 ##122#，需先确认卡产品",
		"https://www.hkcsl.com/en/Recharge-method/", "查询码按产品变化，不能自动选择"),
	smsObservedRule("cteuk_23433", "234", "33", "CTExcel UK", "888", "BAL", "GBP", []string{"888", "CTExcel"},
		"历史项目记录观察到 BAL 发往 888 并收到 888 回复；本功能本轮未重新发送，资费状态仍未知"),
	unsupportedRule("ee_uk_23430", "234", "30", "EE UK", "使用 EE App 或账户页查询",
		"https://ee.co.uk/help", "未找到可审计的统一免费短信/USSD 查询码"),
	unsupportedRule("ee_uk_23431", "234", "31", "EE UK", "使用 EE App 或账户页查询",
		"https://ee.co.uk/help", "未找到可审计的统一免费短信/USSD 查询码；234/31 是 EE 历史 MNC"),
	unsupportedRule("ee_uk_23432", "234", "32", "EE UK", "使用 EE App 或账户页查询",
		"https://ee.co.uk/help", "未找到可审计的统一免费短信/USSD 查询码；234/32 是 EE 历史 MNC"),
	unsupportedRule("elisa_ee_24802", "248", "02", "Elisa Estonia / 乌龟卡", "乌龟卡/TravelSIM 可试 *146*099#，Elisa 官方用户走 Elisa 账户",
		"https://www.elisa.ee/en/ariklient/abi/lisateenused/volte-ja-vowifi", "248/02 同时被 Elisa 官方和 TravelSIM/esim.gg/Nekoko 乌龟卡使用，不能自动选短码"),
	smsRule("giffgaff_23410", "234", "10", "giffgaff", "85075", "INFO", "GBP", costUnknown,
		"https://help.giffgaff.com/en/articles/258872-guide-to-the-usage-statement", []string{"85075", "giffgaff"}),
	unsupportedRule("globe_ph_51502", "515", "02", "Globe / GlobeOne", "使用 GlobeOne App 查询",
		"https://www.globe.com.ph/vowifi", "公开说明没有可审计的统一免费短信查询码；GlobeOne 与 Globe 共用 515/02"),
	unsupportedRule("hotlink_my_50212", "502", "12", "Hotlink / Maxis", "使用 Hotlink App 查询",
		"https://www.hotlink.com.my/en/faq/network/vowifi/", "公开说明没有可审计的统一免费短信查询码；官方写明漫游时 VoWiFi 不可用"),
	unsupportedRule("kpn_nl_20408", "204", "08", "KPN / Simyo NL", "使用 KPN / Simyo App 查询",
		"https://www.simyo.nl/klantenservice/netwerk/volte-4g-bellen", "公开说明没有可审计的统一免费短信查询码；Simyo 走 KPN 204/08，不要和荷兰 VF 204/04 混用"),
	unsupportedRule("kddi_44051", "440", "51", "KDDI au / povo / UQ", "使用 au ID / povo / UQ 账户页查询",
		"https://www.au.com/", "公开说明没有可审计的统一免费短信查询码；440/51 是 au 系 VoLTE/WFC 卡，不要配 440/50 非 VoLTE 或 440/52 IoT"),
	unsupportedRule("lebara_uk_23487", "234", "87", "Lebara UK", "使用 Lebara App 或账户页查询",
		"https://www.lebara.co.uk/en/help.html", "未找到可审计的统一免费短信/USSD 查询码；NextGen 归属 234/87，不要和旧 234/15 Lebara 混用"),
	unsupportedRule("lycamobile_uk_23426", "234", "26", "Lycamobile UK", "使用 Lycamobile App 或账户页查询",
		"https://www.lycamobile.co.uk/en/help/", "未找到可审计的统一免费短信/USSD 查询码；不要和美国 Lyca/AT&T 310/410 混用"),
	unsupportedRule("mtn_ng_62130", "621", "30", "MTN Nigeria", "使用 myMTN App 查询",
		"https://www.mtn.ng/sim/esim/", "公开说明没有可审计的统一免费短信查询码；VoWiFi 按套餐/机型开通"),
	unsupportedRule("o2_de_26203", "262", "03", "O2 Germany", "S/M/L 套餐使用 *105#，其他套餐使用 *101#",
		"https://www.o2online.de/service/guthaben-aufladen/", "查询码取决于套餐，当前无法从 SIM 身份可靠判定"),
	unsupportedRule("o2_de_26207", "262", "07", "O2 Germany", "S/M/L 套餐使用 *105#，其他套餐使用 *101#",
		"https://www.o2online.de/service/guthaben-aufladen/", "查询码取决于套餐，当前无法从 SIM 身份可靠判定"),
	smsRule("one_nz_53001", "530", "01", "One NZ", "777", "BAL", "NZD", verifiedFree,
		"https://one.nz/faq/manage-your-mobile-by-txt", []string{"777", "One NZ", "Vodafone"}),
	unsupportedRule("oneglobal_23425", "234", "25", "1GLOBAL", "使用 1GLOBAL / Truphone App 或账户页查询",
		"https://www.1global.com/", "未找到可审计的统一免费短信/USSD 查询码；原 Truphone 归属 234/25"),
	unsupportedRule("orange_fr_20801", "208", "01", "Orange France", "使用 Orange App 或账户页查询",
		"https://www.orange.fr/", "公开说明没有可审计的统一免费短信查询码"),
	unsupportedRule("smart_ph_51503", "515", "03", "Smart Philippines", "使用 Smart App / GigaLife 查询",
		"https://www.pna.gov.ph/articles/1096002", "公开说明没有可审计的统一免费短信查询码；VoWiFi 按套餐开通"),
	unsupportedRule("softbank_44020", "440", "20", "SoftBank / LINEMO", "使用 SoftBank / LINEMO App 或账户页查询",
		"https://www.softbank.jp/", "公开说明没有可审计的统一免费短信查询码；LINEMO 与 SoftBank 官方卡共用 440/20"),
	unsupportedRule("spark_nz_53005", "530", "05", "Spark NZ", "使用 Spark App 或登录 MySpark 查询",
		"https://www.spark.co.nz/help/account/manage/top-up-prepay", "当前官方页面未提供可审计的免费设备侧查询码"),
	ussdRule("sunrise_22802", "228", "002", "Sunrise Switzerland", "*121#", "CHF", costUnknown,
		"https://www.sunrise.ch/de/support/mobile/nutzung-und-einstellungen/handy-codes-ussd"),
	unsupportedRule("telekom_de_26201", "262", "01", "Telekom Germany", "使用 MeinMagenta App 或账户页查询",
		"https://www.telekom.de/", "公开说明没有可审计的统一免费短信查询码"),
	unsupportedRule("three_hk_454003", "454", "003", "Three Hong Kong", "部分预付卡可使用 ##107#，请按卡产品说明确认",
		"https://www.three.com.hk/download/ppsim/HSDPA_guide_e.pdf", "公开说明针对特定预付卡产品"),
	unsupportedRule("three_uk_234020", "234", "020", "Three UK", "使用 Three App 或 My3 查询",
		"https://www.three.co.uk/support/pay-as-you-go/managing-your-account-online", "当前官方页面仅提供 App/My3 查询"),
	ussdLimitedRule("tmobile_310240", "310", "240", "T-Mobile US", "#999#", "USD",
		"https://www.t-mobile.com/support/plans-features/self-service-short-codes/", "部分新套餐不支持该短码"),
	ussdLimitedRule("tmobile_310260", "310", "260", "T-Mobile US", "#999#", "USD",
		"https://www.t-mobile.com/support/plans-features/self-service-short-codes/", "部分新套餐不支持该短码"),
	unsupportedRule("vodafone_de_26202", "262", "02", "Vodafone Germany", "使用 Vodafone App 或账户页查询",
		"https://www.vodafone.de/", "公开说明没有可审计的统一免费短信查询码"),
	smsRule("vodafone_nl_20404", "204", "04", "Vodafone Netherlands", "4000", "STATUS", "EUR", costUnknown,
		"https://www.vodafone.nl/abonnement/prepaid/en", []string{"4000", "Vodafone"}),
	unsupportedRule("vodafone_uk_23415", "234", "15", "Vodafone UK / VOXI", "使用 Vodafone/VOXI App 或账户页查询",
		"https://www.voxi.co.uk/help/network/what-is-wifi-calling", "未找到可审计的统一免费短信/USSD 查询码；VOXI 与沃达丰 UK 共用 234/15"),
	unsupportedRule("ymobile_44000", "440", "00", "Y!mobile", "使用 Y!mobile App 或账户页查询",
		"https://www.ymobile.jp/", "公开说明没有可审计的统一免费短信查询码；旧 eAccess IMSI 仍可能是 440/00，新卡常见 440/20"),
}

func BuiltInRules() []Rule {
	out := make([]Rule, len(builtInRules))
	for i := range builtInRules {
		out[i] = cloneRule(builtInRules[i])
	}
	return out
}

func FindBuiltIn(mcc, mnc string) (Rule, bool) {
	wanted, err := PLMNKey(mcc, mnc)
	if err != nil {
		return Rule{}, false
	}
	for _, rule := range builtInRules {
		key, _ := PLMNKey(rule.MCC, rule.MNC)
		if key == wanted {
			return cloneRule(rule), true
		}
	}
	return Rule{}, false
}

func cloneRule(rule Rule) Rule {
	rule.ExpectedSenders = append([]string(nil), rule.ExpectedSenders...)
	rule.Limitations = append([]string(nil), rule.Limitations...)
	return rule
}

func smsRule(id, mcc, mnc, operator, destination, payload, currency, cost, source string, senders []string) Rule {
	return Rule{ID: id, MCC: mcc, MNC: mnc, Operator: operator, Transport: TransportSMS, Destination: destination,
		Payload: payload, ResponseMode: ResponseSMS, ExpectedSenders: senders, ParserPattern: balancePattern,
		Currency: currency, CostStatus: cost, EvidenceType: official, EvidenceURL: source, Enabled: true, BuiltIn: true}
}

func smsObservedRule(id, mcc, mnc, operator, destination, payload, currency string, senders []string, note string) Rule {
	rule := smsRule(id, mcc, mnc, operator, destination, payload, currency, costUnknown, "", senders)
	rule.EvidenceType = projectObservation
	rule.Limitations = []string{note}
	return rule
}

func ussdRule(id, mcc, mnc, operator, code, currency, cost, source string) Rule {
	return Rule{ID: id, MCC: mcc, MNC: mnc, Operator: operator, Transport: TransportUSSD, Payload: code,
		ResponseMode: ResponseDirect, ParserPattern: balancePattern, Currency: currency, CostStatus: cost,
		EvidenceType: official, EvidenceURL: source, Enabled: true, BuiltIn: true}
}

func ussdSMSRule(id, mcc, mnc, operator, code, currency, source string, senders []string, limitation string) Rule {
	rule := ussdRule(id, mcc, mnc, operator, code, currency, costUnknown, source)
	rule.ResponseMode = ResponseSMS
	rule.ExpectedSenders = senders
	rule.Limitations = []string{limitation}
	return rule
}

func ussdLimitedRule(id, mcc, mnc, operator, code, currency, source, limitation string) Rule {
	rule := ussdRule(id, mcc, mnc, operator, code, currency, costUnknown, source)
	rule.Limitations = []string{limitation}
	return rule
}

func unsupportedRule(id, mcc, mnc, operator, alternative, source, limitation string) Rule {
	return Rule{ID: id, MCC: mcc, MNC: mnc, Operator: operator, Transport: TransportUnsupported,
		ResponseMode: ResponseNone, CostStatus: costUnknown, EvidenceType: official, EvidenceURL: source,
		Limitations: []string{limitation}, Alternative: alternative, Enabled: true, BuiltIn: true}
}

const balancePattern = `(?i)(?:credit|balance|saldo|guthaben|tegoed|余额)[^0-9]{0,32}(?P<amount>[0-9]+(?:[.,][0-9]{1,2})?)`
