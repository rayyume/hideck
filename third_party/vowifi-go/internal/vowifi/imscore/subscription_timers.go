package imscore

import (
	"fmt"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

func (s *Service) subscriptionDueLocked(mwi bool, now time.Time) bool {
	f := s.subscriptionFieldsLocked(mwi)
	eligible, _ := s.subscriptionGateLocked()
	if !eligible || f.retryBlocked() || !f.lifecycle.notifyDeadline.IsZero() {
		return false
	}
	if s.subscriptionRegistrations.rejected(s.subscriptionBinding, subscriptionEventPackage(mwi)) != 0 {
		return false
	}
	if (mwi && s.mwiSubscriptionInFlight.Load()) || (!mwi && s.subscriptionInFlight.Load()) {
		return false
	}
	if !f.lifecycle.retryAt.IsZero() {
		return !now.Before(f.lifecycle.retryAt)
	}
	if *f.closed {
		// Identity-scoped 405/489 responses close the usage while their
		// registration-lifetime cache is active. Once that cache expires,
		// retry without requiring a process or IMS service restart.
		return f.lifecycle.started && f.lifecycle.rejectedStatus == 0
	}
	return (!mwi && f.refreshAt.IsZero()) || (!f.refreshAt.IsZero() && !now.Before(*f.refreshAt))
}

func (s *Service) nextSubscriptionWakeLocked(now time.Time) time.Time {
	next := now.Add(imsMaintenancePollInterval)
	eligible, _ := s.subscriptionGateLocked()
	for _, mwi := range []bool{false, true} {
		f := s.subscriptionFieldsLocked(mwi)
		if f.lifecycle.started && f.lifecycle.context != s.subscriptionContextLocked() {
			continue
		}
		deadlines := []time.Time{f.lifecycle.notifyDeadline}
		if f.lifecycle.notifyDeadline.IsZero() {
			deadlines = append(deadlines, f.lifecycle.expiresAt)
		}
		inFlight := s.subscriptionInFlight.Load()
		if mwi {
			inFlight = s.mwiSubscriptionInFlight.Load()
		}
		if eligible && !inFlight && !f.retryBlocked() && f.lifecycle.notifyDeadline.IsZero() {
			at := *f.refreshAt
			if !f.lifecycle.retryAt.IsZero() {
				at = f.lifecycle.retryAt
			}
			deadlines = append(deadlines, at)
		}
		for _, at := range deadlines {
			if !at.IsZero() && at.Before(next) {
				next = at
			}
		}
	}
	return next
}

func (s *Service) expireSubscriptionTimersLocked(now time.Time) {
	for _, mwi := range []bool{false, true} {
		f := s.subscriptionFieldsLocked(mwi)
		if f.lifecycle.context != s.subscriptionContextLocked() {
			continue
		}
		deadline := f.lifecycle.notifyDeadline
		if !deadline.IsZero() && !now.Before(deadline) {
			retry := f.lifecycle.retryAt
			f.terminate("SUBSCRIBE Timer N expired without a matching NOTIFY")
			if !f.retryBlocked() {
				f.scheduleRetry(laterSubscriptionRetry(now.Add(f.refreshDelay(registerExpires(s.cfg))), retry))
			}
			logging.WarnRate(fmt.Sprintf("ims-subscription-notify-timeout-%s-%t", s.DeviceID(), mwi),
				"IMS subscription NOTIFY timed out; keeping current registration", "device", s.DeviceID(), "mwi", mwi)
			continue
		}
		if deadline.IsZero() && !f.lifecycle.expiresAt.IsZero() && !now.Before(f.lifecycle.expiresAt) {
			retry := f.lifecycle.retryAt
			f.terminate("subscription negotiated lifetime expired")
			if !f.retryBlocked() {
				f.scheduleRetry(laterSubscriptionRetry(now, retry))
			}
		}
	}
}

func laterSubscriptionRetry(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}

func (s *Service) terminateNotifiedSubscriptionLocked(f subscriptionFields, state subscriptionNotifyState, now time.Time) {
	f.terminate("subscription terminated: " + state.reason)
	if f.retryBlocked() {
		return
	}
	switch state.reason {
	case "rejected", "noresource", "invariant":
		f.lifecycle.blockedReason = *f.lastErr
	case "deactivated", "timeout":
		f.scheduleRetry(now)
	default:
		// Keep the existing package-specific retry pacing when the notifier
		// supplies no delay. This is local scheduling, not IMS flow recovery.
		delay := f.refreshDelay(registerExpires(s.cfg))
		if state.retryAfterPresent {
			delay = state.retryAfter
		}
		if state.reason == "probation" && delay <= 0 {
			delay = f.refreshDelay(registerExpires(s.cfg))
		}
		f.scheduleRetry(now.Add(delay))
	}
}
