package device

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
	"github.com/yibaiba/hideck/internal/modem"
	"github.com/yibaiba/hideck/internal/volte"
	"github.com/yibaiba/hideck/pkg/logger"
)

func (p *Pool) NativeVoLTEController() *volte.Controller {
	if p == nil {
		return nil
	}
	return p.volteCtl
}

func (p *Pool) IsNativeVoLTE(deviceID string) bool {
	w := p.GetWorker(strings.TrimSpace(deviceID))
	if w == nil {
		return false
	}
	return IsNativeVoLTEMode(w.Config.PhoneMode) && w.Config.VoWiFiEnabled
}

func (p *Pool) NativeVoLTEStatus(deviceID string) volte.Status {
	if p == nil || p.volteCtl == nil {
		return volte.Status{DeviceID: deviceID, Phase: volte.PhaseIdle}
	}
	return p.volteCtl.Status(deviceID)
}

func (p *Pool) RestoreNativeVoLTE(ctx context.Context, deviceID string) error {
	if p == nil || p.volteCtl == nil {
		return fmt.Errorf("VoLTE 控制器未初始化")
	}
	return p.volteCtl.Restore(ctx, strings.TrimSpace(deviceID))
}

func (p *Pool) EnableNativeVoLTE(deviceID string) error {
	if p == nil || p.volteCtl == nil {
		return fmt.Errorf("VoLTE 控制器未初始化")
	}
	deviceID = strings.TrimSpace(deviceID)
	w := p.GetWorker(deviceID)
	if w == nil {
		return fmt.Errorf("设备 %s 不存在", deviceID)
	}
	if p.IsESIMSwitching(deviceID) {
		return fmt.Errorf("设备 %s 正在切卡，暂不允许启动 VoLTE", deviceID)
	}
	class, err := ClassifyWorkerLebaraUKForControl(p.Context(), w)
	if err != nil {
		return err
	}
	if class.BlocksVoWiFi() || class.IsLebara {
		return ErrLebaraUKRFLocked
	}
	if err := p.waitQMICoreReady(deviceID, 30*time.Second); err != nil {
		logger.Warn("VoLTE 等待 QMI 就绪失败，继续尝试 AT", "device", deviceID, "err", err)
	}
	return p.volteCtl.Enable(p.Context(), deviceID)
}

func (p *Pool) ScheduleNativeVoLTE(deviceID, reason string) {
	p.scheduleNativeVoLTE(deviceID, reason)
}

func (p *Pool) scheduleNativeVoLTE(deviceID, reason string) {
	if p == nil || strings.TrimSpace(deviceID) == "" {
		return
	}
	go func() {
		if err := p.EnableNativeVoLTE(deviceID); err != nil {
			logger.Warn("启动原生 VoLTE 失败", "device", deviceID, "reason", reason, "err", err)
		}
	}()
}

func (p *Pool) stopNativeVoLTE(deviceID, reason string) {
	if p == nil || p.volteCtl == nil {
		return
	}
	p.volteCtl.Disable(deviceID)
	logger.Debug("已停止原生 VoLTE 会话", "device", deviceID, "reason", reason)
}

func (p *Pool) ExecuteAT(deviceID, cmd string, timeout time.Duration) (string, error) {
	w := p.GetWorker(deviceID)
	if w == nil {
		return "", fmt.Errorf("设备 %s 不存在", deviceID)
	}
	port := strings.TrimSpace(w.ResolvedATPort())
	if port == "" {
		return "", fmt.Errorf("设备 %s 没有 AT 口", deviceID)
	}
	session, err := modem.NewSerialAT(port, 115200, 8, 1, "N")
	if err != nil {
		return "", fmt.Errorf("打开 AT 口 %s: %w", port, err)
	}
	defer session.Close()
	return session.Execute(cmd, timeout)
}

func (p *Pool) StopSoftwareIMS(deviceID string) error {
	if p == nil {
		return nil
	}
	if !p.IsVoWiFiActive(deviceID) && p.GetVoWiFiAppForDevice(deviceID) == nil {
		return nil
	}
	return p.voWiFiHost().Disable(p.Context(), deviceID, "native_volte", true)
}

func (p *Pool) SetNativeIMS(ctx context.Context, deviceID string, enabled bool) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI", deviceID)
	}
	return w.QMICore.SetIMSServiceEnabled(ctx, enabled)
}

func (p *Pool) EnsureIMSClients(ctx context.Context, deviceID string) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI", deviceID)
	}
	return w.QMICore.EnsureIMSClients(ctx)
}

func (p *Pool) ReleaseIMSClients(deviceID string) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return nil
	}
	return w.QMICore.ReleaseIMSClients()
}

func (p *Pool) OnIMSRegistration(deviceID string, handler func(*qmi.IMSARegistrationStatus)) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI", deviceID)
	}
	return w.QMICore.OnIMSRegistrationStatus(handler)
}

func (p *Pool) OnIMSServices(deviceID string, handler func(*qmi.IMSAServicesStatus)) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI", deviceID)
	}
	return w.QMICore.OnIMSServicesStatus(handler)
}

func (p *Pool) IMSAStatus(ctx context.Context, deviceID string) (volte.Registration, error) {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return volte.Registration{}, fmt.Errorf("设备 %s 没有 QMI", deviceID)
	}
	reg, err := w.QMICore.IMSAGetIMSRegistrationStatus(ctx)
	if err != nil {
		return volte.Registration{}, err
	}
	out := volte.Registration{Registered: reg != nil && reg.HasStatus &&
		(reg.Status == qmi.IMSARegistrationStateRegistered || reg.Status == qmi.IMSARegistrationStateLimitedRegistered)}
	svc, svcErr := w.QMICore.IMSAGetIMSServicesStatus(ctx)
	if svcErr == nil && svc != nil && svc.HasVoiceServiceStatus {
		out.VoiceAvailable = svc.VoiceServiceStatus == qmi.IMSAServiceAvailabilityAvailable
	}
	return out, nil
}

func (p *Pool) AudioDevice(deviceID string) string {
	w := p.GetWorker(deviceID)
	if w == nil {
		return ""
	}
	return strings.TrimSpace(w.Config.AudioDevice)
}

func (p *Pool) VOICEDial(ctx context.Context, deviceID, number string) (uint8, error) {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return 0, fmt.Errorf("设备 %s 没有 QMI VOICE", deviceID)
	}
	return w.QMICore.VOICEDialCall(ctx, number)
}

func (p *Pool) VOICEAnswer(ctx context.Context, deviceID string, callID uint8) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI VOICE", deviceID)
	}
	_, err := w.QMICore.VOICEAnswerCall(ctx, callID)
	return err
}

func (p *Pool) VOICEHangup(ctx context.Context, deviceID string, callID uint8) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI VOICE", deviceID)
	}
	_, err := w.QMICore.VOICEEndCall(ctx, callID)
	return err
}

func (p *Pool) VOICEBurstDTMF(ctx context.Context, deviceID string, callID uint8, digits string) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI VOICE", deviceID)
	}
	_, err := w.QMICore.VOICEBurstDTMF(ctx, callID, digits)
	return err
}

func (p *Pool) OnVoiceStatus(deviceID string, handler func(*qmi.VoiceAllCallInfo)) error {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return fmt.Errorf("设备 %s 没有 QMI VOICE", deviceID)
	}
	return w.QMICore.OnVoiceCallStatus(handler)
}

func (p *Pool) VOICEGetAllCallInfo(ctx context.Context, deviceID string) (*qmi.VoiceAllCallInfo, error) {
	w := p.GetWorker(deviceID)
	if w == nil || w.QMICore == nil {
		return nil, fmt.Errorf("设备 %s 没有 QMI VOICE", deviceID)
	}
	return w.QMICore.VOICEGetAllCallInfo(ctx)
}
