package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/iniwex5/vowifi-go/runtimehost/messaging"
	"github.com/yibaiba/hideck/pkg/smscodec"
)

const (
	voWiFiSMSSendRecoveryTimeout = 90 * time.Second
	RoutedSMSViaVoWiFi           = "vowifi"
	RoutedSMSViaCS               = "cs"
)

type routedSMSSendResult struct {
	Via          string
	Outcome      messaging.SendOutcome
	FellBackToCS bool
}

// ShouldFallbackVoWiFiSMSToCS reports whether IMS never accepted the MESSAGE.
func ShouldFallbackVoWiFiSMSToCS(outcome messaging.SendOutcome, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, messaging.ErrSMSNotReady) {
		return true
	}
	return outcome.RecommendCSFallback
}

func sendRoutedSMS(
	viaVoWiFi bool,
	sendVoWiFi func() (messaging.SendOutcome, error),
	sendCS func() error,
) (routedSMSSendResult, error) {
	if viaVoWiFi {
		if sendVoWiFi == nil {
			return routedSMSSendResult{Via: RoutedSMSViaVoWiFi}, errors.New("sms route: VoWiFi sender is unavailable")
		}
		outcome, err := sendVoWiFi()
		if err == nil {
			return routedSMSSendResult{Via: RoutedSMSViaVoWiFi, Outcome: outcome}, nil
		}
		if !ShouldFallbackVoWiFiSMSToCS(outcome, err) || sendCS == nil {
			return routedSMSSendResult{Via: RoutedSMSViaVoWiFi, Outcome: outcome}, err
		}
		if csErr := sendCS(); csErr != nil {
			return routedSMSSendResult{Via: RoutedSMSViaVoWiFi, Outcome: outcome, FellBackToCS: true}, err
		}
		return routedSMSSendResult{Via: RoutedSMSViaCS, Outcome: outcome, FellBackToCS: true}, nil
	}
	if sendCS == nil {
		return routedSMSSendResult{Via: RoutedSMSViaCS}, errors.New("sms route: CS sender is unavailable")
	}
	if err := sendCS(); err != nil {
		return routedSMSSendResult{Via: RoutedSMSViaCS}, err
	}
	return routedSMSSendResult{Via: RoutedSMSViaCS}, nil
}

// SendRoutedSMS sends via VoWiFi when that path is selected, and falls back to
// CS only when IMS never accepted the MESSAGE.
func (p *Pool) SendRoutedSMS(
	ctx context.Context,
	worker *Worker,
	phone, message string,
	opts smscodec.SubmitOptions,
) (routedSMSSendResult, error) {
	if p == nil {
		return routedSMSSendResult{}, errors.New("sms route: pool is nil")
	}
	if worker == nil {
		return routedSMSSendResult{}, errors.New("sms route: worker is nil")
	}
	deviceID := worker.ID
	return sendRoutedSMS(
		p.ShouldRouteSMSViaVoWiFi(deviceID),
		func() (messaging.SendOutcome, error) {
			return p.SendVoWiFiSMSWithOptions(ctx, deviceID, phone, message, opts)
		},
		func() error {
			return worker.SendSMSWithOptions(phone, message, opts)
		},
	)
}

type voWiFiSMSRuntime interface {
	State() runtimehost.State
	SendSMSWithOptions(context.Context, string, string, messaging.SendOptions) (messaging.SendOutcome, error)
}

type voWiFiSMSSendRequest struct {
	DeviceID string
	To       string
	Text     string
	Options  messaging.SendOptions
	Updates  <-chan runtimehost.State
	Runtime  func() voWiFiSMSRuntime
}

func sendVoWiFiSMSWhenReady(
	ctx context.Context,
	request voWiFiSMSSendRequest,
) (messaging.SendOutcome, error) {
	lastReason := "VoWiFi 运行时尚未建立"
	for {
		runtime := currentVoWiFiSMSRuntime(request.Runtime)
		if runtime != nil {
			state := runtime.State()
			lastReason = voWiFiSMSWaitReason(state)
			if state.SMSReady || !shouldWaitForVoWiFiSMS(state) {
				outcome, err := runtime.SendSMSWithOptions(ctx, request.To, request.Text, request.Options)
				if err == nil || !errors.Is(err, messaging.ErrSMSNotReady) {
					return outcome, err
				}
				lastReason = err.Error()
				if !shouldWaitForVoWiFiSMS(runtime.State()) {
					return outcome, err
				}
			}
		}
		if err := waitForVoWiFiSMSUpdate(ctx, request.Updates); err != nil {
			return messaging.SendOutcome{}, fmt.Errorf(
				"设备 %s 的 VoWiFi 等待短信恢复失败（最后状态：%s）: %w",
				request.DeviceID, lastReason, err,
			)
		}
	}
}

func currentVoWiFiSMSRuntime(getter func() voWiFiSMSRuntime) voWiFiSMSRuntime {
	if getter == nil {
		return nil
	}
	return getter()
}

func shouldWaitForVoWiFiSMS(state runtimehost.State) bool {
	return !state.SMSReady && !strings.EqualFold(
		strings.TrimSpace(state.SMSReadyReason), "IMS SMSC is not configured",
	)
}

func voWiFiSMSWaitReason(state runtimehost.State) string {
	if reason := strings.TrimSpace(state.SMSReadyReason); reason != "" {
		return reason
	}
	if phase := strings.TrimSpace(state.Phase); phase != "" {
		return phase
	}
	return "VoWiFi 正在恢复"
}

func waitForVoWiFiSMSUpdate(ctx context.Context, updates <-chan runtimehost.State) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case _, ok := <-updates:
		if !ok {
			return errors.New("VoWiFi 状态订阅已关闭")
		}
		return nil
	}
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
