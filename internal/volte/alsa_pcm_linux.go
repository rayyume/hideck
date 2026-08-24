//go:build linux

package volte

import (
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	alsaHWParamsSize   = 608
	alsaIoctlHWParams  = 0xc2604111
	alsaIoctlPrepare   = 0x4140
	alsaIoctlDrop      = 0x4143
	alsaParamAccess    = 0
	alsaParamFormat    = 1
	alsaParamChannels  = 10
	alsaParamRate      = 11
	alsaParamPeriodSz  = 13
	alsaParamBufferSz  = 17
	alsaAccessRWInter  = 3
	alsaFormatS16LE    = 2
	alsaIntervalFirst  = 8
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
	var params [alsaHWParamsSize]byte
	binary.LittleEndian.PutUint32(params[512:], 0xffffffff) // rmask
	setALSAMask(&params, alsaParamAccess, alsaAccessRWInter)
	setALSAMask(&params, alsaParamFormat, alsaFormatS16LE)
	setALSAInterval(&params, alsaParamChannels, 1)
	setALSAInterval(&params, alsaParamRate, uint32(pcmuClockRate))
	setALSAInterval(&params, alsaParamPeriodSz, uint32(pcmuFrameSamples))
	setALSAInterval(&params, alsaParamBufferSz, uint32(pcmuFrameSamples*8))
	if err := ioctl(f, alsaIoctlHWParams, unsafe.Pointer(&params[0])); err != nil {
		return err
	}
	return ioctl(f, alsaIoctlPrepare, nil)
}

func setALSAMask(params *[alsaHWParamsSize]byte, param, bit int) {
	if param < 0 || param > 2 {
		return
	}
	off := 4 + param*32
	word := bit / 32
	shift := uint(bit % 32)
	if word < 0 || word >= 8 {
		return
	}
	val := binary.LittleEndian.Uint32(params[off+word*4:])
	binary.LittleEndian.PutUint32(params[off+word*4:], val|uint32(1)<<shift)
}

func setALSAInterval(params *[alsaHWParamsSize]byte, param int, value uint32) {
	idx := param - alsaIntervalFirst
	if idx < 0 || idx > 11 {
		return
	}
	off := 260 + idx*12
	binary.LittleEndian.PutUint32(params[off:], value)
	binary.LittleEndian.PutUint32(params[off+4:], value)
	binary.LittleEndian.PutUint32(params[off+8:], 1) // integer
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
	if p == nil || p.capt.f == nil {
		return nil, fmt.Errorf("volte: capture closed")
	}
	buf := make([]byte, pcmuFrameSamples*2)
	if _, err := p.capt.f.Read(buf); err != nil {
		return nil, err
	}
	out := make([]int16, pcmuFrameSamples)
	for i := 0; i < pcmuFrameSamples; i++ {
		out[i] = int16(buf[i*2]) | int16(buf[i*2+1])<<8
	}
	return out, nil
}

func (p *alsaPCM) WriteFrame(samples []int16) error {
	if p == nil || p.play.f == nil {
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
	_, err := p.play.f.Write(buf)
	return err
}

func (p *alsaPCM) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.play.f != nil {
		_ = ioctl(p.play.f, alsaIoctlDrop, nil)
		_ = p.play.f.Close()
		p.play.f = nil
	}
	if p.capt.f != nil {
		_ = ioctl(p.capt.f, alsaIoctlDrop, nil)
		_ = p.capt.f.Close()
		p.capt.f = nil
	}
	return nil
}

func (s alsaStream) close() error {
	if s.f == nil {
		return nil
	}
	return s.f.Close()
}
