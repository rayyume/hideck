package imscore

import (
	"errors"
	"fmt"
	"time"
)

var errSubscriptionUsageChanged = errors.New("imscore: subscription attempt belongs to a retired or blocked usage")

func (s *Service) subscriptionAttemptContextLocked(mwi bool) subscriptionAttemptContext {
	return subscriptionAttemptContext{
		subscriptionContext: s.subscriptionContextLocked(),
		usageGeneration:     s.subscriptionFieldsLocked(mwi).lifecycle.usageGeneration,
	}
}

func (s *Service) beginSubscriptionAttempt(mwi, unsubscribe bool) (subscriptionAttemptContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lifecycle := s.alignSubscriptionContextLocked(mwi)
	s.expireSubscriptionTimersLocked(time.Now())
	current := s.subscriptionAttemptContextLocked(mwi)
	if err := s.subscriptionAttemptBlockedLocked(mwi, unsubscribe); err != nil {
		return current, err
	}
	lifecycle.started = true
	return current, nil
}

func (s *Service) subscriptionAttemptBlockedLocked(mwi, unsubscribe bool) error {
	if unsubscribe {
		return nil
	}
	lifecycle := s.subscriptionFieldsLocked(mwi).lifecycle
	status := lifecycle.rejectedStatus
	if rejected := s.subscriptionRegistrations.rejected(
		s.subscriptionBinding,
		subscriptionEventPackage(mwi),
	); rejected != 0 {
		status = rejected
	}
	if status != 0 {
		return fmt.Errorf("imscore: subscription previously rejected with status %d", status)
	}
	if lifecycle.blockedReason != "" {
		return errors.New(lifecycle.blockedReason)
	}
	if lifecycle.unsubscribing {
		return errors.New("imscore: subscription is being removed")
	}
	if !lifecycle.retryAt.IsZero() && time.Now().Before(lifecycle.retryAt) {
		return errors.New("imscore: subscription retry is not due")
	}
	return nil
}

func (s *Service) validateSubscriptionAttemptLocked(result subscriptionResult, mwi bool) error {
	if !s.subscriptionResultCurrentLocked(result) {
		return errSubscriptionContextChanged
	}
	if result.context.usageGeneration != s.subscriptionFieldsLocked(mwi).lifecycle.usageGeneration {
		return errSubscriptionUsageChanged
	}
	if err := s.subscriptionAttemptBlockedLocked(mwi, result.unsubscribe); err != nil {
		return errors.Join(errSubscriptionUsageChanged, err)
	}
	return nil
}
