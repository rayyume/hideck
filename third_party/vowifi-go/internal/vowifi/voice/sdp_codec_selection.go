package voice

import (
	"errors"
	"strconv"
	"strings"
)

func buildPreconditionStatusSDP(localSDP, remoteSDP string) (string, error) {
	selected := commonAudioPayloads(localSDP, remoteSDP)
	if len(selected) == 0 {
		return "", errors.New("voice: precondition answer selected no offered audio codec")
	}
	replacements := remotePayloadAttributes(remoteSDP, selected)
	filtered := filterLocalAudioPayloads(localSDP, selected, replacements)
	return bumpSDPOriginVersion(advertiseEstablishedSessionQoS(filtered)), nil
}

func commonAudioPayloads(localSDP, remoteSDP string) []string {
	local := audioMediaPayloadSet(localSDP)
	var selected []string
	for _, payload := range audioMediaPayloads(remoteSDP) {
		if _, ok := local[payload]; ok {
			selected = append(selected, payload)
		}
	}
	return selected
}

func audioMediaPayloadSet(sdp string) map[string]struct{} {
	payloads := make(map[string]struct{})
	for _, payload := range audioMediaPayloads(sdp) {
		payloads[payload] = struct{}{}
	}
	return payloads
}

func audioMediaPayloads(sdp string) []string {
	for _, line := range splitSDPTextLines(sdp) {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 4 && strings.EqualFold(fields[0], "m=audio") {
			return append([]string(nil), fields[3:]...)
		}
	}
	return nil
}

func remotePayloadAttributes(sdp string, selected []string) map[string]string {
	wanted := payloadSet(selected)
	attributes := make(map[string]string)
	inAudio := false
	seenAudio := false
	for _, line := range splitSDPTextLines(sdp) {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "m=") {
			inAudio = !seenAudio && strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "m=audio ")
			seenAudio = seenAudio || inAudio
			continue
		}
		if !inAudio {
			continue
		}
		kind, payload, ok := sdpPayloadAttribute(line)
		if !ok {
			continue
		}
		if _, keep := wanted[payload]; keep {
			attributes[kind+":"+payload] = strings.TrimSpace(line)
		}
	}
	return attributes
}

func filterLocalAudioPayloads(localSDP string, selected []string, replacements map[string]string) string {
	wanted := payloadSet(selected)
	var out strings.Builder
	inAudio := false
	seenAudio := false
	for _, source := range splitSDPTextLines(localSDP) {
		line := strings.TrimSpace(source)
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasPrefix(strings.ToLower(fields[0]), "m=") {
			inAudio = !seenAudio && strings.EqualFold(fields[0], "m=audio")
			seenAudio = seenAudio || inAudio
			if inAudio && len(fields) >= 4 {
				line = strings.Join(append(fields[:3], selected...), " ")
			}
		} else if inAudio {
			kind, payload, ok := sdpPayloadAttribute(line)
			if !ok {
				out.WriteString(line)
				out.WriteString("\r\n")
				continue
			}
			if _, keep := wanted[payload]; !keep {
				continue
			}
			if replacement := replacements[kind+":"+payload]; replacement != "" {
				line = replacement
			}
		}
		out.WriteString(line)
		out.WriteString("\r\n")
	}
	return out.String()
}

func payloadSet(payloads []string) map[string]struct{} {
	result := make(map[string]struct{}, len(payloads))
	for _, payload := range payloads {
		result[payload] = struct{}{}
	}
	return result
}

func sdpPayloadAttribute(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	for _, kind := range []string{"rtpmap", "fmtp", "framesize", "rtcp-fb"} {
		prefix := "a=" + kind + ":"
		if !strings.HasPrefix(strings.ToLower(line), prefix) {
			continue
		}
		payload := strings.Fields(line[len(prefix):])
		if len(payload) == 0 || payload[0] == "*" {
			return "", "", false
		}
		if _, err := strconv.Atoi(payload[0]); err != nil {
			return "", "", false
		}
		return kind, payload[0], true
	}
	return "", "", false
}
