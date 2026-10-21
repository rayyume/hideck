//go:build !linux && !windows

package ipsec

import "syscall"

const soReusePort = syscall.SO_REUSEPORT
