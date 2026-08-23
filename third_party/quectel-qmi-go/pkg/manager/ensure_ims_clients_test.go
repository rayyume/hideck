package manager

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

func TestEnsureIMSClientsAllocatesRegistersAndRelease(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	m := newRecoveryTestManager()
	m.client = &qmi.Client{}
	var imsN, imsaN, imspN, closeN, indN atomic.Int32
	m.hasServiceHook = func(service uint8) bool {
		return service == qmi.ServiceIMS || service == qmi.ServiceIMSA || service == qmi.ServiceIMSP
	}
	m.newIMSService = func(context.Context, *qmi.Client) (*qmi.IMSService, error) {
		imsN.Add(1)
		return &qmi.IMSService{}, nil
	}
	m.newIMSAService = func(context.Context, *qmi.Client) (*qmi.IMSAService, error) {
		imsaN.Add(1)
		return &qmi.IMSAService{}, nil
	}
	m.newIMSPService = func(context.Context, *qmi.Client) (*qmi.IMSPService, error) {
		imspN.Add(1)
		return &qmi.IMSPService{}, nil
	}
	m.registerIMSAIndications = func(got context.Context, cfg qmi.IMSAIndicationRegistration) error {
		if _, ok := got.Deadline(); !ok {
			t.Fatal("indication context has no deadline")
		}
		if !cfg.RegistrationStatusChanged || !cfg.ServicesStatusChanged {
			t.Fatalf("indications %+v", cfg)
		}
		indN.Add(1)
		return nil
	}
	m.safeClose = func(interface{ Close() error }) error {
		closeN.Add(1)
		return nil
	}

	if err := m.EnsureIMSClients(ctx); err != nil {
		t.Fatal(err)
	}
	if imsN.Load() != 1 || imsaN.Load() != 1 || imspN.Load() != 1 || indN.Load() != 1 {
		t.Fatalf("alloc ims=%d imsa=%d imsp=%d ind=%d", imsN.Load(), imsaN.Load(), imspN.Load(), indN.Load())
	}
	if err := m.EnsureIMSClients(ctx); err != nil {
		t.Fatal(err)
	}
	if imsN.Load() != 1 || imsaN.Load() != 1 || imspN.Load() != 1 {
		t.Fatal("duplicate Ensure allocated extra clients")
	}
	if indN.Load() != 2 {
		t.Fatalf("duplicate Ensure should re-register indications, got %d", indN.Load())
	}
	if err := m.ReleaseIMSClients(); err != nil {
		t.Fatal(err)
	}
	if closeN.Load() != 3 {
		t.Fatalf("close=%d want 3", closeN.Load())
	}
	if err := m.EnsureIMSClients(ctx); err != nil {
		t.Fatal(err)
	}
	if imsN.Load() != 2 {
		t.Fatalf("after release should allocate again, ims=%d", imsN.Load())
	}
}

func TestEnsureIMSClientsMissingServiceTypedError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m := newRecoveryTestManager()
	m.client = &qmi.Client{}
	m.hasServiceHook = func(service uint8) bool { return service == qmi.ServiceIMS }
	err := m.EnsureIMSClients(ctx)
	var nr *ServiceNotReadyError
	if !errors.As(err, &nr) || nr.Service != "IMSA" {
		t.Fatalf("err=%v", err)
	}
}

func TestEnsureIMSClientsPartialAllocDoesNotLeak(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m := newRecoveryTestManager()
	m.client = &qmi.Client{}
	m.hasServiceHook = func(service uint8) bool {
		return service == qmi.ServiceIMS || service == qmi.ServiceIMSA
	}
	var closeN atomic.Int32
	m.newIMSService = func(context.Context, *qmi.Client) (*qmi.IMSService, error) {
		return &qmi.IMSService{}, nil
	}
	m.newIMSAService = func(context.Context, *qmi.Client) (*qmi.IMSAService, error) {
		return nil, errors.New("ctl 0x001f")
	}
	m.safeClose = func(interface{ Close() error }) error {
		closeN.Add(1)
		return nil
	}
	err := m.EnsureIMSClients(ctx)
	if err == nil || errors.As(err, new(*ServiceNotReadyError)) {
		t.Fatalf("want alloc error, got %v", err)
	}
	if closeN.Load() != 1 {
		t.Fatalf("IMS client leaked, close=%d", closeN.Load())
	}
	if m.ims != nil || m.imsa != nil {
		t.Fatalf("manager still holds clients ims=%v imsa=%v", m.ims, m.imsa)
	}
}

func TestEnsureIMSClientsRace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	m := newRecoveryTestManager()
	m.client = &qmi.Client{}
	m.hasServiceHook = func(service uint8) bool {
		return service == qmi.ServiceIMS || service == qmi.ServiceIMSA
	}
	m.newIMSService = func(context.Context, *qmi.Client) (*qmi.IMSService, error) {
		return &qmi.IMSService{}, nil
	}
	m.newIMSAService = func(context.Context, *qmi.Client) (*qmi.IMSAService, error) {
		return &qmi.IMSAService{}, nil
	}
	m.registerIMSAIndications = func(context.Context, qmi.IMSAIndicationRegistration) error { return nil }
	m.safeClose = func(interface{ Close() error }) error { return nil }

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.EnsureIMSClients(ctx)
			_ = m.ReleaseIMSClients()
			_ = m.EnsureIMSClients(ctx)
		}()
	}
	wg.Wait()
}
