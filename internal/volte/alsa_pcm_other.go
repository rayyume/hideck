//go:build !linux

package volte

import "fmt"

func openALSAPCM(device string) (PCMPort, error) {
	if err := validateALSADevice(device); err != nil {
		return nil, err
	}
	return nil, fmt.Errorf("volte: ALSA PCM is only supported on linux")
}
