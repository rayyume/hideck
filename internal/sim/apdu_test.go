package sim

import (
	"bytes"
	"encoding/hex"
	"errors"
	"testing"

	swusim "github.com/iniwex5/vowifi-go/engine/sim"
)

func TestParseUSIMAuthResponseSyncFailureReturnsErrSyncFailureWithAUTS(t *testing.T) {
	auts := bytes.Repeat([]byte{0xCC}, 14)
	resp := append([]byte{0xDC, 0x0E}, auts...)
	resp = append(resp, 0x90, 0x00)

	got, err := ParseUSIMAuthResponse("wwan0", resp)
	if !errors.Is(err, swusim.ErrSyncFailure) {
		t.Fatalf("err = %v, want ErrSyncFailure", err)
	}
	if !bytes.Equal(got.AUTS, auts) {
		t.Fatalf("AUTS = %s, want %s", hex.EncodeToString(got.AUTS), hex.EncodeToString(auts))
	}
	if len(got.RES) != 0 {
		t.Fatalf("RES = %s, want empty on sync failure", hex.EncodeToString(got.RES))
	}
}
