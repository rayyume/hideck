package voice

import (
	"strconv"
	"strings"
)

const (
	sdpDirectionSendRecv = "sendrecv"
	sdpDirectionSendOnly = "sendonly"
	sdpDirectionRecvOnly = "recvonly"
	sdpDirectionInactive = "inactive"
)

func sdpMediaDirection(raw string) string {
	session := sdpDirectionSendRecv
	direction := session
	inAudio := false
	sawAudio := false
	for _, source := range splitSDPTextLines(raw) {
		line := strings.TrimSpace(source)
		switch {
		case strings.HasPrefix(line, "m="):
			inAudio = strings.HasPrefix(line, "m=audio")
			if inAudio {
				sawAudio = true
				direction = session
			}
		case isSDPDirectionAttribute(line):
			value := strings.TrimPrefix(line, "a=")
			if inAudio {
				direction = value
			} else {
				session = value
				if !sawAudio {
					direction = value
				}
			}
		}
	}
	return direction
}

func rewriteSDPDirection(raw, direction string) string {
	if strings.TrimSpace(raw) == "" || !isSDPDirection(direction) {
		return raw
	}
	lines := splitSDPTextLines(raw)
	rewritten := make([]string, 0, len(lines)+1)
	inAudio := false
	replaced := false
	for _, source := range lines {
		line := strings.TrimRight(source, "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "m="):
			if inAudio && !replaced {
				rewritten = append(rewritten, "a="+direction)
				replaced = true
			}
			inAudio = strings.HasPrefix(trimmed, "m=audio")
			rewritten = append(rewritten, line)
		case inAudio && isSDPDirectionAttribute(trimmed):
			if !replaced {
				rewritten = append(rewritten, "a="+direction)
				replaced = true
			}
		default:
			rewritten = append(rewritten, line)
		}
	}
	if inAudio && !replaced {
		rewritten = append(rewritten, "a="+direction)
	}
	return strings.Join(rewritten, "\r\n") + "\r\n"
}

func bumpSDPOriginVersion(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	var rewritten strings.Builder
	for _, source := range splitSDPTextLines(raw) {
		line := strings.TrimRight(source, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "o=") {
			line = incrementSDPOriginVersion(line)
		}
		rewritten.WriteString(line)
		rewritten.WriteString("\r\n")
	}
	return rewritten.String()
}

func incrementSDPOriginVersion(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 3 {
		return line
	}
	version, err := strconv.ParseUint(fields[2], 10, 64)
	if err != nil {
		return line
	}
	fields[2] = strconv.FormatUint(version+1, 10)
	return strings.Join(fields, " ")
}

func negotiateAnswerDirection(offer string, localHold bool) string {
	if localHold {
		if offer == sdpDirectionSendOnly || offer == sdpDirectionInactive {
			return sdpDirectionInactive
		}
		return sdpDirectionSendOnly
	}
	switch offer {
	case sdpDirectionSendOnly:
		return sdpDirectionRecvOnly
	case sdpDirectionRecvOnly:
		return sdpDirectionSendOnly
	case sdpDirectionInactive:
		return sdpDirectionInactive
	default:
		return sdpDirectionSendRecv
	}
}

func remoteHoldFromDirection(direction string) bool {
	return direction == sdpDirectionSendOnly || direction == sdpDirectionInactive
}

func localDirectionAllowsSend(direction string) bool {
	return direction == sdpDirectionSendRecv || direction == sdpDirectionSendOnly
}

func isSDPDirection(value string) bool {
	switch value {
	case sdpDirectionSendRecv, sdpDirectionSendOnly, sdpDirectionRecvOnly, sdpDirectionInactive:
		return true
	default:
		return false
	}
}

func isSDPDirectionAttribute(line string) bool {
	return strings.HasPrefix(line, "a=sendrecv") ||
		strings.HasPrefix(line, "a=sendonly") ||
		strings.HasPrefix(line, "a=recvonly") ||
		strings.HasPrefix(line, "a=inactive")
}

func splitSDPTextLines(raw string) []string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Split(strings.TrimSuffix(normalized, "\n"), "\n")
}
