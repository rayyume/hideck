package ipsec

import (
	"errors"
	"syscall"
)

func reuseSocketOptionsWithBind(_, _ string, raw syscall.RawConn, bindToDevice string) error {
	if bindToDevice != "" {
		return errors.New("SO_BINDTODEVICE is not supported on this platform")
	}
	// Windows has no SO_REUSEPORT; use its socket handle for SO_REUSEADDR.
	var optionErr error
	if err := raw.Control(func(fd uintptr) {
		optionErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	}); err != nil {
		return err
	}
	return optionErr
}
