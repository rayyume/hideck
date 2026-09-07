package imscore

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/events"
)

type reginfoNotifyFixture struct {
	seq     int
	contact string
	state   string
	full    bool
}

func reginfoNotifyForSubscription(t *testing.T, request *sip.Request, fixture reginfoNotifyFixture) string {
	t.Helper()
	documentState, event := "partial", "registered"
	if fixture.full {
		documentState = "full"
	}
	if fixture.state == "terminated" {
		event = "expired"
	}
	body := fmt.Sprintf(`<reginfo xmlns="urn:ietf:params:xml:ns:reginfo" version="%d" state="%s"><registration aor="sip:+447840844894@o2.co.uk" id="r1" state="active"><contact id="%s" state="%s" event="%s"><uri>sip:%s@10.0.0.2</uri></contact></registration></reginfo>`,
		fixture.seq, documentState, fixture.contact, fixture.state, event, fixture.contact)
	return subscriptionNotifyWithBody(t, notifyForSubscription(request, "active;expires=120", fixture.seq), body)
}

func subscriptionNotifyWithBody(t *testing.T, raw, body string) string {
	t.Helper()
	message, err := parseSIPMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	notify := message.(*sip.Request)
	notify.AppendHeader(sip.NewHeader("Contact", "<sip:server@192.0.2.1:6060>"))
	notify.SetBody([]byte(body))
	return notify.String()
}

func prepareAcceptedNotify(t *testing.T, s *Service, raw string) inboundSIPResult {
	t.Helper()
	result, err := s.prepareInboundNotification(raw)
	if err != nil || !strings.HasPrefix(result.response, "SIP/2.0 200") || result.afterReply == nil {
		t.Fatalf("NOTIFY not accepted: %v %s", err, result.response)
	}
	return result
}

func TestSubscriptionReginfoPartialNotificationsApplyInOrder(t *testing.T) {
	for _, order := range []string{"ordered", "reversed", "concurrent"} {
		t.Run(order, func(t *testing.T) {
			s := newSubscriptionLifecycleTestService(t, nil)
			result := beginProtocolSubscription(t, s, false)
			completeProtocolSubscription(t, s, false, result)
			initial := prepareAcceptedNotify(t, s, reginfoNotifyForSubscription(t, result.request,
				reginfoNotifyFixture{seq: 0, contact: "registered-contact", state: "active", full: true}))
			initial.afterReply()
			first := prepareAcceptedNotify(t, s, reginfoNotifyForSubscription(t, result.request,
				reginfoNotifyFixture{seq: 1, contact: "registered-contact", state: "terminated"}))
			second := prepareAcceptedNotify(t, s, reginfoNotifyForSubscription(t, result.request,
				reginfoNotifyFixture{seq: 2, contact: "other-device", state: "active"}))
			runNotificationCallbacks(order, []func(){first.afterReply, second.afterReply})
			if !s.notifyReconnectPending.Load() {
				t.Fatal("acknowledged partial NOTIFY terminating the current Contact was discarded")
			}
		})
	}
}

func runNotificationCallbacks(order string, callbacks []func()) {
	if order == "reversed" {
		for index := len(callbacks) - 1; index >= 0; index-- {
			callbacks[index]()
		}
		return
	}
	if order == "concurrent" {
		var completed sync.WaitGroup
		start := make(chan struct{})
		for _, callback := range callbacks {
			completed.Add(1)
			go func() { defer completed.Done(); <-start; callback() }()
		}
		close(start)
		completed.Wait()
		return
	}
	for _, callback := range callbacks {
		callback()
	}
}

func TestSubscriptionMWINotificationsApplyOnceInOrder(t *testing.T) {
	for _, order := range []string{"ordered", "reversed", "concurrent"} {
		t.Run(order, func(t *testing.T) {
			s := newSubscriptionLifecycleTestService(t, nil)
			result := beginProtocolSubscription(t, s, true)
			completeProtocolSubscription(t, s, true, result)
			received := make(channelEventSubscriber, 4)
			s.bus.Subscribe(received)
			first := prepareAcceptedNotify(t, s, notifyForSubscription(result.request, "active;expires=120", 1))
			secondRaw := subscriptionNotifyWithBody(t, notifyForSubscription(result.request, "active;expires=120", 2),
				"Messages-Waiting: yes\r\nVoice-Message: 2/0\r\n")
			second := prepareAcceptedNotify(t, s, secondRaw)
			runNotificationCallbacks(order, []func(){first.afterReply, second.afterReply})
			for _, want := range []int{1, 2} {
				select {
				case event := <-received:
					if summary, ok := event.(events.EventMWIUpdated); !ok || summary.VoiceNew != want {
						t.Fatalf("out of order MWI: %+v, want %d", event, want)
					}
				default:
					t.Fatalf("missing MWI update %d", want)
				}
			}
			first.afterReply()
			second.afterReply()
			duplicate, err := s.prepareInboundNotification(secondRaw)
			if err != nil || duplicate.afterReply != nil || !strings.HasPrefix(duplicate.response, "SIP/2.0 200") || len(received) != 0 {
				t.Fatal("duplicate NOTIFY or callback reapplied an update")
			}
			s.mu.RLock()
			defer s.mu.RUnlock()
			q := s.mwiSubscriptionLifecycle.notifications
			if q.draining || len(q.pending) != 0 || parseMWISummary(s.mwiLastSummary).voiceNew != 2 {
				t.Fatal("completed notification queue retained work or stale MWI")
			}
		})
	}
}

func TestSubscriptionRetiredQueueDoesNotBlockOrOverwriteNewRegistration(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		t.Run(fmt.Sprintf("mwi=%t", mwi), func(t *testing.T) {
			s := newSubscriptionLifecycleTestService(t, nil)
			old := beginProtocolSubscription(t, s, mwi)
			raw := reginfoNotifyForSubscription(t, old.request,
				reginfoNotifyFixture{seq: 1, contact: "registered-contact", state: "terminated"})
			if mwi {
				raw = notifyForSubscription(old.request, "active;expires=120", 1)
			}
			retired := prepareAcceptedNotify(t, s, raw)
			s.mu.Lock()
			s.endSubscriptionRegistrationLocked(false)
			s.trackSubscriptionRegistrationLocked(time.Hour)
			s.mu.Unlock()
			current := beginProtocolSubscription(t, s, mwi)
			currentRaw := reginfoNotifyForSubscription(t, current.request,
				reginfoNotifyFixture{seq: 0, contact: "registered-contact", state: "active", full: true})
			if mwi {
				currentRaw = subscriptionNotifyWithBody(t, notifyForSubscription(current.request, "active;expires=120", 0),
					"Messages-Waiting: no\r\nVoice-Message: 0/0\r\n")
			}
			accepted := prepareAcceptedNotify(t, s, currentRaw)
			accepted.afterReply()
			retired.afterReply()
			s.mu.RLock()
			defer s.mu.RUnlock()
			q := s.subscriptionFieldsLocked(mwi).lifecycle.notifications
			if len(q.pending) != 0 || q.draining || s.notifyReconnectPending.Load() || s.mwiMessagesWaiting {
				t.Fatal("old queue blocked or changed the new registration")
			}
			if !mwi && s.reginfoAOR == "" {
				t.Fatal("new registration NOTIFY was not processed")
			}
		})
	}
}

func TestSubscriptionMalformedBodyDoesNotBlockNextNotification(t *testing.T) {
	s := newSubscriptionLifecycleTestService(t, nil)
	result := beginProtocolSubscription(t, s, false)
	invalid := prepareAcceptedNotify(t, s, subscriptionNotifyWithBody(t,
		notifyForSubscription(result.request, "active;expires=120", 1), "<reginfo>"))
	valid := prepareAcceptedNotify(t, s, reginfoNotifyForSubscription(t, result.request,
		reginfoNotifyFixture{seq: 2, contact: "registered-contact", state: "active", full: true}))
	valid.afterReply()
	invalid.afterReply()
	if s.reginfoAOR == "" {
		t.Fatal("invalid body blocked the subsequent valid NOTIFY")
	}
}
