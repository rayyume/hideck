//go:build linux

package pcsc

// Linux pcsclite maps DWORD/LONG to unsigned long / long.
// On LP64 that is 64-bit; on 32-bit ARM it is 32-bit. Go uint/int match.
type pcscDword = uint
type pcscLong = int

type ioRequest struct {
	Protocol pcscDword
	Length   pcscDword
}
