package balance

import (
	"context"
	"errors"
	"testing"

	"github.com/iniwex5/vowifi-go/runtimehost/messaging"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/device"
	"github.com/yibaiba/hideck/pkg/smscodec"
)

func TestPoolGatewaySendVoWiFiSMSFallsBackToCSWithoutDoubleSend(t *testing.T) {
	pool := device.NewPool(&config.Config{})
	pool.AttachWorkerForTest(&device.Worker{ID: "wwan0", Config: config.DeviceConfig{
		ID: "wwan0", VoWiFiEnabled: true, PhoneMode: "wifi",
	}})
	vowifiCalls, csCalls := 0, 0
	pool.SetRoutedSMSTestSenders(
		func(context.Context, string, string, string, smscodec.SubmitOptions) (messaging.SendOutcome, error) {
			vowifiCalls++
			return messaging.SendOutcome{SIPCode: 503, RecommendCSFallback: true}, errors.New("503")
		},
		func(string, string, string) error { csCalls++; return nil },
	)
	if err := NewPoolGateway(pool).SendVoWiFiSMS(context.Background(), "wwan0", "85075", "INFO"); err != nil {
		t.Fatalf("SendVoWiFiSMS() = %v", err)
	}
	if vowifiCalls != 1 || csCalls != 1 {
		t.Fatalf("calls vowifi=%d cs=%d", vowifiCalls, csCalls)
	}
}

func TestPoolGatewaySendVoWiFiSMSDoesNotFallbackAfterIMSAccept(t *testing.T) {
	pool := device.NewPool(&config.Config{})
	pool.AttachWorkerForTest(&device.Worker{ID: "wwan0", Config: config.DeviceConfig{
		ID: "wwan0", VoWiFiEnabled: true, PhoneMode: "wifi",
	}})
	csCalls := 0
	pool.SetRoutedSMSTestSenders(
		func(context.Context, string, string, string, smscodec.SubmitOptions) (messaging.SendOutcome, error) {
			return messaging.SendOutcome{SIPCode: 403, RecommendCSFallback: false}, errors.New("403")
		},
		func(string, string, string) error { csCalls++; return nil },
	)
	if err := NewPoolGateway(pool).SendVoWiFiSMS(context.Background(), "wwan0", "85075", "INFO"); err == nil {
		t.Fatal("expected 403 to stay on VoWiFi")
	}
	if csCalls != 0 {
		t.Fatalf("cs calls=%d", csCalls)
	}
}
