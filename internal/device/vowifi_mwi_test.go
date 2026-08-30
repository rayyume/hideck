package device

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/runtimehost/eventhost"
	"github.com/yibaiba/hideck/internal/config"
)

func TestGetVoWiFiMWIEmptyUntilRecorded(t *testing.T) {
	pool := NewPool(&config.Config{})
	got := pool.GetVoWiFiMWI("wwan0")
	if got.Known || got.MessagesWaiting || got.VoiceNew != 0 {
		t.Fatalf("empty MWI = %+v", got)
	}
	pool.RecordVoWiFiMWI("wwan0", VoWiFiMWIState{
		MessagesWaiting: true, VoiceNew: 2, VoiceOld: 1, Account: "sip:user@example",
	})
	got = pool.GetVoWiFiMWI("wwan0")
	if !got.Known || !got.MessagesWaiting || got.VoiceNew != 2 || got.VoiceOld != 1 {
		t.Fatalf("recorded MWI = %+v", got)
	}
	pool.ClearVoWiFiMWI("wwan0")
	got = pool.GetVoWiFiMWI("wwan0")
	if got.Known || got.MessagesWaiting {
		t.Fatalf("cleared MWI = %+v", got)
	}
}

type incomingCallCaptureNotifier struct {
	incoming [][3]string
	raw      []string
}

func (n *incomingCallCaptureNotifier) NotifySMS(string, string, string, time.Time) {}
func (n *incomingCallCaptureNotifier) NotifyIPRotated(string, string, string, time.Duration) {
}
func (n *incomingCallCaptureNotifier) NotifyRaw(msg string) { n.raw = append(n.raw, msg) }
func (n *incomingCallCaptureNotifier) NotifyIncomingCall(deviceID, caller, callee string) {
	n.incoming = append(n.incoming, [3]string{deviceID, caller, callee})
}

func TestDispatcherRecordsMWIAndNotifiesCallWaiting(t *testing.T) {
	pool := NewPool(&config.Config{})
	notifier := &incomingCallCaptureNotifier{}
	pool.SetNotifier(notifier)
	dispatcher := poolVoWiFiRuntimeDispatcher{pool: pool}
	at := time.Date(2026, 8, 30, 7, 0, 0, 0, time.UTC)
	dispatcher.Dispatch(context.Background(), eventhost.MWIUpdated{
		DevID: "wwan0", MessagesWaiting: true, VoiceNew: 3, VoiceOld: 0, Account: "sip:a", Time: at,
	})
	got := pool.GetVoWiFiMWI("wwan0")
	if !got.Known || !got.MessagesWaiting || got.VoiceNew != 3 || !got.UpdatedAt.Equal(at) {
		t.Fatalf("MWI snapshot = %+v", got)
	}
	dispatcher.Dispatch(context.Background(), eventhost.CallWaiting{
		DevID: "wwan0", CallID: "wait-1", Caller: "+15550002", Callee: "10010",
	})
	if len(notifier.incoming) != 1 || notifier.incoming[0] != [3]string{"wwan0", "+15550002", "10010"} {
		t.Fatalf("incoming notifications = %+v", notifier.incoming)
	}
}

func TestFormatCallWaitingNotify(t *testing.T) {
	got := formatCallWaitingNotify("wwan0", "+1", "10010")
	if !strings.Contains(got, "呼叫等待") || !strings.Contains(got, "wwan0") {
		t.Fatalf("notify=%q", got)
	}
}
