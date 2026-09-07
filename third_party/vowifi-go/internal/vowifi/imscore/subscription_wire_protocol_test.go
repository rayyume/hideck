package imscore

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

func TestSubscriptionWireAcceptsNotifyBeforeFinalResponse(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		t.Run(fmt.Sprintf("mwi=%t", mwi), func(t *testing.T) {
			s := newProtectedKeepaliveTestService(t)
			client, peer := net.Pipe()
			s.activateProtectedRegistrationTCP(client)
			t.Cleanup(func() { _ = peer.Close() })
			_ = peer.SetDeadline(time.Now().Add(2 * time.Second))
			wireDone := make(chan error, 1)
			go func() { wireDone <- serveEarlySubscriptionNotify(peer) }()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			send := s.sendSubscribeReg
			if mwi {
				send = s.sendSubscribeMWI
			}
			if err := send(ctx); err != nil {
				t.Fatal(err)
			}
			if err := <-wireDone; err != nil {
				t.Fatal(err)
			}
			s.mu.RLock()
			defer s.mu.RUnlock()
			f := s.subscriptionFieldsLocked(mwi)
			if *f.expires != time.Minute || !f.lifecycle.notifyDeadline.IsZero() || f.dialog.remoteTag != "reg-notifier" {
				t.Fatalf("wire NOTIFY state lost after 200: %+v expires=%s", f.lifecycle, *f.expires)
			}
		})
	}
}

func serveEarlySubscriptionNotify(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	raw, err := readSIPStreamMessage(reader)
	if err != nil {
		return err
	}
	message, err := parseSIPMessage(raw)
	if err != nil {
		return err
	}
	notify := notifyForSubscription(message.(*sip.Request), "active;expires=60", 1)
	if _, err := io.WriteString(conn, notify); err != nil {
		return err
	}
	reply, err := readSIPStreamMessage(reader)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(reply, "SIP/2.0 200") {
		return fmt.Errorf("early NOTIFY rejected: %s", reply)
	}
	_, err = io.WriteString(conn, subscriptionWireResponse(raw, 200, "Expires: 120\r\n"))
	return err
}

func TestSubscriptionFirstNotifySelectsDialogNotResponseFork(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		for _, responseFirst := range []bool{false, true} {
			s := newSubscriptionLifecycleTestService(t, nil)
			result := beginProtocolSubscription(t, s, mwi)
			if responseFirst {
				completeProtocolSubscription(t, s, mwi, result)
			}
			raw := strings.Replace(notifyForSubscription(result.request, "active;expires=60", 1),
				"tag=reg-notifier", "tag=notify-selected", 1)
			raw = strings.Replace(raw, "Content-Type:", "Record-Route: <sip:first.example;lr>\r\n"+
				"Record-Route: <sip:second.example;lr>\r\nContent-Type:", 1)
			_, status := s.acceptSubscriptionNotification(raw)
			if status != 200 {
				t.Fatalf("first NOTIFY rejected after responseFirst=%t: %d", responseFirst, status)
			}
			if !responseFirst {
				completeProtocolSubscription(t, s, mwi, result)
			}
			f := s.subscriptionFieldsLocked(mwi)
			if f.dialog.remoteTag != "notify-selected" || len(f.dialog.routeSet) != 2 || f.dialog.routeSet[0] != "<sip:first.example;lr>" {
				t.Fatalf("200 replaced NOTIFY dialog: %+v", f.dialog)
			}
			_, status = s.acceptSubscriptionNotification(notifyForSubscription(result.request, "terminated;reason=rejected", 2))
			if status != 481 || *f.closed {
				t.Fatal("second fork changed the selected usage")
			}
		}
	}
}

func TestSubscriptionAcceptsZeroNotifyCSeqAndRejectsOlderUpdates(t *testing.T) {
	s := newSubscriptionLifecycleTestService(t, nil)
	result := beginProtocolSubscription(t, s, false)
	raw := notifyForSubscription(result.request, "active;expires=60", 0)
	notification, status := s.acceptSubscriptionNotification(raw)
	if status != 200 || notification.version == 0 {
		t.Fatalf("valid initial CSeq 0 rejected: %d", status)
	}
	duplicate, status := s.acceptSubscriptionNotification(raw)
	if status != 200 || duplicate.version != 0 {
		t.Fatal("duplicate NOTIFY was applied again")
	}
	_, status = s.acceptSubscriptionNotification(notifyForSubscription(result.request, "active;expires=90", 1))
	if status != 200 || s.subscriptionExpires != 90*time.Second {
		t.Fatal("newer NOTIFY was not applied")
	}
	_, status = s.acceptSubscriptionNotification(raw)
	if status != 500 || s.subscriptionExpires != 90*time.Second {
		t.Fatal("older NOTIFY replaced the new lifetime")
	}
}

func TestSubscriptionFinalNotifyAfterUnsubscribeDoesNotResubscribe(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		initial := beginProtocolSubscription(t, s, mwi)
		completeProtocolSubscription(t, s, mwi, initial)
		_, _ = s.acceptSubscriptionNotification(notifyForSubscription(initial.request, "active;expires=120", 1))
		build := s.buildRegistrationSubscription
		if mwi {
			build = s.buildMWISubscription
		}
		request, expires, err := build(0)
		if err != nil {
			t.Fatal(err)
		}
		result := subscriptionResult{context: s.subscriptionContextLocked(), request: request, requestedExpires: expires, unsubscribe: true}
		if err := s.recordSubscriptionUsageAttempt(result, mwi); err != nil {
			t.Fatal(err)
		}
		if err := s.subscriptionSent(result, mwi); err != nil {
			t.Fatal(err)
		}
		completeProtocolSubscription(t, s, mwi, result)
		_, status := s.acceptSubscriptionNotification(notifyForSubscription(request, "terminated;reason=timeout", 2))
		f := s.subscriptionFieldsLocked(mwi)
		if status != 200 || !*f.closed || !f.lifecycle.retryAt.IsZero() || !f.lifecycle.notifyDeadline.IsZero() {
			t.Fatalf("unsubscribe final NOTIFY mishandled: %d %+v", status, f.lifecycle)
		}
		if start, _ := s.prepareSubscriptionStart(mwi); start {
			t.Fatal("REGISTER restarted an intentionally removed subscription")
		}
	}
}
