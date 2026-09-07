package imscore

// Queue state is protected by Service.mu. A single drainer applies accepted
// bodies in receive order, even when their reply callbacks run out of order.
// Reginfo partial updates are deltas, not superseding snapshots (RFC 3680 5.2).
type subscriptionNotificationQueue struct {
	pending  []*subscriptionNotification
	draining bool
}

func (s *Service) applySubscriptionNotificationBody(n *subscriptionNotification) {
	if n == nil || n.queue == nil {
		return
	}
	s.mu.Lock()
	if n.replyDone {
		s.mu.Unlock()
		return
	}
	n.replyDone = true
	q := n.queue
	if q.draining {
		s.mu.Unlock()
		return
	}
	q.draining = true
	s.mu.Unlock()
	for n := s.nextSubscriptionNotification(q); n != nil; n = s.nextSubscriptionNotification(q) {
		if n.mwi {
			s.applyMWINotification(n)
		} else {
			s.applyRegistrationNotification(n)
		}
	}
}

func (s *Service) nextSubscriptionNotification(q *subscriptionNotificationQueue) *subscriptionNotification {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(q.pending) == 0 || !q.pending[0].replyDone {
		q.draining = false
		return nil
	}
	n := q.pending[0]
	q.pending[0] = nil
	q.pending = q.pending[1:]
	if len(q.pending) == 0 {
		q.pending = nil
	}
	return n
}
