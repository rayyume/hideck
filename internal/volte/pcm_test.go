package volte

import (
	"net"
	"sync"
	"testing"
	"time"
)

type memPCM struct {
	mu      sync.Mutex
	written [][]int16
	toRead  [][]int16
	closed  bool
}

func (m *memPCM) ReadFrame() ([]int16, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.toRead) == 0 {
		return make([]int16, pcmuFrameSamples), nil
	}
	frame := m.toRead[0]
	m.toRead = m.toRead[1:]
	return frame, nil
}
func (m *memPCM) WriteFrame(frame []int16) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := append([]int16(nil), frame...)
	m.written = append(m.written, cp)
	if len(m.written) > pcmJitterDepth {
		return errorsNew("overflow")
	}
	return nil
}
func (m *memPCM) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func errorsNew(s string) error { return errStr(s) }

type errStr string

func (e errStr) Error() string { return string(e) }

func TestPCMBridgeBidirectionalPCMU(t *testing.T) {
	a, b := net.Pipe()
	left, right := newPacketPipe(a), newPacketPipe(b)
	pcmDown := &memPCM{}
	tone := make([]int16, pcmuFrameSamples)
	for i := range tone {
		tone[i] = 1234
	}
	pcmUp := &memPCM{toRead: [][]int16{tone, tone, tone}}
	down := NewPCMBridge(left, addr{"peer"}, pcmDown, false)
	up := NewPCMBridge(right, addr{"peer"}, pcmUp, false)
	defer down.Close()
	defer up.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		toPCM, _, _, _, _ := down.Stats()
		_, upFrom, _, _, _ := up.Stats()
		if toPCM > 0 && upFrom > 0 && len(pcmDown.written) > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("down stats %+v written=%d", mustStats(down), len(pcmDown.written))
}

func TestPCMBridgeListenOnlyWritesSilence(t *testing.T) {
	a, b := net.Pipe()
	left, right := newPacketPipe(a), newPacketPipe(b)
	pcm := &memPCM{toRead: [][]int16{nonzeroFrame()}}
	bridge := NewPCMBridge(left, addr{"peer"}, pcm, true)
	peer := NewPCMBridge(right, addr{"peer"}, &memPCM{}, false)
	defer bridge.Close()
	defer peer.Close()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, _, silent, _, _ := bridge.Stats()
		if silent > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("listen-only did not write silent uplink")
}

func TestPCMBridgeCloseReleases(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	pcm := &memPCM{}
	bridge := NewPCMBridge(newPacketPipe(a), addr{"peer"}, pcm, false)
	if err := bridge.Close(); err != nil {
		t.Fatal(err)
	}
	if !pcm.closed {
		t.Fatal("pcm not closed")
	}
}

func nonzeroFrame() []int16 {
	out := make([]int16, pcmuFrameSamples)
	for i := range out {
		out[i] = 9
	}
	return out
}

func mustStats(b *PCMBridge) [5]uint64 {
	a, c, d, e, f := b.Stats()
	return [5]uint64{a, c, d, e, f}
}

type addr struct{ s string }

func (a addr) Network() string { return "pipe" }
func (a addr) String() string  { return a.s }

type packetPipe struct{ net.Conn }

func newPacketPipe(c net.Conn) *packetPipe { return &packetPipe{Conn: c} }

func (p *packetPipe) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := p.Conn.Read(b)
	return n, addr{"peer"}, err
}
func (p *packetPipe) WriteTo(b []byte, _ net.Addr) (int, error) {
	return p.Conn.Write(b)
}
