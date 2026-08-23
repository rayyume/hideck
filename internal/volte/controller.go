package volte

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
	"github.com/yibaiba/hideck/pkg/logger"
)

type Controller struct {
	host           Host
	mu             sync.Mutex
	sess           map[string]*session
	globalIncoming []func(voicehost.IncomingCall)
	globalEvents   []func(voicehost.CallEvent)
}

type session struct {
	status Status
	voice  *voiceSession
}

func NewController(host Host) *Controller {
	return &Controller{host: host, sess: make(map[string]*session)}
}

func (c *Controller) Enable(ctx context.Context, deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if c == nil || c.host == nil || deviceID == "" {
		return errors.New("volte: controller is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	s := c.sess[deviceID]
	if s == nil {
		s = &session{status: Status{DeviceID: deviceID, Phase: PhaseEnabling}}
		c.sess[deviceID] = s
	}
	s.status.Phase = PhaseEnabling
	s.status.LastError = ""
	c.mu.Unlock()

	if err := c.host.StopSoftwareIMS(deviceID); err != nil {
		return c.fail(deviceID, fmt.Errorf("stop software IMS: %w", err))
	}
	if err := c.applyIMS(ctx, deviceID); err != nil {
		return c.fail(deviceID, err)
	}
	if err := c.applyUAC(deviceID); err != nil {
		logger.Warn("VoLTE UAC 查询失败", "device", deviceID, "err", err)
		c.setError(deviceID, err)
	}
	c.attachVoice(deviceID)
	c.refreshRegistration(ctx, deviceID)
	return nil
}

func (c *Controller) Disable(deviceID string) {
	deviceID = strings.TrimSpace(deviceID)
	if c == nil || deviceID == "" {
		return
	}
	c.mu.Lock()
	delete(c.sess, deviceID)
	c.mu.Unlock()
}

func (c *Controller) Status(deviceID string) Status {
	if c == nil {
		return Status{DeviceID: deviceID, Phase: PhaseIdle}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sess[strings.TrimSpace(deviceID)]
	if s == nil {
		return Status{DeviceID: deviceID, Phase: PhaseIdle}
	}
	return s.status
}

func (c *Controller) Active(deviceID string) bool {
	st := c.Status(deviceID)
	return st.Phase == PhaseRegistered || st.Phase == PhaseUnverified || st.Phase == PhaseEnabling
}

func (c *Controller) applyIMS(ctx context.Context, deviceID string) error {
	resp, err := c.host.ExecuteAT(deviceID, IMSQueryCommand(), 8*time.Second)
	if err != nil {
		return fmt.Errorf("query IMS: %w", err)
	}
	cfg, err := ParseIMSConfig(resp)
	if err != nil {
		return err
	}
	if !cfg.IMSEnabled || !cfg.VoLTEEnabled {
		if _, err := c.host.ExecuteAT(deviceID, IMSEnableCommand(), 8*time.Second); err != nil {
			return fmt.Errorf("enable IMS: %w", err)
		}
		resp, err = c.host.ExecuteAT(deviceID, IMSQueryCommand(), 8*time.Second)
		if err != nil {
			return fmt.Errorf("re-query IMS: %w", err)
		}
		cfg, err = ParseIMSConfig(resp)
		if err != nil {
			return err
		}
	}
	c.patch(deviceID, func(st *Status) {
		st.IMSEnabled = cfg.IMSEnabled
		st.VoLTEEnabled = cfg.VoLTEEnabled
		if !cfg.IMSEnabled || !cfg.VoLTEEnabled {
			st.RebootRequired = true
		}
	})
	if err := c.host.SetNativeIMS(ctx, deviceID, true); err != nil {
		logger.Warn("QMI 打开原生 IMS 失败，继续用 AT 结果", "device", deviceID, "err", err)
	}
	if err := c.host.EnsureIMSClients(ctx, deviceID); err != nil {
		logger.Warn("QMI IMS/IMSA 客户端不可用", "device", deviceID, "err", err)
		c.patch(deviceID, func(st *Status) { st.QMIIMSUnavailable = true })
	}
	return nil
}

func (c *Controller) applyUAC(deviceID string) error {
	resp, err := c.host.ExecuteAT(deviceID, USBConfigQueryCommand(), 8*time.Second)
	if err != nil {
		return err
	}
	cfg, err := ParseUSBConfig(resp)
	if err != nil {
		return err
	}
	reboot := false
	if !cfg.UACEnabled && cfg.EnableCommand != "" {
		if _, err := c.host.ExecuteAT(deviceID, cfg.EnableCommand, 8*time.Second); err != nil {
			return fmt.Errorf("enable UAC: %w", err)
		}
		reboot = true
	}
	audio := strings.TrimSpace(c.host.AudioDevice(deviceID))
	c.patch(deviceID, func(st *Status) {
		st.UACEnabled = cfg.UACEnabled || reboot
		st.AudioDevice = audio
		if reboot {
			st.RebootRequired = true
		}
	})
	return nil
}

func (c *Controller) refreshRegistration(ctx context.Context, deviceID string) {
	reg, err := c.host.IMSAStatus(ctx, deviceID)
	c.patch(deviceID, func(st *Status) {
		if err != nil {
			if st.IMSEnabled && st.VoLTEEnabled {
				st.Phase = PhaseUnverified
			}
			if st.LastError == "" {
				st.LastError = err.Error()
			}
			return
		}
		st.IMSRegistered = reg.Registered
		st.VoiceAvailable = reg.VoiceAvailable
		if reg.Registered {
			st.Phase = PhaseRegistered
			st.LastError = ""
		} else if st.IMSEnabled && st.VoLTEEnabled {
			st.Phase = PhaseUnverified
		}
	})
}

func (c *Controller) fail(deviceID string, err error) error {
	c.patch(deviceID, func(st *Status) {
		st.Phase = PhaseFailed
		st.LastError = err.Error()
	})
	return err
}

func (c *Controller) setError(deviceID string, err error) {
	if err == nil {
		return
	}
	c.patch(deviceID, func(st *Status) { st.LastError = err.Error() })
}

func (c *Controller) patch(deviceID string, fn func(*Status)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sess[deviceID]
	if s == nil {
		s = &session{status: Status{DeviceID: deviceID}}
		c.sess[deviceID] = s
	}
	fn(&s.status)
}
