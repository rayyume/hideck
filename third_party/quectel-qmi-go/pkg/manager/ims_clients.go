package manager

import (
	"context"
	"fmt"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

// SetIMSServiceEnabled enables or disables the modem's native IMS service
// (including VoLTE). This is used by cellular mode to suppress native IMS
// so that the software IMS stack can take over without conflict.
func (m *Manager) SetIMSServiceEnabled(ctx context.Context, enabled bool) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("manager not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.imsLifecycleMu.Lock()
	defer m.imsLifecycleMu.Unlock()
	m.mu.Lock()
	ims := m.ims
	m.mu.Unlock()
	if ims == nil {
		svc, err := m.createIMSService(ctx)
		if err != nil {
			return fmt.Errorf("allocate IMS service: %w", err)
		}
		m.mu.Lock()
		if m.ims == nil {
			m.ims = svc
			ims = svc
		} else {
			ims = m.ims
			_ = m.closeSvc(svc)
		}
		m.mu.Unlock()
	}
	v := enabled
	return ims.SetServicesEnabledSetting(ctx, qmi.IMSServicesEnabledSettingsUpdate{
		IMSServiceEnabled:   &v,
		VoiceOverLTEEnabled: &v,
	})
}

// EnsureIMSClients allocates IMS, IMSA, and IMSP on demand. Startup still skips
// these services because some SKUs return CTL 0x001f; VoLTE can retry later.
func (m *Manager) EnsureIMSClients(ctx context.Context) error {
	if m == nil || m.client == nil {
		return fmt.Errorf("manager not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	m.imsLifecycleMu.Lock()
	defer m.imsLifecycleMu.Unlock()

	if !m.hasQMIService(qmi.ServiceIMS) {
		return ErrServiceNotReady("IMS")
	}
	if !m.hasQMIService(qmi.ServiceIMSA) {
		return ErrServiceNotReady("IMSA")
	}

	m.mu.Lock()
	needIMS := m.ims == nil
	needIMSA := m.imsa == nil
	needIMSP := m.imsp == nil && m.hasQMIService(qmi.ServiceIMSP)
	m.mu.Unlock()

	var (
		ims  *qmi.IMSService
		imsa *qmi.IMSAService
		imsp *qmi.IMSPService
		err  error
	)
	rollback := func() {
		if ims != nil {
			_ = m.closeSvc(ims)
		}
		if imsa != nil {
			_ = m.closeSvc(imsa)
		}
		if imsp != nil {
			_ = m.closeSvc(imsp)
		}
	}

	if needIMS {
		ims, err = m.createIMSService(ctx)
		if err != nil {
			return fmt.Errorf("allocate IMS: %w", err)
		}
	}
	if needIMSA {
		imsa, err = m.createIMSAService(ctx)
		if err != nil {
			rollback()
			return fmt.Errorf("allocate IMSA: %w", err)
		}
	}
	if needIMSP {
		imsp, err = m.createIMSPService(ctx)
		if err != nil {
			rollback()
			return fmt.Errorf("allocate IMSP: %w", err)
		}
	}

	m.mu.Lock()
	if m.ims == nil && ims != nil {
		m.ims = ims
		ims = nil
	}
	if m.imsa == nil && imsa != nil {
		m.imsa = imsa
		imsa = nil
	}
	if m.imsp == nil && imsp != nil {
		m.imsp = imsp
		imsp = nil
	}
	ready := m.imsa
	m.mu.Unlock()
	rollback()
	if ready == nil {
		return ErrServiceNotReady("IMSA")
	}
	if cfg, ok := m.imsaIndicationRegistration(); ok {
		if err := m.registerIMSAIndicationsWithContext(ctx, ready, cfg); err != nil {
			return fmt.Errorf("register IMSA indications: %w", err)
		}
	}
	return nil
}

// ReleaseIMSClients drops on-demand IMS/IMSA/IMSP clients without tearing down
// the rest of the QMI manager. Safe to call when none are allocated.
func (m *Manager) ReleaseIMSClients() error {
	if m == nil {
		return nil
	}
	m.imsLifecycleMu.Lock()
	defer m.imsLifecycleMu.Unlock()
	m.mu.Lock()
	ims, imsa, imsp := m.ims, m.imsa, m.imsp
	m.ims, m.imsa, m.imsp = nil, nil, nil
	m.mu.Unlock()
	var first error
	if ims != nil {
		if err := m.closeSvc(ims); err != nil && first == nil {
			first = err
		}
	}
	if imsa != nil {
		if err := m.closeSvc(imsa); err != nil && first == nil {
			first = err
		}
	}
	if imsp != nil {
		if err := m.closeSvc(imsp); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (m *Manager) createIMSService(ctx context.Context) (*qmi.IMSService, error) {
	if m.newIMSService != nil {
		return m.newIMSService(ctx, m.client)
	}
	return qmi.NewIMSService(m.client)
}

func (m *Manager) createIMSAService(ctx context.Context) (*qmi.IMSAService, error) {
	if m.newIMSAService != nil {
		return m.newIMSAService(ctx, m.client)
	}
	return qmi.NewIMSAService(m.client)
}

func (m *Manager) createIMSPService(ctx context.Context) (*qmi.IMSPService, error) {
	if m.newIMSPService != nil {
		return m.newIMSPService(ctx, m.client)
	}
	return qmi.NewIMSPService(m.client)
}

func (m *Manager) registerIMSAIndicationsWithContext(ctx context.Context, imsa *qmi.IMSAService, cfg qmi.IMSAIndicationRegistration) error {
	if m.registerIMSAIndications != nil {
		return m.registerIMSAIndications(ctx, cfg)
	}
	return imsa.RegisterIndications(ctx, cfg)
}

func (m *Manager) closeSvc(c interface{ Close() error }) error {
	if c == nil {
		return nil
	}
	if m.safeClose != nil {
		return m.safeClose(c)
	}
	return c.Close()
}
