package audiotranscode

import (
	"errors"
	"io"
)

var (
	amrNBSpeechBits = [...]int{95, 103, 118, 134, 148, 159, 204, 244, 39, 43, 38, 37, 0, 0, 0, 0}
	amrWBSpeechBits = [...]int{132, 177, 253, 285, 317, 365, 397, 461, 477, 40, 0, 0, 0, 0, 0, 0}
)

func hasOctetAlignedFMTP(fmtp string) bool {
	return parseFMTP(fmtp)["octet-align"] == "1"
}

const maxAMRFramesPerRTP = 32

type amrParsedFrame struct {
	storage []byte
	bad     int
}

func parseAMRRTPFrame(payload []byte, octetAligned bool, frameSizes, speechBits []int) ([]byte, int, error) {
	_, frames, err := parseAMRRTPFrames(payload, octetAligned, frameSizes, speechBits)
	if err != nil {
		return nil, 0, err
	}
	if len(frames) == 0 {
		return nil, 0, errors.New("truncated AMR RTP payload")
	}
	return frames[0].storage, frames[0].bad, nil
}

func parseAMRRTPFrames(payload []byte, octetAligned bool, frameSizes, speechBits []int) (int, []amrParsedFrame, error) {
	if octetAligned {
		return parseOctetAlignedFrames(payload, frameSizes)
	}
	return parseBandwidthEfficientFrames(payload, speechBits)
}

func storageFrameToNegotiatedRTP(storage []byte, written int, octetAligned bool, frameSizes, speechBits []int) ([]byte, error) {
	if octetAligned {
		return storageFrameToRTP(storage, written, frameSizes)
	}
	return storageFrameToBandwidthEfficientRTP(storage, written, speechBits)
}

func parseBandwidthEfficientFrame(payload []byte, speechBits []int) ([]byte, int, error) {
	_, frames, err := parseBandwidthEfficientFrames(payload, speechBits)
	if err != nil {
		return nil, 0, err
	}
	return frames[0].storage, frames[0].bad, nil
}

func parseBandwidthEfficientFrames(payload []byte, speechBits []int) (int, []amrParsedFrame, error) {
	if len(payload) == 0 {
		return 0, nil, errors.New("truncated AMR RTP payload")
	}
	reader := amrBitReader{data: payload}
	cmr, err := reader.read(4)
	if err != nil {
		return 0, nil, errors.New("truncated AMR RTP payload")
	}
	var toc []byte
	for {
		if len(toc) >= maxAMRFramesPerRTP {
			return 0, nil, errors.New("too many AMR frames in RTP payload")
		}
		follow, err := reader.read(1)
		if err != nil {
			return 0, nil, errors.New("truncated AMR table of contents")
		}
		frameType, err := reader.read(4)
		if err != nil {
			return 0, nil, errors.New("truncated AMR table of contents")
		}
		quality, err := reader.read(1)
		if err != nil {
			return 0, nil, errors.New("truncated AMR table of contents")
		}
		entry := byte(frameType<<3) | byte(quality<<2)
		if follow == 1 {
			entry |= 0x80
		}
		toc = append(toc, entry)
		if follow == 0 {
			break
		}
	}
	frames := make([]amrParsedFrame, 0, len(toc))
	for _, entry := range toc {
		frameType := int((entry >> 3) & 0x0f)
		speech, err := reader.readBitsAsBytes(speechBits[frameType])
		if err != nil {
			return 0, nil, errors.New("truncated AMR speech frame")
		}
		badFrame := 0
		if entry&amrQualityBit == 0 {
			badFrame = 1
		}
		frames = append(frames, amrParsedFrame{
			storage: append([]byte{entry & 0x7c}, speech...),
			bad:     badFrame,
		})
	}
	return int(cmr), frames, nil
}

func storageFrameToBandwidthEfficientRTP(storage []byte, written int, speechBits []int) ([]byte, error) {
	if written <= 0 || written > len(storage) {
		return nil, errors.New("AMR encoder returned invalid length")
	}
	frameType := int((storage[0] >> 3) & 0x0f)
	bits := speechBits[frameType]
	if bits == 0 && frameType < 14 {
		return nil, errors.New("unsupported AMR frame type")
	}
	if written != 1+(bits+7)/8 && bits > 0 {
		return nil, errors.New("AMR encoder frame length does not match speech bits")
	}
	if bits == 0 && written != 1 {
		return nil, errors.New("AMR encoder frame length does not match speech bits")
	}
	var writer amrBitWriter
	writer.write(4, 0x0f)
	writer.write(1, 0)
	writer.write(4, uint(frameType))
	if storage[0]&amrQualityBit != 0 {
		writer.write(1, 1)
	} else {
		writer.write(1, 0)
	}
	if bits > 0 {
		writer.writeBytes(storage[1:written], bits)
	}
	return writer.bytes(), nil
}

type amrBitReader struct {
	data []byte
	bit  int
}

func (r *amrBitReader) remaining() int {
	return len(r.data)*8 - r.bit
}

func (r *amrBitReader) read(n int) (uint8, error) {
	if n <= 0 {
		return 0, nil
	}
	if n > 8 || r.remaining() < n {
		return 0, io.ErrUnexpectedEOF
	}
	var value uint8
	for i := 0; i < n; i++ {
		value <<= 1
		if r.data[r.bit/8]&(1<<uint(7-r.bit%8)) != 0 {
			value |= 1
		}
		r.bit++
	}
	return value, nil
}

func (r *amrBitReader) readBitsAsBytes(n int) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	if r.remaining() < n {
		return nil, io.ErrUnexpectedEOF
	}
	out := make([]byte, (n+7)/8)
	for i := 0; i < n; i++ {
		bit, err := r.read(1)
		if err != nil {
			return nil, err
		}
		if bit == 1 {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out, nil
}

type amrBitWriter struct {
	bits []bool
}

func (w *amrBitWriter) write(n int, value uint) {
	for i := n - 1; i >= 0; i-- {
		w.bits = append(w.bits, (value>>uint(i))&1 == 1)
	}
}

func (w *amrBitWriter) writeBytes(data []byte, nbits int) {
	for i := 0; i < nbits; i++ {
		w.bits = append(w.bits, data[i/8]&(1<<uint(7-i%8)) != 0)
	}
}

func (w *amrBitWriter) bytes() []byte {
	out := make([]byte, (len(w.bits)+7)/8)
	for i, bit := range w.bits {
		if bit {
			out[i/8] |= 1 << uint(7-i%8)
		}
	}
	return out
}
