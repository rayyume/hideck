//go:build darwin

package pcsc

// macOS PCSC.framework uses Windows-style 32-bit DWORD/LONG even on arm64.
type pcscDword = uint32
type pcscLong = int32

type ioRequest struct {
	Protocol pcscDword
	Length   pcscDword
}
