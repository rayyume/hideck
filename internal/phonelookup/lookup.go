package phonelookup

import "strings"

type Result struct {
	Number        string `json:"number"`
	DisplayNumber string `json:"display_number"`
	ContactID     string `json:"contact_id,omitempty"`
	Name          string `json:"name,omitempty"`
	Title         string `json:"title"`
	Subtitle      string `json:"subtitle"`
	Carrier       string `json:"carrier,omitempty"`
	Region        string `json:"region,omitempty"`
	Country       string `json:"country,omitempty"`
	Kind          string `json:"kind"`
}

func Lookup(raw string) Result {
	return LookupWithRegion(raw, "")
}

func LookupWithRegion(raw, region string) Result {
	normalized := NormalizeWithRegion(raw, region)
	out := Result{
		Number:        normalized,
		DisplayNumber: displayNumber(raw, normalized),
		Kind:          "unknown",
	}
	if normalized == "" {
		out.Title = strings.TrimSpace(raw)
		if out.Title == "" {
			out.Title = "未知号码"
		}
		return out
	}

	cc, national := nationalDigits(normalized)
	candidates := []string{digitsOnly(normalized), national, strings.TrimPrefix(normalized, "+")}
	if svc, ok := matchService(candidates...); ok {
		out.Kind = "service"
		out.Carrier = svc.carrier
		out.Region = "客服"
		out.Country = "中国"
		out.Title = out.DisplayNumber
		out.Subtitle = joinMeta(out.Carrier, out.Region)
		return out
	}

	if cc == "86" {
		if rec, ok := lookupCNHLR(national); ok {
			out.Kind = "mobile"
			out.Carrier = rec.CardType
			out.Country = "中国"
			out.Region = formatCNRegion(rec.Province, rec.City)
			out.Title = formatCNMobile(national)
			out.DisplayNumber = out.Title
			out.Subtitle = joinMeta(out.Carrier, out.Region)
			return out
		}
		if carrier, ok := matchCNMobile(national); ok {
			out.Kind = "mobile"
			out.Carrier = carrier
			out.Country = "中国"
			out.Region = intlRegionHint(normalized, "中国")
			if out.Region == "" {
				out.Region = "中国"
			}
			out.Title = formatCNMobile(national)
			out.DisplayNumber = out.Title
			out.Subtitle = joinMeta(out.Carrier, out.Region)
			return out
		}
		if area, ok := matchCNArea(national); ok {
			out.Kind = "landline"
			out.Country = "中国"
			out.Region = area
			out.Carrier = "固定电话"
			out.Title = out.DisplayNumber
			out.Subtitle = joinMeta(out.Region, out.Country)
			return out
		}
		out.Country = "中国"
	}

	info := lookupIntl(normalized)
	if info.Country != "" {
		out.Country = info.Country
	} else if country, ok := matchCountry(normalized); ok {
		out.Country = country
	}
	if info.Region != "" {
		out.Region = info.Region
	}
	if info.Carrier != "" {
		out.Carrier = info.Carrier
	}
	out.Title = out.DisplayNumber
	out.Subtitle = joinMeta(out.Carrier, out.Region, out.Country)
	if out.Kind == "unknown" {
		if info.Kind != "" {
			out.Kind = info.Kind
		} else if out.Country != "" {
			out.Kind = "international"
		}
	}
	return out
}

func intlRegionHint(e164, country string) string {
	info := lookupIntl(e164)
	if info.Region == "" || info.Region == country {
		return ""
	}
	return info.Region
}

func (r Result) WithName(name string) Result {
	name = strings.TrimSpace(name)
	if name == "" {
		return r
	}
	r.Name = name
	r.Title = name
	return r
}

func (r Result) WithContact(id, name string) Result {
	r = r.WithName(name)
	r.ContactID = strings.TrimSpace(id)
	return r
}

func displayNumber(raw, normalized string) string {
	if d := digitsOnly(raw); len(d) > 0 && len(d) <= 6 {
		return d
	}
	if strings.HasPrefix(normalized, "+86") {
		nat := strings.TrimPrefix(normalized, "+86")
		if _, ok := cnServiceNumbers[nat]; ok {
			return nat
		}
		if len(nat) == 11 && nat[0] == '1' {
			return formatCNMobile(nat)
		}
	}
	if normalized != "" {
		return normalized
	}
	return strings.TrimSpace(raw)
}

func formatCNMobile(national string) string {
	if len(national) == 11 {
		return national[:3] + " " + national[3:7] + " " + national[7:]
	}
	return national
}

func joinMeta(parts ...string) string {
	var out []string
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return strings.Join(out, " · ")
}

func matchLongest(haystack string, table map[string]string) (string, bool) {
	best, label := "", ""
	for prefix, name := range table {
		if strings.HasPrefix(haystack, prefix) && len(prefix) >= len(best) {
			best, label = prefix, name
		}
	}
	if best == "" {
		return "", false
	}
	return label, true
}
