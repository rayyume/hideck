package device

import (
	"context"
	"errors"
	"testing"

	"github.com/yibaiba/hideck/internal/backend"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/pcsc"
)

type pinFailurePCSCBackend struct {
	reader        pcsc.Reader
	opens         int
	queries       int
	verifications int
	imsiSelected  bool
}

func (state *pinFailurePCSCBackend) Readers(context.Context) ([]pcsc.Reader, error) {
	return []pcsc.Reader{state.reader}, nil
}

func (state *pinFailurePCSCBackend) Open(context.Context, pcsc.Selector) (pcsc.Card, error) {
	state.opens++
	return state, nil
}

func (state *pinFailurePCSCBackend) Close() error { return nil }

func (state *pinFailurePCSCBackend) Transmit(_ context.Context, command []byte) ([]byte, uint16, error) {
	switch command[1] {
	case 0xA4:
		state.imsiSelected = command[2] == 0 && len(command) >= 7 && command[5] == 0x6F && command[6] == 0x07
		if command[2] == 4 {
			return []byte{0x62, 0x08, 0xC6, 0x06, 0x90, 0x01, 0x80, 0x83, 0x01, 0x01}, 0x9000, nil
		}
		return nil, 0x9000, nil
	case 0xB0:
		if state.imsiSelected {
			return nil, 0x6982, nil
		}
		return []byte{0x98, 0x10, 0x32, 0x54, 0x76, 0x98, 0x10, 0x32, 0x54, 0x76}, 0x9000, nil
	case 0xB2:
		return []byte{0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}, 0x9000, nil
	case 0x20:
		if len(command) == 5 {
			state.queries++
			return nil, 0x63C3, nil
		}
		state.verifications++
		return nil, 0x6984, nil
	default:
		return nil, 0, errors.New("unexpected APDU")
	}
}

func TestReconcilePCSCReadersDoesNotRetryPINFailure(test *testing.T) {
	test.Setenv("HIDECK_TEST_PIN", "1234")
	cfg := config.DeviceConfig{
		ID: "reader-1", DeviceBackend: backend.BackendPCSC,
		PCSCReaderName: "Reader A", PCSCUSBPath: "1-2", SIMPINEnv: "HIDECK_TEST_PIN",
	}
	pool := NewPool(&config.Config{Devices: []config.DeviceConfig{cfg}})
	test.Cleanup(pool.cancel)
	native := &pinFailurePCSCBackend{reader: pcsc.Reader{Name: "Reader A", USBPath: "1-2", CardPresent: true}}
	pool.pcscService = pcsc.NewWithBackend(native)
	if _, err := pool.AddWorkerFromConfig(cfg); !errors.Is(err, pcsc.ErrPINRetryBlocked) || !errors.Is(err, pcsc.ErrPINVerification) {
		test.Fatalf("unexpected startup error: %v", err)
	}
	for _, inserted := range []bool{true, true, false, true} {
		native.reader.CardPresent = inserted
		if err := pool.reconcilePCSCReaders(rescanReconnectOptions{}, []config.DeviceConfig{cfg}, []config.DeviceConfig{cfg}); err != nil {
			test.Fatal(err)
		}
	}
	if native.opens != 1 || native.queries != 1 || native.verifications != 1 {
		test.Fatalf("automatic retry after failure: opens=%d queries=%d verifications=%d", native.opens, native.queries, native.verifications)
	}
	if _, err := pool.AddWorkerFromConfig(cfg); !errors.Is(err, pcsc.ErrPINRetryBlocked) {
		test.Fatalf("manual worker rebuild bypassed guard: %v", err)
	}
	if native.queries != 1 || native.verifications != 1 {
		test.Fatal("manual worker rebuild submitted another PIN operation")
	}
}

type pcscReaderStateBackend struct {
	readers []pcsc.Reader
}

func (state *pcscReaderStateBackend) Readers(context.Context) ([]pcsc.Reader, error) {
	return append([]pcsc.Reader(nil), state.readers...), nil
}

func (*pcscReaderStateBackend) Open(context.Context, pcsc.Selector) (pcsc.Card, error) {
	return nil, pcsc.ErrNoCard
}

type closingPCSCDeviceBackend struct {
	*workerStatusBackendStub
	closed bool
}

func (device *closingPCSCDeviceBackend) Close() error {
	device.closed = true
	return nil
}

func TestReconcilePCSCReadersRemovesWorkerAfterCardRemoval(t *testing.T) {
	cfg := config.DeviceConfig{
		ID: "reader-1", DeviceBackend: backend.BackendPCSC,
		PCSCReaderName: "Reader A", PCSCUSBPath: "1-2",
	}
	pool := NewPool(&config.Config{Devices: []config.DeviceConfig{cfg}})
	t.Cleanup(pool.cancel)
	pool.pcscService = pcsc.NewWithBackend(&pcscReaderStateBackend{readers: []pcsc.Reader{{
		Name: "Reader A", USBPath: "1-2", CardPresent: false,
	}}})
	deviceBackend := &closingPCSCDeviceBackend{workerStatusBackendStub: &workerStatusBackendStub{
		mode: backend.BackendPCSC, simInserted: true,
	}}
	pool.workers[cfg.ID] = &Worker{
		ID: cfg.ID, Config: cfg, Backend: deviceBackend, stop: make(chan struct{}), Pool: pool,
	}

	if err := pool.reconcilePCSCReaders(rescanReconnectOptions{}, []config.DeviceConfig{cfg}, []config.DeviceConfig{cfg}); err != nil {
		t.Fatal(err)
	}
	if worker := pool.GetWorker(cfg.ID); worker != nil {
		t.Fatalf("worker remained after card removal: %+v", worker)
	}
	if !deviceBackend.closed {
		t.Fatal("PC/SC backend was not closed after card removal")
	}
	if len(pool.cfg.Devices) != 1 || pool.cfg.Devices[0].ID != cfg.ID {
		t.Fatalf("configured reader was removed during hot-unplug: %+v", pool.cfg.Devices)
	}
}
