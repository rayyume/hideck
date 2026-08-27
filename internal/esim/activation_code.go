package esim

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/damonto/euicc-go/lpa"
)

// ParsedActivationCode is the GSMA SGP.22 activation payload used to download a profile.
type ParsedActivationCode struct {
	SMDP                 string
	MatchingID           string
	OID                  string
	ConfirmationRequired bool
	ConfirmationCode     string
}

var (
	smdpFieldPattern     = regexp.MustCompile(`(?i)(?:SM-?DP\+?\s*(?:Address|地址)?|服务器地址|下载地址)\s*[:：]\s*(\S+)`)
	matchingFieldPattern = regexp.MustCompile(`(?i)(?:Matching\s*ID|Activation\s*Code|AC(?:tivation)?\s*Token|激活码|匹配码)\s*[:：]\s*(\S+)`)
	confirmFieldPattern  = regexp.MustCompile(`(?i)(?:Confirmation\s*Code|确认码)\s*[:：]\s*(\S+)`)
	hostTokenPattern     = regexp.MustCompile(`(?i)^((?:https?://)?[A-Za-z0-9.-]+\.[A-Za-z0-9.-]+(?::\d+)?)\$([^$]+)(?:\$([^$]*))?(?:\$([^$]*))?$`)
)

func looksLikeActivationCode(raw string) bool {
	value := strings.TrimSpace(raw)
	upper := strings.ToUpper(value)
	if strings.Contains(upper, "LPA:") || strings.HasPrefix(value, "1$") || strings.Contains(strings.ToLower(value), "carddata=") {
		return true
	}
	if _, ok := parseTwoLineActivation(value); ok {
		return true
	}
	if parsed, ok := parseLabeledActivation(value); ok && parsed.MatchingID != "" {
		return true
	}
	return hostTokenPattern.MatchString(compactActivationText(value))
}

// ParseActivationCode accepts a QR payload, Apple-wrapped URL, labeled carrier text, or raw LPA:1$ string.
func ParseActivationCode(raw string) (ParsedActivationCode, error) {
	if code, err := extractActivationCode(raw); err == nil {
		return unmarshalActivationCode(code)
	}
	if parsed, ok := parseLabeledActivation(raw); ok {
		return parsed, nil
	}
	if parsed, ok := parseTwoLineActivation(raw); ok {
		return parsed, nil
	}
	return ParsedActivationCode{}, fmt.Errorf("不是可识别的 eSIM 激活码，请粘贴 LPA:1$ 二维码内容，或包含 SM-DP+ 地址和激活码的文本")
}

func unmarshalActivationCode(code string) (ParsedActivationCode, error) {
	var ac lpa.ActivationCode
	if err := ac.UnmarshalText([]byte(code)); err != nil {
		return ParsedActivationCode{}, fmt.Errorf("无效的 eSIM 激活码")
	}
	if ac.SMDP == nil || strings.TrimSpace(ac.SMDP.Host) == "" {
		return ParsedActivationCode{}, fmt.Errorf("激活码里没有 SM-DP+ 地址")
	}
	parts := strings.Split(code, "$")
	confirmationRequired := false
	switch {
	case len(parts) >= 5 && parts[4] == "1":
		confirmationRequired = true
	case len(parts) == 4 && parts[3] == "1":
		confirmationRequired = true
	}
	oid := ac.OID
	if confirmationRequired && oid == "1" {
		oid = ""
	}
	return ParsedActivationCode{
		SMDP:                 ac.SMDP.Host,
		MatchingID:           ac.MatchingID,
		OID:                  oid,
		ConfirmationRequired: confirmationRequired,
	}, nil
}

// ResolveDownloadAddress accepts either a host or a full activation QR payload.
func ResolveDownloadAddress(smdpOrCode, matchingID string) (string, string, error) {
	raw := strings.TrimSpace(smdpOrCode)
	if raw == "" {
		return "", "", fmt.Errorf("请提供激活码或 SM-DP+ 地址")
	}
	if looksLikeActivationCode(raw) {
		parsed, err := ParseActivationCode(raw)
		if err != nil {
			return "", "", err
		}
		matching := strings.TrimSpace(matchingID)
		if matching == "" {
			matching = parsed.MatchingID
		}
		return parsed.SMDP, matching, nil
	}
	host := stripDownloadScheme(raw)
	if !looksLikeHost(host) {
		return "", "", fmt.Errorf("请提供激活码或 SM-DP+ 地址")
	}
	return host, strings.TrimSpace(matchingID), nil
}

// ResolveDownloadQuery prefers a parsed activation payload, then a bare SM-DP+ host.
func ResolveDownloadQuery(activationCode, smdp, matchingID string) (string, string, error) {
	activationCode = strings.TrimSpace(activationCode)
	smdp = strings.TrimSpace(smdp)
	if activationCode != "" {
		host, matching, err := ResolveDownloadAddress(activationCode, matchingID)
		if err == nil && looksLikeHost(host) {
			return host, matching, nil
		}
		if smdp == "" {
			if err != nil {
				return "", "", err
			}
			return "", "", fmt.Errorf("请提供激活码或 SM-DP+ 地址")
		}
	}
	return ResolveDownloadAddress(smdp, matchingID)
}

func parseLabeledActivation(raw string) (ParsedActivationCode, bool) {
	smdp := firstSubmatch(smdpFieldPattern, raw)
	if smdp == "" || !looksLikeHost(smdp) {
		return ParsedActivationCode{}, false
	}
	matching := firstSubmatch(matchingFieldPattern, raw)
	if !looksLikeMatchingID(matching) {
		return ParsedActivationCode{}, false
	}
	confirmation := firstSubmatch(confirmFieldPattern, raw)
	return ParsedActivationCode{
		SMDP:                 stripDownloadScheme(smdp),
		MatchingID:           matching,
		ConfirmationRequired: confirmation != "",
		ConfirmationCode:     confirmation,
	}, true
}

func parseTwoLineActivation(raw string) (ParsedActivationCode, bool) {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) != 2 {
		return ParsedActivationCode{}, false
	}
	switch {
	case looksLikeHost(lines[0]) && looksLikeMatchingID(lines[1]):
		return ParsedActivationCode{SMDP: stripDownloadScheme(lines[0]), MatchingID: lines[1]}, true
	case looksLikeHost(lines[1]) && looksLikeMatchingID(lines[0]):
		return ParsedActivationCode{SMDP: stripDownloadScheme(lines[1]), MatchingID: lines[0]}, true
	default:
		return ParsedActivationCode{}, false
	}
}

func extractActivationCode(raw string) (string, error) {
	value := compactActivationText(raw)
	if value == "" {
		return "", fmt.Errorf("激活码不能为空")
	}
	if strings.Contains(value, "%") {
		if decoded, err := url.QueryUnescape(value); err == nil && decoded != "" {
			value = compactActivationText(decoded)
		}
	}
	if parsedURL, err := url.Parse(value); err == nil && parsedURL.RawQuery != "" {
		query := parsedURL.Query()
		for _, key := range []string{"carddata", "activationcode", "activation_code", "lpa", "data"} {
			if found := strings.TrimSpace(query.Get(key)); found != "" {
				value = compactActivationText(found)
				break
			}
		}
	}
	upper := strings.ToUpper(value)
	switch {
	case strings.Contains(upper, "LPA:"):
		value = value[strings.Index(upper, "LPA:"):]
	case strings.HasPrefix(value, "1$"):
		value = "LPA:" + value
	case hostTokenPattern.MatchString(value):
		parts := hostTokenPattern.FindStringSubmatch(value)
		value = strings.Join([]string{"LPA:1", stripDownloadScheme(parts[1]), parts[2], parts[3], parts[4]}, "$")
	default:
		return "", fmt.Errorf("不是可识别的 eSIM 激活码，请粘贴 LPA:1$ 二维码内容，或包含 SM-DP+ 地址和激活码的文本")
	}
	value = normalizeLPAPrefix(value)
	if !strings.HasPrefix(value, "LPA:1") {
		return "", fmt.Errorf("无效的 eSIM 激活码")
	}
	return value, nil
}

func compactActivationText(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.ReplaceAll(value, "\n", "")
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func normalizeLPAPrefix(value string) string {
	if len(value) < 4 || !strings.EqualFold(value[:4], "lpa:") {
		return value
	}
	rest := value[4:]
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
	}
	return "LPA:" + rest
}

func stripDownloadScheme(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	return strings.Trim(value, "/")
}

func looksLikeHost(raw string) bool {
	host := stripDownloadScheme(raw)
	if !strings.Contains(host, ".") {
		return false
	}
	for _, r := range host {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == ':' {
			continue
		}
		return false
	}
	return true
}

func looksLikeMatchingID(raw string) bool {
	if looksLikeHost(raw) || len(raw) < 4 {
		return false
	}
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == ':' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func firstSubmatch(pattern *regexp.Regexp, raw string) string {
	match := pattern.FindStringSubmatch(raw)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}
