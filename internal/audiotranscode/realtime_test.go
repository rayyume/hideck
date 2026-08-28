package audiotranscode

import (
	"strings"
	"testing"
)

func TestRealtimeCodecFramesAMRInBothDirections(t *testing.T) {
	for _, test := range []struct {
		name, fmtp      string
		sampleRate      int
		samplesPerFrame int
		frameSizes      []int
		speechBits      []int
		maxMode         int
	}{
		{name: "AMR", fmtp: "octet-align=1; mode-set=0,2,7", sampleRate: 8000, samplesPerFrame: 160, frameSizes: amrNBFrameBytes[:], speechBits: amrNBSpeechBits[:], maxMode: 7},
		{name: "AMR-WB", fmtp: "octet-align=1; mode-set=0,2", sampleRate: 16000, samplesPerFrame: 320, frameSizes: amrWBFrameBytes[:], speechBits: amrWBSpeechBits[:], maxMode: 8},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := &fakeRealtimeNative{}
			api := capture.api(test.frameSizes)
			config := realtimeCodecConfig{
				name: test.name, sampleRate: test.sampleRate, samplesPerFrame: test.samplesPerFrame,
				frameSizes: test.frameSizes, speechBits: test.speechBits, maxMode: test.maxMode, api: api,
			}
			mode, err := selectAMRMode(test.fmtp, test.maxMode)
			if err != nil {
				t.Fatal(err)
			}
			codec, err := newRealtimeCodec(config, mode)
			if err != nil {
				t.Fatal(err)
			}
			codec.octetAligned = true
			defer codec.Close()
			payload := amrTestPayload(test.frameSizes, mode)
			pcm, err := codec.Decode(payload)
			if err != nil {
				t.Fatal(err)
			}
			if len(pcm) != test.samplesPerFrame || capture.decodedFrame[0] != byte(mode<<3)|amrQualityBit {
				t.Fatalf("decoded samples=%d frame header=%#x", len(pcm), capture.decodedFrame[0])
			}
			encoded, err := codec.Encode(pcm)
			if err != nil {
				t.Fatal(err)
			}
			if capture.encodedMode != mode || len(encoded) != 2+test.frameSizes[mode] || encoded[0] != amrNoModeRequest {
				t.Fatalf("mode=%d payload=%x", capture.encodedMode, encoded)
			}
		})
	}
}

func TestRealtimeCodecFramesBandwidthEfficientAMR(t *testing.T) {
	capture := &fakeRealtimeNative{}
	config := realtimeCodecConfig{
		name: "AMR", sampleRate: 8000, samplesPerFrame: 160,
		frameSizes: amrNBFrameBytes[:], speechBits: amrNBSpeechBits[:], maxMode: 7,
		api: capture.api(amrNBFrameBytes[:]),
	}
	mode, err := selectAMRMode("mode-change-capability=2;max-red=0", 7)
	if err != nil || mode != 7 {
		t.Fatalf("mode=%d err=%v", mode, err)
	}
	codec, err := newRealtimeCodec(config, mode)
	if err != nil {
		t.Fatal(err)
	}
	defer codec.Close()
	storage := []byte{byte(mode<<3) | amrQualityBit}
	speech := make([]byte, amrNBFrameBytes[mode])
	for i := range speech {
		speech[i] = byte(i + 2)
	}
	storage = append(storage, speech...)
	payload, err := storageFrameToBandwidthEfficientRTP(storage, len(storage), amrNBSpeechBits[:])
	if err != nil {
		t.Fatal(err)
	}
	pcm, err := codec.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != 160 || capture.decodedFrame[0] != storage[0] {
		t.Fatalf("decoded samples=%d header=%#x", len(pcm), capture.decodedFrame[0])
	}
	encoded, err := codec.Encode(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if capture.encodedMode != mode || encoded[0]&0xf0 != 0xf0 {
		t.Fatalf("BE encode mode=%d payload=%x", capture.encodedMode, encoded)
	}
	if _, _, err := parseBandwidthEfficientFrame(encoded, amrNBSpeechBits[:]); err != nil {
		t.Fatalf("encoded BE payload is not parseable: %v", err)
	}
}

func TestRealtimeCodecDecodesMultipleAMRFramesAndAppliesCMR(t *testing.T) {
	capture := &fakeRealtimeNative{}
	config := realtimeCodecConfig{
		name: "AMR", sampleRate: 8000, samplesPerFrame: 160,
		frameSizes: amrNBFrameBytes[:], speechBits: amrNBSpeechBits[:], maxMode: 7,
		api: capture.api(amrNBFrameBytes[:]),
	}
	codec, err := newRealtimeCodec(config, 7)
	if err != nil {
		t.Fatal(err)
	}
	codec.octetAligned = true
	defer codec.Close()
	first := amrTestPayload(amrNBFrameBytes[:], 0)
	second := amrTestPayload(amrNBFrameBytes[:], 7)
	payload := []byte{0x20, first[1] | 0x80}
	payload = append(payload, first[2:]...)
	payload = append(payload, second[1])
	payload = append(payload, second[2:]...)
	pcm, err := codec.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm) != 320 {
		t.Fatalf("decoded samples=%d", len(pcm))
	}
	if codec.mode != 2 {
		t.Fatalf("CMR did not update encoder mode: %d", codec.mode)
	}
	if _, err := codec.Encode(pcm[:160]); err != nil {
		t.Fatal(err)
	}
	if capture.encodedMode != 2 {
		t.Fatalf("encode after CMR used mode %d", capture.encodedMode)
	}
}

func TestParseAMRRejectsTooManyFrames(t *testing.T) {
	payload := make([]byte, 1+maxAMRFramesPerRTP+1)
	payload[0] = amrNoModeRequest
	for i := 1; i <= maxAMRFramesPerRTP; i++ {
		payload[i] = 0x80
	}
	if _, _, err := parseOctetAlignedFrames(payload, amrNBFrameBytes[:]); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("too many frames error = %v", err)
	}
}

type fakeRealtimeNative struct {
	decodedFrame []byte
	encodedMode  int
}

func (fake *fakeRealtimeNative) api(frameSizes []int) *amrRealtimeAPI {
	return &amrRealtimeAPI{
		decoder: &amrDecoderAPI{
			init: func() uintptr { return 1 },
			decode: func(_ uintptr, frame []byte, pcm []int16, _ int) {
				fake.decodedFrame = append([]byte(nil), frame...)
				for index := range pcm {
					pcm[index] = int16(index)
				}
			},
			close: func(uintptr) {},
		},
		encoder: &amrEncoderAPI{
			init: func() uintptr { return 2 },
			encode: func(_ uintptr, mode int, _ []int16, output []byte) int {
				fake.encodedMode = mode
				length := frameSizes[mode] + 1
				output[0] = byte(mode<<3) | amrQualityBit
				for index := 1; index < length; index++ {
					output[index] = byte(index)
				}
				return length
			},
			close: func(uintptr) {},
		},
	}
}

func amrTestPayload(frameSizes []int, mode int) []byte {
	payload := make([]byte, 2+frameSizes[mode])
	payload[0], payload[1] = amrNoModeRequest, byte(mode<<3)|amrQualityBit
	for index := 2; index < len(payload); index++ {
		payload[index] = byte(index)
	}
	return payload
}
