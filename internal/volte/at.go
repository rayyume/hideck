package volte

import (
	"fmt"
	"strconv"
	"strings"
)

type IMSConfig struct {
	IMSEnabled   bool
	VoLTEEnabled bool
	Raw          string
}

type USBConfig struct {
	VID           string
	PID           string
	Fields        []string
	UACEnabled    bool
	EnableCommand string
	Raw           string
}

func ParseIMSConfig(resp string) (IMSConfig, error) {
	line, ok := findQCFGLine(resp, "ims")
	if !ok {
		return IMSConfig{}, fmt.Errorf("volte: missing +QCFG: \"ims\" in %q", compactAT(resp))
	}
	parts := splitQCFGArgs(line)
	if len(parts) < 2 {
		return IMSConfig{}, fmt.Errorf("volte: malformed IMS config %q", line)
	}
	imsOn, err := parse01(parts[0])
	if err != nil {
		return IMSConfig{}, fmt.Errorf("volte: ims enable: %w", err)
	}
	volteOn := imsOn
	if len(parts) > 1 {
		volteOn, err = parse01(parts[1])
		if err != nil {
			return IMSConfig{}, fmt.Errorf("volte: volte enable: %w", err)
		}
	}
	return IMSConfig{IMSEnabled: imsOn, VoLTEEnabled: volteOn, Raw: line}, nil
}

func IMSEnableCommand() string {
	return `AT+QCFG="ims",1,1`
}

func IMSQueryCommand() string {
	return `AT+QCFG="ims"?`
}

func USBConfigQueryCommand() string {
	return `AT+QCFG="usbcfg"?`
}

func ParseUSBConfig(resp string) (USBConfig, error) {
	line, ok := findQCFGLine(resp, "usbcfg")
	if !ok {
		return USBConfig{}, fmt.Errorf("volte: missing +QCFG: \"usbcfg\" in %q", compactAT(resp))
	}
	parts := splitQCFGArgs(line)
	if len(parts) < 3 {
		return USBConfig{}, fmt.Errorf("volte: malformed usbcfg %q", line)
	}
	cfg := USBConfig{
		VID:    parts[0],
		PID:    parts[1],
		Fields: parts,
		Raw:    line,
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	cfg.UACEnabled = last == "1"
	if !cfg.UACEnabled {
		enabled := append([]string(nil), parts...)
		enabled[len(enabled)-1] = "1"
		cfg.EnableCommand = `AT+QCFG=` + strings.Join(quoteUSBCFG(enabled), ",")
	}
	return cfg, nil
}

func findQCFGLine(resp, key string) (string, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	for _, raw := range strings.Split(resp, "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		prefix := `+qcfg: "` + key + `"`
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		_, rest, ok := strings.Cut(line, ",")
		if !ok {
			return "", false
		}
		return strings.TrimSpace(rest), true
	}
	return "", false
}

func splitQCFGArgs(args string) []string {
	parts := strings.Split(args, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func parse01(v string) (bool, error) {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return false, err
	}
	switch n {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("expected 0 or 1, got %s", v)
	}
}

func quoteUSBCFG(parts []string) []string {
	out := make([]string, len(parts)+1)
	out[0] = `"usbcfg"`
	copy(out[1:], parts)
	return out
}

func compactAT(resp string) string {
	return strings.Join(strings.Fields(resp), " ")
}
