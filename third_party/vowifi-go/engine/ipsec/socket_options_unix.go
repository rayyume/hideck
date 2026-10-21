//go:build !windows

package ipsec

import "syscall"

func reuseSocketOptionsWithBind(_, _ string, raw syscall.RawConn, bindToDevice string) error {
	var optionErr error
	err := raw.Control(func(fd uintptr) {
		optionErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		if optionErr == nil {
			optionErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort, 1)
		}
		if optionErr == nil && bindToDevice != "" {
			optionErr = setSockBindToDevice(int(fd), bindToDevice)
		}
	})
	if err != nil {
		return err
	}
	return optionErr
}
