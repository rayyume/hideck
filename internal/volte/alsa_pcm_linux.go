//go:build linux

package volte

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	alsaIoctlHWRefine = 0xc2604110
	alsaIoctlHWParams = 0xc2604111
	alsaIoctlPrepare  = 0x4140
	alsaIoctlDrop     = 0x4143
)

type alsaStream struct {
	f *os.File
}

type alsaPCM struct {
	device string
	play   alsaStream
	capt   alsaStream
	mu     sync.Mutex
}

func openALSAPCM(device string) (PCMPort, error) {
	card, pcmDev, err := parseALSADevice(device)
	if err != nil {
		return nil, err
	}
	play, err := openALSAStream(fmt.Sprintf("/dev/snd/pcmC%dD%dp", card, pcmDev), true)
	if err != nil {
		return nil, err
	}
	capt, err := openALSAStream(fmt.Sprintf("/dev/snd/pcmC%dD%dc", card, pcmDev), false)
	if err != nil {
		_ = play.close()
		return nil, err
	}
	return &alsaPCM{device: device, play: play, capt: capt}, nil
}

func openALSAStream(path string, playback bool) (alsaStream, error) {
	flag := unix.O_RDONLY
	if playback {
		flag = unix.O_WRONLY
	}
	f, err := os.OpenFile(path, flag, 0)
	if err != nil {
		return alsaStream{}, fmt.Errorf("volte: open %s: %w", path, err)
	}
	if err := configureALSA(f); err != nil {
		_ = f.Close()
		return alsaStream{}, fmt.Errorf("volte: configure %s: %w", path, err)
	}
	return alsaStream{f: f}, nil
}

func configureALSA(f *os.File) error {
	params := newALSAHWParams()
	constrainALSAMask(&params, alsaParamAccess, alsaAccessRWInter)
	constrainALSAMask(&params, alsaParamFormat, alsaFormatS16LE)
	constrainALSAInterval(&params, alsaParamChannels, 1)
	constrainALSAInterval(&params, alsaParamRate, uint32(pcmuClockRate))
	constrainALSAInterval(&params, alsaParamPeriodSz, uint32(pcmuFrameSamples))
	constrainALSAInterval(&params, alsaParamBufferSz, uint32(pcmuFrameSamples*4))
	if err := ioctl(f, alsaIoctlHWRefine, unsafe.Pointer(&params[0])); err != nil {
		return err
	}
	if err := ioctl(f, alsaIoctlHWParams, unsafe.Pointer(&params[0])); err != nil {
		return err
	}
	return ioctl(f, alsaIoctlPrepare, nil)
}

func ioctl(f *os.File, req uint, arg unsafe.Pointer) error {
	var ap uintptr
	if arg != nil {
		ap = uintptr(arg)
	}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), uintptr(req), ap)
	if errno != 0 {
		return errno
	}
	return nil
}

func (p *alsaPCM) ReadFrame() ([]int16, error) {
	capt := p.captureFile()
	if capt == nil {
		return nil, fmt.Errorf("volte: capture closed")
	}
	buf := make([]byte, pcmuFrameSamples*2)
	if _, err := capt.Read(buf); err != nil {
		return nil, err
	}
	out := make([]int16, pcmuFrameSamples)
	for i := 0; i < pcmuFrameSamples; i++ {
		out[i] = int16(buf[i*2]) | int16(buf[i*2+1])<<8
	}
	return out, nil
}

func (p *alsaPCM) WriteFrame(samples []int16) error {
	play := p.playbackFile()
	if play == nil {
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
	_, err := play.Write(buf)
	return err
}

func (p *alsaPCM) captureFile() *os.File {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.capt.f
}

func (p *alsaPCM) playbackFile() *os.File {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.play.f
}

func (p *alsaPCM) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	play, capt := p.play.f, p.capt.f
	p.play.f, p.capt.f = nil, nil
	p.mu.Unlock()
	closeALSAFile(play)
	closeALSAFile(capt)
	return nil
}

const alsaCloseBudget = 300 * time.Millisecond

func closeALSAFile(f *os.File) {
	if f == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_ = ioctl(f, alsaIoctlDrop, nil)
		_ = f.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(alsaCloseBudget):
		_ = f.Close()
	}
}

func (s alsaStream) close() error {
	if s.f == nil {
		return nil
	}
	return s.f.Close()
}
