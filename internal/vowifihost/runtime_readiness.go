package vowifihost

import (
	"strings"
	"sync"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost"
	"github.com/yibaiba/hideck/pkg/logger"
)

type runtimeReadinessConfig struct {
	DeviceID  string
	TraceID   string
	StartedAt time.Time
}

type runtimeReadinessTracker struct {
	manager   *Manager
	deviceID  string
	traceID   string
	startedAt time.Time
	imsOnce   sync.Once
	smsOnce   sync.Once
}

func newRuntimeReadinessTracker(manager *Manager, config runtimeReadinessConfig) *runtimeReadinessTracker {
	startedAt := config.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &runtimeReadinessTracker{
		manager: manager, deviceID: strings.TrimSpace(config.DeviceID),
		traceID: strings.TrimSpace(config.TraceID), startedAt: startedAt,
	}
}

func (tracker *runtimeReadinessTracker) Observe(event runtimehost.Event) {
	if tracker == nil || tracker.manager == nil || event.Session == nil ||
		!tracker.manager.IsCurrentInstance(tracker.deviceID, event.Session) {
		return
	}
	traceID := tracker.traceID
	if value := strings.TrimSpace(event.TraceID); value != "" {
		traceID = value
	}
	if event.State.IMSReady {
		tracker.imsOnce.Do(func() {
			logger.Info("VoWiFi IMS 注册已就绪", "trace_id", traceID,
				"device", tracker.deviceID, "cost_ms", tracker.elapsedMilliseconds())
		})
	}
	if event.State.SMSReady && !event.State.IMSReady {
		logger.Error("VoWiFi SMS 就绪事件缺少 IMS 就绪状态",
			"trace_id", traceID, "device", tracker.deviceID)
		return
	}
	if event.State.SMSReady {
		tracker.smsOnce.Do(func() { tracker.markSMSReady(traceID) })
	}
}

func (tracker *runtimeReadinessTracker) markSMSReady(traceID string) {
	activeCount := 0
	if tracker.manager.Active(tracker.deviceID) {
		activeCount = 1
	}
	if adapter := tracker.manager.hostAdapter(); adapter != nil {
		adapter.MarkRuntimeStarted(RuntimeStartedRequest{
			TraceID: traceID, DeviceID: tracker.deviceID,
			ActiveCount: activeCount, Elapsed: time.Since(tracker.startedAt),
		})
	}
	logger.Info("VoWiFi 已启用、短信模式已切换为 VoWiFi",
		"trace_id", traceID, "device", tracker.deviceID,
		"active_count", activeCount, "cost_ms", tracker.elapsedMilliseconds())
}

func (tracker *runtimeReadinessTracker) elapsedMilliseconds() int64 {
	return time.Since(tracker.startedAt).Milliseconds()
}
