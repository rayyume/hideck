package imscore

import (
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

// Deadlines in penalties have already been expired by states(now). Preferences
// rank eligible nodes only; a Retry-After/backoff deadline is never bypassed.
func preferredRegistrarIndex(candidates []string, start int, penalties map[string]registrarPenaltyEntry) (int, bool) {
	if start < 0 || start >= len(candidates) {
		start = 0
	}
	preferred := -1
	for offset := 0; offset < len(candidates); offset++ {
		index := (start + offset) % len(candidates)
		candidate := strings.TrimSpace(candidates[index])
		entry := penalties[candidate]
		if candidate == "" || !entry.retryNotBefore.IsZero() {
			continue
		}
		if entry.deprioritizedUntil.IsZero() {
			return index, true
		}
		if preferred < 0 || entry.deprioritizedUntil.Before(penalties[strings.TrimSpace(candidates[preferred])].deprioritizedUntil) {
			preferred = index
		}
	}
	return preferred, preferred >= 0
}

// Caller holds s.mu. Exclude the current node even if its retry deadline has
// passed, so a failover cannot silently turn into a retry of the same path.
func (s *Service) advanceAvailableRegistrarLocked() string {
	candidates := append([]string(nil), s.registrarCandidates...)
	for index, candidate := range candidates {
		if strings.TrimSpace(candidate) == strings.TrimSpace(s.registrar) {
			candidates[index] = ""
		}
	}
	index, ok := preferredRegistrarIndex(candidates, s.registrarIndex+1, s.registrarPenalties.states(time.Now()))
	if !ok {
		return ""
	}
	s.registrarIndex = index
	s.registrar = strings.TrimSpace(candidates[index])
	return s.registrar
}

func (s *Service) logRegistrarDiscovery(candidates []string, source string) {
	previous := s.registrarPenalties.rememberCandidates(candidates)
	logging.Info("IMS P-CSCF candidates resolved",
		"device", s.DeviceID(), "source", source,
		"previous_candidates", previous, "candidates", candidates)
}

func (s *Service) logRegistrarEligibility(candidates []string, penalties map[string]registrarPenaltyEntry, selected string) {
	type candidateState struct {
		Registrar          string    `json:"registrar"`
		RetryNotBefore     time.Time `json:"retry_not_before,omitempty"`
		DeprioritizedUntil time.Time `json:"deprioritized_until,omitempty"`
		Reason             string    `json:"reason,omitempty"`
		Failures           uint32    `json:"failures"`
	}
	states := make([]candidateState, 0, len(candidates))
	for _, candidate := range candidates {
		entry := penalties[strings.TrimSpace(candidate)]
		states = append(states, candidateState{candidate, entry.retryNotBefore, entry.deprioritizedUntil, entry.reason, entry.consecutiveFailures})
	}
	logging.Info("IMS P-CSCF candidate eligibility",
		"device", s.DeviceID(), "selected", selected, "candidates", states)
}
