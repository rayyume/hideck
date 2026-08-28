package media

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestRTPAudioRecorderWritesAMRWB(t *testing.T) {
	base := filepath.Join(t.TempDir(), "call")
	recorder, err := newRTPAudioRecorder(base, []AudioCodec{{
		PayloadType: 104, Name: "AMR-WB", ClockRate: 16000, Fmtp: "octet-align=1; max-red=0",
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{0xf0, 0x04}, make([]byte, amrWBFrameBytes[0])...)
	if err := recorder.writeRTP(testRTPPacket(104, payload)); err != nil {
		t.Fatal(err)
	}
	if err := recorder.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base + ".amr-wb")
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:9]) != "#!AMR-WB\n" || data[9] != 0x04 || len(data) != 27 {
		t.Fatalf("AMR-WB recording is malformed: length=%d prefix=%q toc=%#x", len(data), data[:9], data[9])
	}
}

func TestRTPAudioRecorderWritesPCMWave(t *testing.T) {
	base := filepath.Join(t.TempDir(), "call")
	recorder, err := newRTPAudioRecorder(base, []AudioCodec{{
		PayloadType: 0, Name: "PCMU", ClockRate: 8000,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.writeRTP(testRTPPacket(0, make([]byte, 160))); err != nil {
		t.Fatal(err)
	}
	if err := recorder.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base + ".wav")
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		t.Fatalf("WAV header is malformed: %q", data[:12])
	}
	if got := binary.LittleEndian.Uint32(data[40:44]); got != 320 || len(data) != wavHeaderLength+320 {
		t.Fatalf("WAV data length=%d file length=%d", got, len(data))
	}
}

func TestRTPAudioRecorderWritesBandwidthEfficientAMR(t *testing.T) {
	speech := patternedAMRSpeech(amrFrameBytes[7], amrSpeechBits[7])
	want := recordOctetAlignedAMR(t, "AMR", 102, 8000, 7, speech)
	got := recordBandwidthEfficientAMR(t, "AMR", 102, 8000, amrSpeechBits[:], 7, speech)
	if string(got) != string(want) {
		t.Fatalf("BE AMR-NB storage mismatch\n got=%x\nwant=%x", got, want)
	}
}

func TestRTPAudioRecorderWritesBandwidthEfficientAMRWB(t *testing.T) {
	speech := patternedAMRSpeech(amrWBFrameBytes[2], amrWBSpeechBits[2])
	want := recordOctetAlignedAMR(t, "AMR-WB", 104, 16000, 2, speech)
	got := recordBandwidthEfficientAMR(t, "AMR-WB", 104, 16000, amrWBSpeechBits[:], 2, speech)
	if string(got) != string(want) {
		t.Fatalf("BE AMR-WB storage mismatch\n got=%x\nwant=%x", got, want)
	}
}

func TestRTPAudioRecorderWritesBandwidthEfficientAMRMultipleFrames(t *testing.T) {
	speech0 := patternedAMRSpeech(amrFrameBytes[0], amrSpeechBits[0])
	speech7 := patternedAMRSpeech(amrFrameBytes[7], amrSpeechBits[7])
	base := filepath.Join(t.TempDir(), "call")
	recorder, err := newRTPAudioRecorder(base, []AudioCodec{{
		PayloadType: 102, Name: "AMR", ClockRate: 8000, Fmtp: "mode-change-capability=2;max-red=0",
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload := packAMRBandwidthEfficient(0x0f, []amrPackedFrame{
		{frameType: 0, quality: true, follow: true, speech: speech0, bits: amrSpeechBits[0]},
		{frameType: 7, quality: true, follow: false, speech: speech7, bits: amrSpeechBits[7]},
	})
	if err := recorder.writeRTP(testRTPPacket(102, payload)); err != nil {
		t.Fatal(err)
	}
	if err := recorder.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base + ".amr")
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte("#!AMR\n"), 0x04)
	want = append(want, speech0...)
	want = append(want, 0x3c)
	want = append(want, speech7...)
	if string(data) != string(want) {
		t.Fatalf("multi-frame BE AMR storage mismatch\n got=%x\nwant=%x", data, want)
	}
}

func TestRTPAudioRecorderWritesEVS(t *testing.T) {
	base := filepath.Join(t.TempDir(), "call")
	recorder, err := newRTPAudioRecorder(base, []AudioCodec{{
		PayloadType: 106, Name: "EVS", ClockRate: 16000, Fmtp: "br=5.9-24.4;bw=nb-wb",
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0x70, 0x11, 0x22, 0x33, 0x44, 0x55}
	if err := recorder.writeRTP(testRTPPacket(106, payload)); err != nil {
		t.Fatal(err)
	}
	if err := recorder.close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(base + ".evs")
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:12]) != "#!EVS_MC1.0\n" {
		t.Fatalf("EVS header=%q", data[:12])
	}
	if binary.BigEndian.Uint16(data[12:14]) != uint16(len(payload)*8) {
		t.Fatalf("EVS frame bits=%d", binary.BigEndian.Uint16(data[12:14]))
	}
	if string(data[14:]) != string(payload) {
		t.Fatalf("EVS payload=%x", data[14:])
	}
}

func TestSelectRecordableCodecAcceptsBandwidthEfficientAMR(t *testing.T) {
	codec, ext, err := selectRecordableCodec([]AudioCodec{{
		PayloadType: 102, Name: "AMR", ClockRate: 8000, Fmtp: "mode-change-capability=2;max-red=0",
	}})
	if err != nil || ext != ".amr" || codec.PayloadType != 102 {
		t.Fatalf("codec=%+v ext=%q err=%v", codec, ext, err)
	}
}

func TestRTPAudioRecorderRejectsEmptyRecording(t *testing.T) {
	recorder, err := newRTPAudioRecorder(filepath.Join(t.TempDir(), "call"), []AudioCodec{{
		PayloadType: 0, Name: "PCMU", ClockRate: 8000,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.close(); err == nil || err.Error() != "media: recording contains no audio frames" {
		t.Fatalf("close error=%v", err)
	}
}

func testRTPPacket(payloadType int, payload []byte) []byte {
	packet := make([]byte, rtpFixedHeaderLength+len(payload))
	packet[0] = 0x80
	packet[1] = byte(payloadType)
	copy(packet[rtpFixedHeaderLength:], payload)
	return packet
}

func patternedAMRSpeech(n, bits int) []byte {
	speech := make([]byte, n)
	for i := range speech {
		speech[i] = byte(i*7 + 3)
	}
	if bits > 0 && n > 0 {
		if unused := (8 - bits%8) % 8; unused > 0 {
			speech[n-1] &= byte(0xff << unused)
		}
	}
	return speech
}

func recordOctetAlignedAMR(t *testing.T, name string, payloadType, clockRate, frameType int, speech []byte) []byte {
	t.Helper()
	base := filepath.Join(t.TempDir(), "oa")
	recorder, err := newRTPAudioRecorder(base, []AudioCodec{{
		PayloadType: payloadType, Name: name, ClockRate: clockRate, Fmtp: "octet-align=1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload := append([]byte{0xf0, byte(frameType<<3) | 0x04}, speech...)
	if err := recorder.writeRTP(testRTPPacket(payloadType, payload)); err != nil {
		t.Fatal(err)
	}
	if err := recorder.close(); err != nil {
		t.Fatal(err)
	}
	ext := ".amr"
	if name == "AMR-WB" {
		ext = ".amr-wb"
	}
	data, err := os.ReadFile(base + ext)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func recordBandwidthEfficientAMR(t *testing.T, name string, payloadType, clockRate int, speechBits []int, frameType int, speech []byte) []byte {
	t.Helper()
	base := filepath.Join(t.TempDir(), "be")
	recorder, err := newRTPAudioRecorder(base, []AudioCodec{{
		PayloadType: payloadType, Name: name, ClockRate: clockRate, Fmtp: "mode-change-capability=2;max-red=0",
	}})
	if err != nil {
		t.Fatal(err)
	}
	payload := packAMRBandwidthEfficient(0x0f, []amrPackedFrame{{
		frameType: frameType, quality: true, speech: speech, bits: speechBits[frameType],
	}})
	if err := recorder.writeRTP(testRTPPacket(payloadType, payload)); err != nil {
		t.Fatal(err)
	}
	if err := recorder.close(); err != nil {
		t.Fatal(err)
	}
	ext := ".amr"
	if name == "AMR-WB" {
		ext = ".amr-wb"
	}
	data, err := os.ReadFile(base + ext)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type amrPackedFrame struct {
	frameType int
	quality   bool
	follow    bool
	speech    []byte
	bits      int
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

func packAMRBandwidthEfficient(cmr uint8, frames []amrPackedFrame) []byte {
	var writer amrBitWriter
	writer.write(4, uint(cmr&0x0f))
	for _, frame := range frames {
		if frame.follow {
			writer.write(1, 1)
		} else {
			writer.write(1, 0)
		}
		writer.write(4, uint(frame.frameType&0x0f))
		if frame.quality {
			writer.write(1, 1)
		} else {
			writer.write(1, 0)
		}
	}
	for _, frame := range frames {
		writer.writeBytes(frame.speech, frame.bits)
	}
	return writer.bytes()
}
