package imscore

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/events"
)

func TestSubscriptionReplyFailureDoesNotLoseOrBlockUpdates(t *testing.T) {
	writeErr := errors.New("NOTIFY response write failed")
	for _, test := range []struct {
		name  string
		reply func(string) error
	}{
		{name: "write failure", reply: func(string) error { return writeErr }},
		{name: "missing reply path"},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := newSubscriptionLifecycleTestService(t, nil)
			result := beginProtocolSubscription(t, s, true)
			completeProtocolSubscription(t, s, true, result)
			received := make(channelEventSubscriber, 4)
			s.bus.Subscribe(received)
			firstRaw := notifyForSubscription(result.request, "active;expires=120", 1)
			err := s.dispatchInboundSIP(firstRaw, test.reply)
			if err == nil || (test.reply != nil && !errors.Is(err, writeErr)) {
				t.Fatalf("response error was hidden: %v", err)
			}
			waitForMWINotification(t, received, 1)
			acknowledge := func(raw string) error {
				if !strings.HasPrefix(raw, "SIP/2.0 200") {
					return errors.New("expected NOTIFY 200 response")
				}
				return nil
			}
			if err := s.dispatchInboundSIP(firstRaw, acknowledge); err != nil {
				t.Fatal(err)
			}
			secondRaw := subscriptionNotifyWithBody(t, notifyForSubscription(result.request, "active;expires=120", 2),
				"Messages-Waiting: yes\r\nVoice-Message: 2/0\r\n")
			if err := s.dispatchInboundSIP(secondRaw, acknowledge); err != nil {
				t.Fatal(err)
			}
			waitForMWINotification(t, received, 2)
			if len(received) != 0 {
				t.Fatal("retransmitted NOTIFY applied its body twice")
			}
		})
	}
}

func waitForMWINotification(t *testing.T, received channelEventSubscriber, want int) {
	t.Helper()
	select {
	case event := <-received:
		if summary, ok := event.(events.EventMWIUpdated); !ok || summary.VoiceNew != want {
			t.Fatalf("MWI update = %+v, want %d new messages", event, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("notification queue did not deliver MWI update %d", want)
	}
}
