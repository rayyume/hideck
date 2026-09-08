package imscore

import (
	"strings"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

func (s *Service) markSelfRoutedSubscription(request *sip.Request) {
	if s == nil || request == nil || request.Method != sip.SUBSCRIBE || request.CallID() == nil || request.CSeq() == nil {
		return
	}
	eventPackage := subscriptionRequestEventPackage(request)
	if eventPackage != registrationEventPackage && eventPackage != mwiEventPackage {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lifecycle := &s.subscriptionLifecycle
	if eventPackage == mwiEventPackage {
		lifecycle = &s.mwiSubscriptionLifecycle
	}
	key := lifecycle.attemptKey
	if key.Method != string(sip.SUBSCRIBE) || key.CSeq != int(request.CSeq().SeqNo) {
		return
	}
	if key.CallID != strings.TrimSpace(request.CallID().Value()) {
		return
	}
	lifecycle.selfRouted = true
}

func subscriptionRequestEventPackage(request *sip.Request) string {
	value := sipkit.FirstHeaderValue(request, "Event", true)
	eventPackage, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(value)), ";")
	return strings.TrimSpace(eventPackage)
}
