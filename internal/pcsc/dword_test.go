//go:build darwin || linux

package pcsc

import (
	"runtime"
	"testing"
	"unsafe"
)

func TestPCSCDwordMatchesNativeUnsignedLong(t *testing.T) {
	size := unsafe.Sizeof(pcscDword(0))
	requestSize := unsafe.Sizeof(ioRequest{})
	switch runtime.GOOS {
	case "linux":
		if size != unsafe.Sizeof(uint(0)) {
			t.Fatalf("linux DWORD size=%d, want %d", size, unsafe.Sizeof(uint(0)))
		}
		if requestSize != 2*size {
			t.Fatalf("linux SCARD_IO_REQUEST size=%d, want %d", requestSize, 2*size)
		}
	case "darwin":
		if size != 4 {
			t.Fatalf("darwin DWORD size=%d, want 4", size)
		}
		if requestSize != 8 {
			t.Fatalf("darwin SCARD_IO_REQUEST size=%d, want 8", requestSize)
		}
	}
}
