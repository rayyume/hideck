package api

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/runtimehost/messaging"
	"github.com/yibaiba/hideck/internal/automation"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/device"
	"github.com/yibaiba/hideck/pkg/smscodec"
)

func TestExecuteSMSFallsBackToCSWithoutDoubleSend(t *testing.T) {
	pool := device.NewPool(&config.Config{})
	worker := &device.Worker{ID: "wwan0", Config: config.DeviceConfig{
		ID: "wwan0", VoWiFiEnabled: true, PhoneMode: "wifi",
	}}
	pool.AttachWorkerForTest(worker)
	vowifiCalls, csCalls := 0, 0
	pool.SetRoutedSMSTestSenders(
		func(context.Context, string, string, string, smscodec.SubmitOptions) (messaging.SendOutcome, error) {
			vowifiCalls++
			return messaging.SendOutcome{MessageID: "ims-1", SIPCode: 503, RecommendCSFallback: true}, errors.New("503")
		},
		func(string, string, string) error { csCalls++; return nil },
	)
	executor := &automaticTaskExecutor{server: &Server{pool: pool}}
	output, err := executor.executeSMS(context.Background(), worker, automation.Task{
		DeviceID: "wwan0", Environment: automation.EnvironmentVoWiFi,
		Payload: automation.Payload{Phone: "888", Message: "BAL"},
	}, nil)
	if err != nil {
		t.Fatalf("executeSMS() = %v", err)
	}
	if !strings.Contains(output, "回落") || !strings.Contains(output, "蜂窝") {
		t.Fatalf("output=%q", output)
	}
	if vowifiCalls != 1 || csCalls != 1 {
		t.Fatalf("calls vowifi=%d cs=%d", vowifiCalls, csCalls)
	}
}

func TestExecuteSMSDoesNotFallbackAfterIMSAccept(t *testing.T) {
	pool := device.NewPool(&config.Config{})
	worker := &device.Worker{ID: "wwan0", Config: config.DeviceConfig{
		ID: "wwan0", VoWiFiEnabled: true, PhoneMode: "wifi",
	}}
	pool.AttachWorkerForTest(worker)
	csCalls := 0
	pool.SetRoutedSMSTestSenders(
		func(context.Context, string, string, string, smscodec.SubmitOptions) (messaging.SendOutcome, error) {
			return messaging.SendOutcome{SIPCode: 403, RecommendCSFallback: false}, errors.New("403")
		},
		func(string, string, string) error { csCalls++; return nil },
	)
	executor := &automaticTaskExecutor{server: &Server{pool: pool}}
	_, err := executor.executeSMS(context.Background(), worker, automation.Task{
		DeviceID: "wwan0", Environment: automation.EnvironmentVoWiFi,
		Payload: automation.Payload{Phone: "888", Message: "BAL"},
	}, nil)
	if err == nil {
		t.Fatal("expected 403 to stay on VoWiFi")
	}
	if csCalls != 0 {
		t.Fatalf("cs calls=%d", csCalls)
	}
}
