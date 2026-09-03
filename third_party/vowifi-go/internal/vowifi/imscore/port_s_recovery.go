package imscore

import (
	"errors"
	"math/rand/v2"
	"strings"
	"time"
)

const (
	rfc5626RecoveryBaseAllFailed = 30 * time.Second
	rfc5626RecoveryBaseFlowAlive = 90 * time.Second
	rfc5626RecoveryMaxDelay      = 30 * time.Minute
)

type portSRecoveryBackoff struct {
	registrar string
	failures  uint32
	retryAt   time.Time
}

type portSRecoverySchedule struct {
	failures      uint32
	delay         time.Duration
	retryAt       time.Time
	retryAfter    time.Duration
	retryAfterSet bool
}

func (s *Service) recordPortSRecoveryFailure(err error, now time.Time) portSRecoverySchedule {
	registrar := s.currentPortSRecoveryRegistrar()
	retryAfter, retryAfterSet := registerRetryAfterFromError(err)
	s.portSWatchMu.Lock()
	defer s.portSWatchMu.Unlock()
	if s.portSBackoff.registrar != registrar {
		s.portSBackoff = portSRecoveryBackoff{registrar: registrar}
	}
	s.portSBackoff.failures++
	// The existing registration flow is still alive, so RFC 5626 section 4.5
	// uses the 90-second base. Retry-After may extend, but not shorten, it.
	upper := rfc5626RecoveryUpperBound(s.portSBackoff.failures, false)
	delay := s.jitterPortSRecoveryDelay(upper)
	if retryAfterSet && retryAfter > delay {
		delay = retryAfter
	}
	s.portSBackoff.retryAt = now.Add(delay)
	return portSRecoverySchedule{
		failures: s.portSBackoff.failures, delay: delay, retryAt: s.portSBackoff.retryAt,
		retryAfter: retryAfter, retryAfterSet: retryAfterSet,
	}
}

func (s *Service) portSRecoveryDeadline(now time.Time) (time.Time, bool) {
	registrar := s.currentPortSRecoveryRegistrar()
	s.portSWatchMu.Lock()
	defer s.portSWatchMu.Unlock()
	if s.portSBackoff.registrar != registrar {
		s.portSWatchGeneration++
		if s.portSWatchTimer != nil {
			s.portSWatchTimer.Stop()
			s.portSWatchTimer = nil
		}
		s.portSBackoff = portSRecoveryBackoff{}
		return time.Time{}, false
	}
	return s.portSBackoff.retryAt, s.portSBackoff.retryAt.After(now)
}

func (s *Service) resetPortSRecoveryBackoff() {
	if s == nil {
		return
	}
	s.portSWatchMu.Lock()
	defer s.portSWatchMu.Unlock()
	s.portSWatchGeneration++
	if s.portSWatchTimer != nil {
		s.portSWatchTimer.Stop()
		s.portSWatchTimer = nil
	}
	s.portSBackoff = portSRecoveryBackoff{}
}

func (s *Service) currentPortSRecoveryRegistrar() string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return strings.TrimSpace(s.registrar)
}

func (s *Service) jitterPortSRecoveryDelay(upper time.Duration) time.Duration {
	jitter := randomRFC5626RecoveryDelay
	if s != nil && s.portSRecoveryJitter != nil {
		jitter = s.portSRecoveryJitter
	}
	return clampRFC5626RecoveryDelay(jitter(upper), upper)
}

func rfc5626RecoveryUpperBound(failures uint32, allFlowsFailed bool) time.Duration {
	base := rfc5626RecoveryBaseFlowAlive
	if allFlowsFailed {
		base = rfc5626RecoveryBaseAllFailed
	}
	upper := base
	for attempt := uint32(0); attempt < failures; attempt++ {
		if upper >= rfc5626RecoveryMaxDelay/2 {
			return rfc5626RecoveryMaxDelay
		}
		upper *= 2
	}
	return upper
}

func randomRFC5626RecoveryDelay(upper time.Duration) time.Duration {
	lower := upper / 2
	span := upper - lower
	if span <= 0 {
		return upper
	}
	return lower + time.Duration(rand.Int64N(int64(span)+1))
}

func clampRFC5626RecoveryDelay(delay, upper time.Duration) time.Duration {
	lower := upper / 2
	if delay < lower {
		return lower
	}
	if delay > upper {
		return upper
	}
	return delay
}

func registerRetryAfterFromError(err error) (time.Duration, bool) {
	var responseErr *registerResponseError
	if !errors.As(err, &responseErr) {
		return 0, false
	}
	return responseErr.retryAfter, responseErr.retryAfterSet
}
