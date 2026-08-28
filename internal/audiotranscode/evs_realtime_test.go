package audiotranscode

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealtimeCodecFramesEVSIAMRWBIO(t *testing.T) {
	capture := &fakeRealtimeNative{}
	transcoder := New()
	transcoder.amrWBRealtimeOnce.Do(func() {
		transcoder.amrWBRealtime = capture.api(amrWBFrameBytes[:])
	})
	if err := transcoder.ValidateRealtimeCodec("EVS"); err != nil {
		t.Fatal(err)
	}
	codec, err := transcoder.NewRealtimeCodec("EVS", "evs-mode-switch=1;hf-only=0;br=6.6-23.85;bw=wb")
	if err != nil {
		t.Fatal(err)
	}
	defer codec.Close()
	if codec.SampleRate() != 16000 {
		t.Fatalf("sample rate=%d", codec.SampleRate())
	}
	storage := patternedEVSStorage(8)
	payload, err := storageToEVSIOCompact(storage, evsCMRNone)
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := codec.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != 320 || !bytes.Equal(capture.decodedFrame, storage) {
		t.Fatalf("decoded samples=%d frame=%x", len(pcm), capture.decodedFrame)
	}
	encoded, err := codec.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if capture.encodedMode != 8 {
		t.Fatalf("encode mode=%d", capture.encodedMode)
	}
	got, err := evsIOPayloadToStorage(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if int((got[0]>>3)&0x0f) != 8 {
		t.Fatalf("encoded storage=%x", got)
	}
}

func TestRealtimeCodecFramesEVSHeaderFullWhenRequested(t *testing.T) {
	capture := &fakeRealtimeNative{}
	transcoder := New()
	transcoder.amrWBRealtimeOnce.Do(func() {
		transcoder.amrWBRealtime = capture.api(amrWBFrameBytes[:])
	})
	codec, err := transcoder.NewRealtimeCodec("EVS", "hf-only=1;br=6.6-12.65")
	if err != nil {
		t.Fatal(err)
	}
	defer codec.Close()
	pcm := make([]int16, 320)
	encoded, err := codec.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if capture.encodedMode != 2 || encoded[0]&0x80 == 0 || encoded[1] != byte(evsIOFrameTypeBase+2) {
		t.Fatalf("header-full encode mode=%d payload=%x", capture.encodedMode, encoded)
	}
}

func TestDecodeEVSRecordingUsesAMRWBIO(t *testing.T) {
	capture := &fakeRealtimeNative{}
	storage := patternedEVSStorage(2)
	payload, err := storageToEVSIOCompact(storage, evsCMRNone)
	if err != nil {
		t.Fatal(err)
	}
	var file bytes.Buffer
	file.WriteString(evsRecordingMagic)
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(payload)*8))
	file.Write(header)
	file.Write(payload)
	input := filepath.Join(t.TempDir(), "call.evs")
	if err := os.WriteFile(input, file.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	transcoder := New()
	transcoder.amrWBDecoderOnce.Do(func() {
		transcoder.amrWBDecoder = capture.api(amrWBFrameBytes[:]).decoder
	})
	pcm, rate, err := transcoder.decodeInput(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if rate != 16000 || len(pcm) != 320 || !bytes.Equal(capture.decodedFrame, storage) {
		t.Fatalf("rate=%d samples=%d frame=%x", rate, len(pcm), capture.decodedFrame)
	}
}

func TestDecodeEVSRecordingRejectsPrimary(t *testing.T) {
	var file bytes.Buffer
	file.WriteString(evsRecordingMagic)
	payload := []byte{0xf0, 0x02, 0x00}
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(payload)*8))
	file.Write(header)
	file.Write(payload)
	input := filepath.Join(t.TempDir(), "call.evs")
	if err := os.WriteFile(input, file.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	transcoder := New()
	transcoder.amrWBDecoderOnce.Do(func() {
		transcoder.amrWBDecoder = (&fakeRealtimeNative{}).api(amrWBFrameBytes[:]).decoder
	})
	_, _, err := transcoder.decodeInput(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "primary") {
		t.Fatalf("error=%v", err)
	}
}
