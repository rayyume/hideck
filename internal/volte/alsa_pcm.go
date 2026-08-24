package volte

import (
	"fmt"
	"regexp"
	"strconv"
)

var alsaDevicePattern = regexp.MustCompile(`^hw:(\d+),(\d+)$`)

func validateALSADevice(device string) error {
	if !alsaDevicePattern.MatchString(device) {
		return fmt.Errorf("volte: invalid ALSA device %q", device)
	}
	return nil
}

func parseALSADevice(device string) (card, pcmDev int, err error) {
	m := alsaDevicePattern.FindStringSubmatch(device)
	if m == nil {
		return 0, 0, fmt.Errorf("volte: invalid ALSA device %q", device)
	}
	card, _ = strconv.Atoi(m[1])
	pcmDev, _ = strconv.Atoi(m[2])
	return card, pcmDev, nil
}
