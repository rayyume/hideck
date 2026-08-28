package audiotranscode

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	evsCMRNone          = 0x7
	evsHeaderFullCMR    = 0x80
	evsHeaderFullToCFT  = 0x3f
	evsHeaderFullFollow = 0x40
	evsIOFrameTypeBase  = 16
	evsIOFrameTypeSID   = 25
	evsAMRWBModeSID     = 9
)

// 3GPP TS 26.445 A.2.1 compact AMR-WB IO protected payload sizes, including CMR and padding.
var evsAMRWBIOProtectedBits = [...]int{136, 184, 256, 288, 320, 368, 400, 464, 480}

// 6.60 pads 1 bit and 8.85 pads 4 bits so compact sizes stay unique.
var evsCompactIOPadding = [...]int{1, 4, 0, 0, 0, 0, 0, 0, 0}

var evsAMRWBBitrates = [...]float64{6.6, 8.85, 12.65, 14.25, 15.85, 18.25, 19.85, 23.05, 23.85}

func evsIOPayloadToStorage(payload []byte) ([]byte, error) {
	return evsIOPayloadToStorageBits(payload, len(payload)*8)
}

func evsIOPayloadToStorageBits(payload []byte, bits int) ([]byte, error) {
	if len(payload) == 0 || bits <= 0 {
		return nil, errors.New("truncated EVS RTP payload")
	}
	need := (bits + 7) / 8
	if need > len(payload) {
		return nil, errors.New("truncated EVS RTP payload")
	}
	payload = payload[:need]
	if mode, ok := evsCompactIOMode(bits); ok {
		return unpackEVSCompactIO(payload, mode)
	}
	return parseEVSHeaderFullIO(payload)
}

func evsCompactIOMode(bits int) (int, bool) {
	for mode, protected := range evsAMRWBIOProtectedBits {
		if bits == protected {
			return mode, true
		}
	}
	return 0, false
}

func unpackEVSCompactIO(payload []byte, mode int) ([]byte, error) {
	bits := amrWBSpeechBits[mode]
	if bits <= 0 || mode >= len(amrWBFrameBytes) {
		return nil, fmt.Errorf("unsupported EVS AMR-WB IO mode %d", mode)
	}
	reader := amrBitReader{data: payload}
	if _, err := reader.read(3); err != nil {
		return nil, errors.New("truncated EVS AMR-WB IO payload")
	}
	speech := make([]byte, amrWBFrameBytes[mode])
	for index := 1; index < bits; index++ {
		bit, err := reader.read(1)
		if err != nil {
			return nil, errors.New("truncated EVS AMR-WB IO speech")
		}
		if bit == 1 {
			setSpeechBit(speech, index)
		}
	}
	d0, err := reader.read(1)
	if err != nil {
		return nil, errors.New("truncated EVS AMR-WB IO speech")
	}
	if d0 == 1 {
		setSpeechBit(speech, 0)
	}
	return append([]byte{byte(mode<<3) | amrQualityBit}, speech...), nil
}

func storageToEVSIOCompact(storage []byte, cmr uint8) ([]byte, error) {
	mode, speech, err := evsStorageSpeech(storage)
	if err != nil {
		return nil, err
	}
	if mode == evsAMRWBModeSID {
		return storageToEVSIOHeaderFull(storage, cmr)
	}
	bits := amrWBSpeechBits[mode]
	var writer amrBitWriter
	writer.write(3, uint(cmr&0x7))
	for index := 1; index < bits; index++ {
		if speechBit(speech, index) {
			writer.write(1, 1)
		} else {
			writer.write(1, 0)
		}
	}
	if speechBit(speech, 0) {
		writer.write(1, 1)
	} else {
		writer.write(1, 0)
	}
	if pad := evsCompactIOPadding[mode]; pad > 0 {
		writer.write(pad, 0)
	}
	return writer.bytes(), nil
}

func storageToEVSIOHeaderFull(storage []byte, cmr uint8) ([]byte, error) {
	mode, speech, err := evsStorageSpeech(storage)
	if err != nil {
		return nil, err
	}
	ft := evsIOFrameTypeSID
	if mode <= 8 {
		ft = evsIOFrameTypeBase + mode
	}
	out := make([]byte, 2+len(speech))
	out[0] = evsHeaderFullCMR | ((cmr & 0x7) << 4)
	out[1] = byte(ft & evsHeaderFullToCFT)
	copy(out[2:], speech)
	return out, nil
}

func parseEVSHeaderFullIO(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("truncated EVS header-full payload")
	}
	index := 0
	if payload[0]&evsHeaderFullCMR != 0 {
		index++
	}
	if index >= len(payload) {
		return nil, errors.New("truncated EVS header-full table of contents")
	}
	toc := payload[index]
	if toc&evsHeaderFullCMR != 0 {
		return nil, errors.New("expected EVS header-full ToC")
	}
	if toc&evsHeaderFullFollow != 0 {
		return nil, errors.New("multiple EVS frames per RTP packet are unsupported")
	}
	ft := int(toc & evsHeaderFullToCFT)
	index++
	speech := payload[index:]
	switch {
	case ft >= evsIOFrameTypeBase && ft <= evsIOFrameTypeBase+8:
		mode := ft - evsIOFrameTypeBase
		want := amrWBFrameBytes[mode]
		if len(speech) < want {
			return nil, errors.New("truncated EVS AMR-WB IO speech")
		}
		return append([]byte{byte(mode<<3) | amrQualityBit}, speech[:want]...), nil
	case ft == evsIOFrameTypeSID:
		want := amrWBFrameBytes[evsAMRWBModeSID]
		if len(speech) < want {
			return nil, errors.New("truncated EVS AMR-WB IO SID")
		}
		return append([]byte{byte(evsAMRWBModeSID<<3) | amrQualityBit}, speech[:want]...), nil
	default:
		return nil, fmt.Errorf("EVS primary frame type %d is not AMR-WB IO", ft)
	}
}

func evsStorageSpeech(storage []byte) (int, []byte, error) {
	if len(storage) == 0 {
		return 0, nil, errors.New("empty AMR-WB storage frame")
	}
	mode := int((storage[0] >> 3) & 0x0f)
	if mode > evsAMRWBModeSID || (mode <= 8 && amrWBSpeechBits[mode] == 0) {
		return 0, nil, fmt.Errorf("unsupported EVS AMR-WB IO mode %d", mode)
	}
	want := amrWBFrameBytes[mode]
	if len(storage) < 1+want {
		return 0, nil, errors.New("truncated AMR-WB storage frame")
	}
	return mode, storage[1 : 1+want], nil
}

func speechBit(speech []byte, index int) bool {
	return speech[index/8]&(1<<uint(7-index%8)) != 0
}

func setSpeechBit(speech []byte, index int) {
	speech[index/8] |= 1 << uint(7-index%8)
}

func selectEVSMode(fmtp string) (int, error) {
	fields := parseFMTP(fmtp)
	if strings.TrimSpace(fields["mode-set"]) != "" {
		return selectAMRMode(fmtp, 8)
	}
	br := strings.TrimSpace(fields["br"])
	if br == "" {
		return 8, nil
	}
	high := br
	if hyphen := strings.LastIndex(br, "-"); hyphen >= 0 {
		high = strings.TrimSpace(br[hyphen+1:])
	}
	rate, err := strconv.ParseFloat(high, 64)
	if err != nil || rate <= 0 {
		return 0, fmt.Errorf("invalid EVS br %q", br)
	}
	return evsBitrateToAMRWBMode(rate), nil
}

func evsBitrateToAMRWBMode(rate float64) int {
	selected := 0
	for mode, bitrate := range evsAMRWBBitrates {
		if bitrate <= rate+0.001 {
			selected = mode
		}
	}
	return selected
}

func evsHeaderFullFMTP(fmtp string) bool {
	return parseFMTP(fmtp)["hf-only"] == "1"
}
