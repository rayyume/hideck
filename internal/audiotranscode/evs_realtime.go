package audiotranscode

import (
	"fmt"
	"strings"
)

// PacketCodec is a bidirectional RTP payload codec for one call leg.
type PacketCodec interface {
	SampleRate() int
	Decode(payload []byte) ([]int16, error)
	Encode(pcm []int16) ([]byte, error)
	Close() error
}

type evsIOCodec struct {
	amr        *RealtimeCodec
	headerFull bool
}

func (t *Transcoder) ValidateRealtimeCodec(codec string) error {
	name := strings.ToUpper(strings.TrimSpace(codec))
	if name == "EVS" {
		name = "AMR-WB"
	}
	_, err := t.realtimeAPI(name)
	return err
}

func (t *Transcoder) NewRealtimeCodec(codec, fmtp string) (PacketCodec, error) {
	name := strings.ToUpper(strings.TrimSpace(codec))
	if name == "EVS" {
		return t.newEVSIOCodec(fmtp)
	}
	api, err := t.realtimeAPI(name)
	if err != nil {
		return nil, err
	}
	config, err := realtimeConfig(name, api)
	if err != nil {
		return nil, err
	}
	mode, err := selectAMRMode(fmtp, config.maxMode)
	if err != nil {
		return nil, fmt.Errorf("%s realtime codec: %w", name, err)
	}
	instance, err := newRealtimeCodec(config, mode)
	if err != nil {
		return nil, err
	}
	instance.octetAligned = hasOctetAlignedFMTP(fmtp)
	return instance, nil
}

func (t *Transcoder) newEVSIOCodec(fmtp string) (*evsIOCodec, error) {
	api, err := t.realtimeAPI("AMR-WB")
	if err != nil {
		return nil, fmt.Errorf("EVS AMR-WB IO: %w", err)
	}
	config, err := realtimeConfig("AMR-WB", api)
	if err != nil {
		return nil, err
	}
	mode, err := selectEVSMode(fmtp)
	if err != nil {
		return nil, fmt.Errorf("EVS realtime codec: %w", err)
	}
	amr, err := newRealtimeCodec(config, mode)
	if err != nil {
		return nil, err
	}
	return &evsIOCodec{amr: amr, headerFull: evsHeaderFullFMTP(fmtp)}, nil
}

func (codec *evsIOCodec) SampleRate() int { return codec.amr.SampleRate() }

func (codec *evsIOCodec) Close() error { return codec.amr.Close() }

func (codec *evsIOCodec) Decode(payload []byte) ([]int16, error) {
	frame, err := evsIOPayloadToStorage(payload)
	if err != nil {
		return nil, err
	}
	badFrame := 0
	if frame[0]&amrQualityBit == 0 {
		badFrame = 1
	}
	return codec.amr.decodeStorageFrame(frame, badFrame)
}

func (codec *evsIOCodec) Encode(pcm []int16) ([]byte, error) {
	storage, err := codec.amr.encodeStorageFrame(pcm)
	if err != nil {
		return nil, err
	}
	if codec.headerFull {
		return storageToEVSIOHeaderFull(storage, evsCMRNone)
	}
	return storageToEVSIOCompact(storage, evsCMRNone)
}
