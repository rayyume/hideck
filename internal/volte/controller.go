package volte

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"github.com/iniwex5/vowifi-go/runtimehost/voicehost"
	"github.com/yibaiba/hideck/pkg/logger"
)

type Controller struct {
	host           Host
	provision      *Provisioner
	audio          *AudioRuntime
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
	return NewControllerWithBackup(host, DefaultBackupDir)
}

func NewControllerWithBackup(host Host, backupDir string) *Controller {
	c := &Controller{host: host, sess: make(map[string]*session)}
	c.provision = NewProvisioner(host, &FileStore{Dir: backupDir})
	return c
}

func (c *Controller) SetAudioRuntime(audio *AudioRuntime) {
	if c == nil {
		return
	}
	c.audio = audio
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
	if err := c.provisionConfig(ctx, deviceID); err != nil {
		return err
	}
	c.patch(deviceID, func(st *Status) {
		if st.Phase == PhaseEnabling {
			st.Phase = PhaseRegistering
		}
	})
	c.attachIMS(deviceID)
	c.attachVoice(deviceID)
	c.refreshRegistration(ctx, deviceID)
	return nil
}

func (c *Controller) Restore(ctx context.Context, deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if c == nil || c.provision == nil || deviceID == "" {
		return errors.New("volte: controller is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := c.provision.Restore(ctx, deviceID)
	c.applyProvision(deviceID, res)
	if err != nil {
		return c.fail(deviceID, err)
	}
	c.Disable(deviceID)
	return nil
}

func (c *Controller) Disable(deviceID string) {
	deviceID = strings.TrimSpace(deviceID)
	if c == nil || deviceID == "" {
		return
	}
	if c.host != nil {
		if err := c.host.ReleaseIMSClients(deviceID); err != nil {
			logger.Warn("释放原生 IMS 客户端失败", "device", deviceID, "err", err)
		}
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
	return st.Phase == PhaseRegistered || st.Phase == PhaseUnverified || st.Phase == PhaseEnabling || st.Phase == PhaseRegistering
}

func (c *Controller) provisionConfig(ctx context.Context, deviceID string) error {
	res, err := c.provision.Ensure(ctx, deviceID, Desired{
		IMSEnabled:   boolPtr(true),
		VoLTEEnabled: boolPtr(true),
		UACEnabled:   boolPtr(true),
	})
	c.applyProvision(deviceID, res)
	if err != nil && !res.Current.IMSEnabled && !errors.Is(err, ErrRebootRequired) && !errors.Is(err, ErrFieldDrift) {
		return c.fail(deviceID, err)
	}
	if err != nil {
		c.setError(deviceID, err)
	}
	if err := c.host.SetNativeIMS(ctx, deviceID, true); err != nil {
		logger.Warn("QMI 打开原生 IMS 失败，继续用 AT 结果", "device", deviceID, "err", err)
	}
	if err := c.host.EnsureIMSClients(ctx, deviceID); err != nil {
		logger.Warn("QMI IMS/IMSA 客户端不可用", "device", deviceID, "err", err)
		c.patch(deviceID, func(st *Status) { st.QMIIMSUnavailable = true })
		return c.fail(deviceID, err)
	}
	audio := strings.TrimSpace(c.host.AudioDevice(deviceID))
	c.patch(deviceID, func(st *Status) { st.AudioDevice = audio })
	if !res.Current.IMSEnabled {
		return c.fail(deviceID, fmt.Errorf("native IMS did not enable"))
	}
	return nil
}

func (c *Controller) applyProvision(deviceID string, res Result) {
	c.patch(deviceID, func(st *Status) {
		st.IMSEnabled = res.Current.IMSEnabled
		st.VoLTEEnabled = res.Current.VoLTEEnabled
		st.UACEnabled = res.Current.UACEnabled && res.Verified
		st.RebootRequired = res.RebootRequired
		st.ProvisionStage = res.Stage
		st.IMEITail = res.IMEITail
	})
}

func (c *Controller) attachIMS(deviceID string) {
	if c == nil || c.host == nil {
		return
	}
	_ = c.host.OnIMSRegistration(deviceID, func(info *qmi.IMSARegistrationStatus) {
		c.patch(deviceID, func(st *Status) { applyIMSARegistration(st, info) })
	})
	_ = c.host.OnIMSServices(deviceID, func(info *qmi.IMSAServicesStatus) {
		c.patch(deviceID, func(st *Status) {
			if info != nil && info.HasVoiceServiceStatus {
				st.VoiceAvailable = info.VoiceServiceStatus == qmi.IMSAServiceAvailabilityAvailable
			}
		})
	})
}

func applyIMSARegistration(st *Status, info *qmi.IMSARegistrationStatus) {
	if st == nil || info == nil || !info.HasStatus {
		return
	}
	switch info.Status {
	case qmi.IMSARegistrationStateRegistered, qmi.IMSARegistrationStateLimitedRegistered:
		st.IMSRegistered = true
		st.Phase = PhaseRegistered
		st.LastError = ""
	case qmi.IMSARegistrationStateRegistering:
		st.IMSRegistered = false
		st.Phase = PhaseRegistering
	default:
		st.IMSRegistered = false
		st.Phase = PhaseFailed
	}
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
