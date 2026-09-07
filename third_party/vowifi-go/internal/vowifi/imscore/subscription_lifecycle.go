package imscore

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
)

var errSubscriptionContextChanged = errors.New("imscore: subscription result belongs to a retired registration context")

type subscriptionContext struct {
	binding    subscriptionRegistration
	route      string
	generation uint64
}

type subscriptionLifecycle struct {
	context        subscriptionContext
	started        bool
	rejectedStatus int
	blockedReason  string
	attemptKey     sipTransactionKey
	sentAt         time.Time
	notifyDeadline time.Time
	expiresAt      time.Time
	retryAt        time.Time
	notifyExpires  bool
	initial        bool
	unsubscribing  bool
	notifyVersion  uint64
	notifications  *subscriptionNotificationQueue
}

type subscriptionResult struct {
	context          subscriptionContext
	request          *sip.Request
	response         *sip.Response
	requestedExpires time.Duration
	unsubscribe      bool
	err              error
}

func (s *Service) subscriptionContextLocked() subscriptionContext {
	return subscriptionContext{
		binding:    s.subscriptionBinding,
		route:      s.serviceRoute,
		generation: s.subscriptionGeneration,
	}
}

func (s *Service) trackSubscriptionRegistrationLocked(expires time.Duration) {
	identity := firstNonBlank(s.regSession.publicID, s.reginfoAOR, primaryPublicIdentity(s.cfg))
	route := s.registeredSIPRouteLocked()
	contact, _ := registeredVoiceContact(s.cfg, firstNonBlank(s.regSession.contactUser, contactUser(s.cfg)), route.serverAddress)
	binding := subscriptionRegistration{
		identity: strings.Join([]string{s.DeviceID(), s.cfg.IMPI, s.cfg.Domain, identity}, "\x00"),
		contact:  contact,
	}
	s.subscriptionBinding = s.subscriptionRegistrations.registered(binding, time.Now().Add(expires))
}

// A normal REGISTER refresh preserves dialogs and negative subscription results.
// MWI 405/489 is scoped to identity deregistration (TS 24.606 4.7.2.1), not a flow.
func (s *Service) prepareSubscriptionStart(mwi bool) (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if eligible, reason := s.subscriptionGateLocked(); !eligible {
		return false, reason
	}
	lifecycle := s.alignSubscriptionContextLocked(mwi)
	current := lifecycle.context
	if mwi {
		if status := s.subscriptionRegistrations.mwiRejected(current.binding); status != 0 {
			s.mwiSubscriptionClosed = true
			s.mwiSubscriptionRefreshAt = time.Time{}
			s.mwiSubscriptionLastErr = fmt.Sprintf("MWI rejected with status %d; waiting for IMS identity deregistration", status)
			return false, s.mwiSubscriptionLastErr
		}
	}
	if lifecycle.rejectedStatus != 0 {
		return false, fmt.Sprintf("subscription previously rejected with status %d", lifecycle.rejectedStatus)
	}
	if lifecycle.blockedReason != "" {
		return false, lifecycle.blockedReason
	}
	if lifecycle.unsubscribing {
		return false, "subscription is being removed"
	}
	if !lifecycle.retryAt.IsZero() && time.Now().Before(lifecycle.retryAt) {
		return false, "subscription retry is not due"
	}
	closed := s.subscriptionClosed
	if mwi {
		closed = s.mwiSubscriptionClosed
	}
	if lifecycle.started && !closed && s.subscriptionAttemptPendingLocked(mwi) {
		return false, "subscription lifecycle already started"
	}
	if closed {
		if mwi {
			s.resetMWISubscriptionLocked()
		} else {
			s.resetRegistrationSubscriptionLocked()
		}
	}
	lifecycle.started = true
	return true, ""
}

func (s *Service) alignSubscriptionContextLocked(mwi bool) *subscriptionLifecycle {
	current := s.subscriptionContextLocked()
	lifecycle := &s.subscriptionLifecycle
	if mwi {
		lifecycle = &s.mwiSubscriptionLifecycle
	}
	if lifecycle.context != current {
		if mwi {
			s.resetMWISubscriptionLocked()
		} else {
			s.resetRegistrationSubscriptionLocked()
		}
		*lifecycle = subscriptionLifecycle{context: current}
	}
	return lifecycle
}

func (s *Service) subscriptionAttemptPendingLocked(mwi bool) bool {
	if mwi {
		return s.mwiSubscriptionInFlight.Load() || !s.mwiSubscriptionRefreshAt.IsZero() || s.mwiSubscriptionDialog.ready()
	}
	return s.subscriptionInFlight.Load() || !s.subscriptionRefreshAt.IsZero() || s.subscriptionDialog.ready()
}

func (s *Service) beginSubscriptionAttempt(mwi, unsubscribe bool) (subscriptionContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lifecycle := s.alignSubscriptionContextLocked(mwi)
	current := lifecycle.context
	status := lifecycle.rejectedStatus
	if mwi {
		if rejected := s.subscriptionRegistrations.mwiRejected(current.binding); rejected != 0 {
			status = rejected
		}
	}
	if !unsubscribe && status != 0 {
		return current, fmt.Errorf("imscore: subscription previously rejected with status %d", status)
	}
	if !unsubscribe && lifecycle.blockedReason != "" {
		return current, errors.New(lifecycle.blockedReason)
	}
	if !unsubscribe && lifecycle.unsubscribing {
		return current, errors.New("imscore: subscription is being removed")
	}
	if !unsubscribe && !lifecycle.retryAt.IsZero() && time.Now().Before(lifecycle.retryAt) {
		return current, errors.New("imscore: subscription retry is not due")
	}
	lifecycle.context, lifecycle.started = current, true
	return current, nil
}

func (s *Service) subscriptionResultCurrentLocked(result subscriptionResult) bool {
	// An in-place REGISTER refresh does not retire the existing subscription.
	registered := s.regState == regRegistered || s.regState == regRegistering
	return !s.stopped() && registered && result.context == s.subscriptionContextLocked()
}

func (s *Service) retrySubscriptionAfter481(result subscriptionResult, mwi bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.subscriptionResultCurrentLocked(result) {
		return false
	}
	dialog := &s.subscriptionDialog
	if mwi {
		dialog = &s.mwiSubscriptionDialog
	}
	if !dialog.ready() {
		return false
	}
	*dialog = registrationSubscriptionDialog{}
	fields := s.subscriptionFieldsLocked(mwi)
	fields.lifecycle.expiresAt = time.Time{}
	fields.lifecycle.notifyDeadline = time.Time{}
	*fields.expires = 0
	*fields.refreshAt = time.Time{}
	return true
}

func (s *Service) endSubscriptionRegistrationLocked(all bool) {
	s.subscriptionRegistrations.deregistered(s.subscriptionBinding, all)
	s.subscriptionGeneration++
	s.subscriptionLifecycle = subscriptionLifecycle{}
	s.mwiSubscriptionLifecycle = subscriptionLifecycle{}
}
