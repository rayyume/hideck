package imscore

import "time"

// Every non-terminal recovery owner must release through this helper. A stale
// 488/503/RST handler must not consume another path's pending recovery work.
func (s *Service) finishPCSCFRecovery() {
	s.pcscfRecoveryPending.Store(false)
	if s.stopped() {
		return
	}
	s.mu.Lock()
	if watch := s.replacementDownlinkWatch; watch != nil && watch.timer == nil {
		s.armReplacementDownlinkWatchLocked(watch)
	}
	s.mu.Unlock()
	state := s.pendingPortSTimeoutFailover()
	if state.registrar == "" || state.registrar != s.currentPortSRecoveryRegistrar() || s.RegState() != regRegistered {
		return
	}
	retryAt, _ := s.portSRecoveryDeadline(time.Now())
	if retryAt.IsZero() {
		retryAt = time.Now()
	}
	s.schedulePortSReconnectWatchAt(retryAt)
}
