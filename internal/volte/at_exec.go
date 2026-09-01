package volte

import (
	"errors"
	"strings"
	"time"
)

const (
	mbnListTimeout   = 30 * time.Second
	atRetryPause     = 200 * time.Millisecond
	atTimeoutRetries = 1
)

func atTimeout(cmd string) time.Duration {
	switch strings.TrimSpace(cmd) {
	case MBNListQueryCommand(), MBNActivateCommand():
		return mbnListTimeout
	default:
		return defaultATTimeout
	}
}

func isATTimeout(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out")
}

func execAT(at ATTransport, deviceID, cmd string) (string, error) {
	if at == nil {
		return "", errors.New("volte provision: AT transport is not configured")
	}
	timeout := atTimeout(cmd)
	var (
		lastResp string
		last     error
	)
	attempts := 1 + atTimeoutRetries
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(atRetryPause)
		}
		resp, err := at.ExecuteAT(deviceID, cmd, timeout)
		err = atResult(resp, err)
		if err == nil {
			return resp, nil
		}
		lastResp, last = resp, err
		if !isATTimeout(err) {
			return resp, err
		}
	}
	return lastResp, last
}
