package imscore

import "time"

// A locked view of the two independent subscription usages. Keep the existing
// diagnostic fields, but share lifecycle transitions rather than duplicating them.
type subscriptionFields struct {
	lifecycle     *subscriptionLifecycle
	dialog        *registrationSubscriptionDialog
	closed        *bool
	expires       *time.Duration
	refreshAt     *time.Time
	lastAttemptAt *time.Time
	lastOKAt      *time.Time
	lastErr       *string
	mwi           bool
}

func (s *Service) subscriptionFieldsLocked(mwi bool) subscriptionFields {
	if mwi {
		return subscriptionFields{
			lifecycle: &s.mwiSubscriptionLifecycle, dialog: &s.mwiSubscriptionDialog,
			closed: &s.mwiSubscriptionClosed, expires: &s.mwiSubscriptionExpires,
			refreshAt: &s.mwiSubscriptionRefreshAt, lastAttemptAt: &s.mwiSubscriptionLastAttemptAt,
			lastOKAt: &s.mwiSubscriptionLastOKAt, lastErr: &s.mwiSubscriptionLastErr, mwi: true,
		}
	}
	return subscriptionFields{
		lifecycle: &s.subscriptionLifecycle, dialog: &s.subscriptionDialog,
		closed: &s.subscriptionClosed, expires: &s.subscriptionExpires,
		refreshAt: &s.subscriptionRefreshAt, lastAttemptAt: &s.subscriptionLastAttemptAt,
		lastOKAt: &s.subscriptionLastOKAt, lastErr: &s.subscriptionLastErr,
	}
}

func (f subscriptionFields) refreshDelay(expires time.Duration) time.Duration {
	if f.mwi {
		return mwiSubscriptionRefreshDelay(expires)
	}
	return subscriptionRefreshDelay(expires)
}

func (f subscriptionFields) setExpiry(now time.Time, expires time.Duration) {
	*f.expires = expires
	f.lifecycle.expiresAt = now.Add(expires)
	*f.refreshAt = now.Add(f.refreshDelay(expires))
}

func (f subscriptionFields) terminate(reason string) {
	// Invalidate requests built before termination without retiring already
	// accepted NOTIFY bodies belonging to the same IMS registration.
	f.lifecycle.usageGeneration++
	*f.closed = true
	*f.dialog = registrationSubscriptionDialog{}
	*f.expires = 0
	*f.refreshAt = time.Time{}
	*f.lastErr = reason
	f.lifecycle.expiresAt = time.Time{}
	f.lifecycle.notifyDeadline = time.Time{}
	f.lifecycle.retryAt = time.Time{}
}

func (f subscriptionFields) scheduleRetry(at time.Time) {
	f.lifecycle.retryAt = at
	*f.refreshAt = at
}

func (f subscriptionFields) retryBlocked() bool {
	return f.lifecycle.rejectedStatus != 0 || f.lifecycle.blockedReason != "" || f.lifecycle.unsubscribing
}
