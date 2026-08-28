package imscore

import (
	"strconv"
	"strings"
)

type mwiSummary struct {
	waiting  bool
	account  string
	voiceNew int
	voiceOld int
	raw      string
}

func parseMWISummary(body string) mwiSummary {
	summary := mwiSummary{raw: strings.TrimSpace(body)}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		switch name {
		case "messages-waiting":
			summary.waiting = strings.EqualFold(value, "yes") || value == "1" || strings.EqualFold(value, "true")
		case "message-account":
			summary.account = firstSIPHeaderURI(value)
			if summary.account == "" {
				summary.account = value
			}
		case "voice-message":
			summary.voiceNew, summary.voiceOld = parseMWICounts(value)
		}
	}
	return summary
}

func parseMWICounts(value string) (newCount, oldCount int) {
	value = strings.TrimSpace(value)
	if start := strings.IndexByte(value, '('); start >= 0 {
		value = strings.TrimSpace(value[:start])
	}
	parts := strings.Split(value, "/")
	if len(parts) > 0 {
		newCount, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	}
	if len(parts) > 1 {
		oldCount, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	}
	return newCount, oldCount
}
