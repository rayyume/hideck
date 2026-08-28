package audiotranscode

import (
	"bytes"
	"strings"
	"testing"
)

func TestEVSIOCompactRoundTripAllModes(t *testing.T) {
	for mode := 0; mode <= 8; mode++ {
		storage := patternedEVSStorage(mode)
		payload, err := storageToEVSIOCompact(storage, evsCMRNone)
		if err != nil {
			t.Fatalf("mode %d pack: %v", mode, err)
		}
		if got := len(payload) * 8; got != evsAMRWBIOProtectedBits[mode] {
			t.Fatalf("mode %d compact bits=%d want %d", mode, got, evsAMRWBIOProtectedBits[mode])
		}
		got, err := evsIOPayloadToStorage(payload)
		if err != nil {
			t.Fatalf("mode %d unpack: %v", mode, err)
		}
		if !bytes.Equal(got, storage) {
			t.Fatalf("mode %d storage mismatch\n got=%x\nwant=%x", mode, got, storage)
		}
	}
}

func TestEVSIOCompactMovesD0ToTheEnd(t *testing.T) {
	storage := append([]byte{byte(2<<3) | amrQualityBit}, make([]byte, amrWBFrameBytes[2])...)
	storage[1] = 0x80
	payload, err := storageToEVSIOCompact(storage, evsCMRNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 32 || payload[0] != 0xe0 || payload[31] != 0x01 {
		t.Fatalf("d(0) layout payload=%x", payload)
	}
}

func TestEVSIOHeaderFullRoundTrip(t *testing.T) {
	storage := patternedEVSStorage(4)
	payload, err := storageToEVSIOHeaderFull(storage, evsCMRNone)
	if err != nil {
		t.Fatal(err)
	}
	if payload[0] != 0xf0 || payload[1] != byte(evsIOFrameTypeBase+4) {
		t.Fatalf("header-full header=%x", payload[:2])
	}
	got, err := evsIOPayloadToStorage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, storage) {
		t.Fatalf("header-full storage mismatch\n got=%x\nwant=%x", got, storage)
	}
}

func TestEVSIOHeaderFullSID(t *testing.T) {
	storage := append([]byte{byte(evsAMRWBModeSID<<3) | amrQualityBit}, []byte{0x11, 0x22, 0x33, 0x44, 0x55}...)
	payload, err := storageToEVSIOCompact(storage, evsCMRNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 7 || payload[1] != evsIOFrameTypeSID {
		t.Fatalf("SID should use header-full, got %x", payload)
	}
	got, err := evsIOPayloadToStorage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, storage) {
		t.Fatalf("SID storage mismatch\n got=%x\nwant=%x", got, storage)
	}
}

func TestEVSIORejectsPrimaryFrameType(t *testing.T) {
	_, err := evsIOPayloadToStorage([]byte{0xf0, 0x02, 0x00})
	if err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("primary error=%v", err)
	}
}

func TestSelectEVSModeFromFMTP(t *testing.T) {
	tests := []struct {
		fmtp string
		mode int
	}{
		{"", 8},
		{"evs-mode-switch=1;br=6.6-23.85;bw=wb", 8},
		{"br=6.6-12.65", 2},
		{"br=23.85", 8},
		{"br=5.9-24.4", 8},
		{"mode-set=0,2,4", 4},
	}
	for _, test := range tests {
		mode, err := selectEVSMode(test.fmtp)
		if err != nil || mode != test.mode {
			t.Fatalf("fmtp %q mode=%d err=%v want %d", test.fmtp, mode, err, test.mode)
		}
	}
}

func patternedEVSStorage(mode int) []byte {
	speech := make([]byte, amrWBFrameBytes[mode])
	for i := range speech {
		speech[i] = byte(i*7 + 3)
	}
	if unused := (8 - amrWBSpeechBits[mode]%8) % 8; unused > 0 {
		speech[len(speech)-1] &= byte(0xff << unused)
	}
	return append([]byte{byte(mode<<3) | amrQualityBit}, speech...)
}
