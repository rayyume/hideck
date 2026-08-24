package volte

import (
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"sync"
)

var alsaDevicePattern = regexp.MustCompile(`^hw:\d+,\d+$`)

type alsaPCM struct {
	device string
	rec    *exec.Cmd
	play   *exec.Cmd
	from   io.ReadCloser
	to     io.WriteCloser
	mu     sync.Mutex
}

func validateALSADevice(device string) error {
	if !alsaDevicePattern.MatchString(device) {
		return fmt.Errorf("volte: invalid ALSA device %q", device)
	}
	return nil
}

func openALSAPCM(device string) (PCMPort, error) {
	if err := validateALSADevice(device); err != nil {
		return nil, err
	}
	recPath, err := exec.LookPath("arecord")
	if err != nil {
		return nil, fmt.Errorf("volte: arecord not found: %w", err)
	}
	playPath, err := exec.LookPath("aplay")
	if err != nil {
		return nil, fmt.Errorf("volte: aplay not found: %w", err)
	}
	args := []string{"-D", device, "-f", "S16_LE", "-r", "8000", "-c", "1", "-t", "raw", "-q"}
	rec := exec.Command(recPath, args...)
	play := exec.Command(playPath, args...)
	from, err := rec.StdoutPipe()
	if err != nil {
		return nil, err
	}
	to, err := play.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := rec.Start(); err != nil {
		return nil, fmt.Errorf("volte: start arecord: %w", err)
	}
	if err := play.Start(); err != nil {
		_ = rec.Process.Kill()
		_, _ = rec.Process.Wait()
		return nil, fmt.Errorf("volte: start aplay: %w", err)
	}
	return &alsaPCM{device: device, rec: rec, play: play, from: from, to: to}, nil
}

func (p *alsaPCM) ReadFrame() ([]int16, error) {
	if p == nil || p.from == nil {
		return make([]int16, pcmuFrameSamples), io.EOF
	}
	buf := make([]byte, pcmuFrameSamples*2)
	if _, err := io.ReadFull(p.from, buf); err != nil {
		return nil, err
	}
	out := make([]int16, pcmuFrameSamples)
	for i := 0; i < pcmuFrameSamples; i++ {
		out[i] = int16(buf[i*2]) | int16(buf[i*2+1])<<8
	}
	return out, nil
}

func (p *alsaPCM) WriteFrame(samples []int16) error {
	if p == nil || p.to == nil {
		return nil
	}
	buf := make([]byte, pcmuFrameSamples*2)
	n := len(samples)
	if n > pcmuFrameSamples {
		n = pcmuFrameSamples
	}
	for i := 0; i < n; i++ {
		v := samples[i]
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	_, err := p.to.Write(buf)
	return err
}

func (p *alsaPCM) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.to != nil {
		_ = p.to.Close()
		p.to = nil
	}
	if p.from != nil {
		_ = p.from.Close()
		p.from = nil
	}
	if p.play != nil && p.play.Process != nil {
		_ = p.play.Process.Kill()
		_, _ = p.play.Process.Wait()
		p.play = nil
	}
	if p.rec != nil && p.rec.Process != nil {
		_ = p.rec.Process.Kill()
		_, _ = p.rec.Process.Wait()
		p.rec = nil
	}
	return nil
}
