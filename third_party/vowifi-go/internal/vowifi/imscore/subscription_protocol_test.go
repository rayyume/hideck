package imscore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

func primeSubscriptionNotifyDialog(s *Service, mwi bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.subscriptionFieldsLocked(mwi)
	*f.dialog = registrationSubscriptionDialog{callID: "notify-call", localTag: "client", cseq: 5}
	if mwi {
		f.dialog.callID, f.dialog.localTag = "mwi-call", "mwi-client"
	}
	*f.lifecycle = subscriptionLifecycle{context: s.subscriptionContextLocked(), started: true,
		sentAt: time.Now(), notifyDeadline: time.Now().Add(32 * time.Second)}
}

func beginProtocolSubscription(t *testing.T, s *Service, mwi bool) subscriptionResult {
	t.Helper()
	attempt, err := s.beginSubscriptionAttempt(mwi, false)
	if err != nil {
		t.Fatal(err)
	}
	build := s.buildRegistrationSubscription
	if mwi {
		build = s.buildMWISubscription
	}
	request, expires, err := build(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result := subscriptionResult{context: attempt, request: request, requestedExpires: expires}
	if err := s.recordSubscriptionUsageAttempt(result, mwi); err != nil {
		t.Fatal(err)
	}
	if err := s.subscriptionSent(result, mwi); err != nil {
		t.Fatal(err)
	}
	return result
}

func completeProtocolSubscription(t *testing.T, s *Service, mwi bool, result subscriptionResult) {
	t.Helper()
	expires := min(result.requestedExpires, 120*time.Second)
	message, err := parseSIPMessage(subscriptionWireResponse(result.request.String(), 200,
		fmt.Sprintf("Expires: %d\r\n", int64(expires/time.Second))))
	if err != nil {
		t.Fatal(err)
	}
	result.response = message.(*sip.Response)
	if err := s.recordSubscriptionUsageResult(result, mwi); err != nil {
		t.Fatal(err)
	}
}

func notifyForSubscription(request *sip.Request, state string, seq int) string {
	body, contentType := "<reginfo/>", reginfoContentType
	if sipEventPackage(request.String()) == mwiEventPackage {
		body, contentType = "Messages-Waiting: yes\r\nVoice-Message: 1/0\r\n", mwiContentType
	}
	return fmt.Sprintf("NOTIFY sip:user@example SIP/2.0\r\n"+
		"Via: SIP/2.0/TCP 192.0.2.1:6060;branch=z9hG4bK-notify-%d\r\n"+
		"From: <sip:server@example>;tag=reg-notifier\r\nTo: %s\r\nCall-ID: %s\r\n"+
		"CSeq: %d NOTIFY\r\nEvent: %s\r\nSubscription-State: %s\r\n"+
		"Content-Type: %s\r\nContent-Length: %d\r\n\r\n%s", seq, request.From().Value(),
		request.CallID().Value(), seq, rawSIPHeaderValue(request.String(), "Event"), state, contentType, len(body), body)
}

func TestSubscriptionNotifyBefore200PreservesAuthoritativeExpiry(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		result := beginProtocolSubscription(t, s, mwi)
		raw := notifyForSubscription(result.request, "active;expires=60", 1)
		accepted, err := s.prepareInboundNotification(raw)
		if err != nil || !strings.HasPrefix(accepted.response, "SIP/2.0 200") {
			t.Fatalf("early NOTIFY rejected: %v %s", err, accepted.response)
		}
		expiresAt := s.subscriptionFieldsLocked(mwi).lifecycle.expiresAt
		completeProtocolSubscription(t, s, mwi, result)
		f := s.subscriptionFieldsLocked(mwi)
		if *f.expires != time.Minute || !f.lifecycle.expiresAt.Equal(expiresAt) || !f.lifecycle.notifyDeadline.IsZero() {
			t.Fatalf("200 overwrote NOTIFY state: %+v expires=%s", f.lifecycle, *f.expires)
		}
		if *f.closed || !f.dialog.ready() {
			t.Fatal("early NOTIFY failed to establish the subscription")
		}
	}
}

func TestSubscriptionFailedRefreshConsumesCSeqAndKeepsExpiry(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		initial := beginProtocolSubscription(t, s, mwi)
		completeProtocolSubscription(t, s, mwi, initial)
		_, status := s.acceptSubscriptionNotification(notifyForSubscription(initial.request, "active;expires=60", 1))
		if status != 200 {
			t.Fatal(status)
		}
		f := s.subscriptionFieldsLocked(mwi)
		expiresAt := f.lifecycle.expiresAt
		failed := beginProtocolSubscription(t, s, mwi)
		if !f.lifecycle.expiresAt.Equal(expiresAt) || *f.expires != time.Minute {
			t.Fatal("refresh attempt extended the previous lifetime")
		}
		failed.response = sip.NewResponse(503, "Service Unavailable")
		failed.response.AppendHeader(sip.NewHeader("Retry-After", "7200"))
		failed.err = errors.New("SUBSCRIBE rejected with status 503")
		if err := s.recordSubscriptionUsageResult(failed, mwi); err == nil {
			t.Fatal("503 was hidden")
		}
		if *f.closed || !f.lifecycle.expiresAt.Equal(expiresAt) || !f.lifecycle.notifyDeadline.IsZero() {
			t.Fatal("recoverable failure changed the valid subscription")
		}
		retry := f.lifecycle.retryAt
		if time.Until(retry) < 119*time.Minute {
			t.Fatal("Retry-After was shortened")
		}
		f.lifecycle.retryAt = time.Now().Add(-time.Second)
		next := beginProtocolSubscription(t, s, mwi)
		if next.request.CallID().Value() != failed.request.CallID().Value() || next.request.CSeq().SeqNo != failed.request.CSeq().SeqNo+1 {
			t.Fatalf("failed refresh reused CSeq: %s / %s", failed.request.CSeq(), next.request.CSeq())
		}
		if next.request.Via().Value() == failed.request.Via().Value() {
			t.Fatal("new transaction reused its branch")
		}
	}
}

func TestSubscriptionRejectsUnmatchedNotifyWithoutChangingState(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		for _, replace := range []func(string) string{
			func(raw string) string { return strings.Replace(raw, "Call-ID: ", "Call-ID: retired-", 1) },
			func(raw string) string { return strings.Replace(raw, "tag=reg-notifier", "tag=another-notifier", 1) },
			func(raw string) string {
				return strings.Replace(raw, "\r\nSubscription-State:", ";id=other\r\nSubscription-State:", 1)
			},
		} {
			s := newSubscriptionLifecycleTestService(t, nil)
			result := beginProtocolSubscription(t, s, mwi)
			completeProtocolSubscription(t, s, mwi, result)
			_, _ = s.acceptSubscriptionNotification(notifyForSubscription(result.request, "active;expires=60", 1))
			f := s.subscriptionFieldsLocked(mwi)
			expiresAt := f.lifecycle.expiresAt
			raw := replace(notifyForSubscription(result.request, "terminated;reason=rejected", 2))
			response, err := s.prepareInboundNotification(raw)
			if err != nil || !strings.HasPrefix(response.response, "SIP/2.0 481") || response.afterReply != nil {
				t.Fatalf("unmatched NOTIFY accepted: %v %s", err, response.response)
			}
			if *f.closed || !f.lifecycle.expiresAt.Equal(expiresAt) || s.mwiMessagesWaiting {
				t.Fatal("unmatched NOTIFY changed subscription state")
			}
		}
	}
}

func TestSubscriptionRetiredNotificationBodyIsDiscarded(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		result := beginProtocolSubscription(t, s, mwi)
		raw := notifyForSubscription(result.request, "active;expires=60", 1)
		notification, status := s.acceptSubscriptionNotification(raw)
		if status != 200 {
			t.Fatal(status)
		}
		s.mu.Lock()
		s.subscriptionGeneration++
		s.mu.Unlock()
		s.applySubscriptionNotificationBody(notification)
		if s.mwiMessagesWaiting || s.reginfoAOR != "" || s.notifyReconnectPending.Load() {
			t.Fatal("old notification body changed the new registration")
		}
	}
}

func TestSubscriptionQueuedNotifyDoesNotConfirmUnsentRefresh(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		initial := beginProtocolSubscription(t, s, mwi)
		completeProtocolSubscription(t, s, mwi, initial)
		_, _ = s.acceptSubscriptionNotification(notifyForSubscription(initial.request, "active;expires=120", 1))
		build := s.buildRegistrationSubscription
		if mwi {
			build = s.buildMWISubscription
		}
		request, expires, err := build(time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		refresh := subscriptionResult{context: s.subscriptionContextLocked(), request: request, requestedExpires: expires}
		if err := s.recordSubscriptionUsageAttempt(refresh, mwi); err != nil {
			t.Fatal(err)
		}
		_, status := s.acceptSubscriptionNotification(notifyForSubscription(initial.request, "active;expires=45", 2))
		if status != 200 {
			t.Fatal(status)
		}
		if err := s.subscriptionSent(refresh, mwi); err != nil {
			t.Fatal(err)
		}
		completeProtocolSubscription(t, s, mwi, refresh)
		f := s.subscriptionFieldsLocked(mwi)
		if *f.expires != 120*time.Second || f.lifecycle.notifyDeadline.IsZero() {
			t.Fatal("queued NOTIFY confirmed the not-yet-sent refresh")
		}
	}
}
