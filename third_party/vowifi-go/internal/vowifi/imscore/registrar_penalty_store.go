package imscore

import (
	"strings"
	"sync"
	"time"
)

// RegistrarPenaltyStore retains retry deadlines, selection preferences and
// consecutive failures across service instances in one runtime reconnect loop.
type RegistrarPenaltyStore struct {
	mu             sync.Mutex
	entries        map[string]registrarPenaltyEntry
	lastCandidates []string
	recovering     bool
	generation     uint64
}

// A successful binding can confirm only failures observed before its attempt.
type registrarRecoveryAttempt struct {
	registrar  string
	generation uint64
}

func (store *RegistrarPenaltyStore) recoveryGeneration() uint64 {
	if store == nil {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.generation
}

func (store *RegistrarPenaltyStore) rememberCandidates(candidates []string) []string {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	previous := append([]string(nil), store.lastCandidates...)
	store.lastCandidates = append([]string(nil), candidates...)
	return previous
}

type registrarPenaltyEntry struct {
	retryNotBefore      time.Time
	deprioritizedUntil  time.Time
	reason              string
	consecutiveFailures uint32
	failureGeneration   uint64
}

func NewRegistrarPenaltyStore() *RegistrarPenaltyStore {
	return &RegistrarPenaltyStore{entries: make(map[string]registrarPenaltyEntry)}
}

func (store *RegistrarPenaltyStore) mark(registrar string, until time.Time) {
	registrar = strings.TrimSpace(registrar)
	if store == nil || registrar == "" || until.IsZero() {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.entries == nil {
		store.entries = make(map[string]registrarPenaltyEntry)
	}
	entry := store.entries[registrar]
	if !until.After(entry.retryNotBefore) {
		return
	}
	entry.retryNotBefore = until
	store.entries[registrar] = entry
}

func (store *RegistrarPenaltyStore) clearFailures(attempt registrarRecoveryAttempt) {
	registrar := strings.TrimSpace(attempt.registrar)
	if store == nil || registrar == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, exists := store.entries[registrar]
	if entry.failureGeneration > attempt.generation {
		return
	}
	store.recovering = false
	if !exists {
		return
	}
	entry.consecutiveFailures = 0
	if entry.retryNotBefore.IsZero() && entry.deprioritizedUntil.IsZero() {
		delete(store.entries, registrar)
	} else {
		store.entries[registrar] = entry
	}
}

func (store *RegistrarPenaltyStore) recoveryInProgress() bool {
	if store == nil {
		return false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.recovering
}

func (store *RegistrarPenaltyStore) snapshot(now time.Time) map[string]time.Time {
	result := make(map[string]time.Time)
	for registrar, entry := range store.states(now) {
		until := entry.retryNotBefore
		if entry.deprioritizedUntil.After(until) {
			until = entry.deprioritizedUntil
		}
		if !until.IsZero() {
			result[registrar] = until
		}
	}
	return result
}

func (store *RegistrarPenaltyStore) states(now time.Time) map[string]registrarPenaltyEntry {
	result := make(map[string]registrarPenaltyEntry)
	if store == nil {
		return result
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for registrar, entry := range store.entries {
		if !now.Before(entry.retryNotBefore) {
			entry.retryNotBefore = time.Time{}
		}
		if !now.Before(entry.deprioritizedUntil) {
			entry.deprioritizedUntil = time.Time{}
		}
		if entry.retryNotBefore.IsZero() && entry.deprioritizedUntil.IsZero() && entry.consecutiveFailures == 0 {
			delete(store.entries, registrar)
			continue
		}
		store.entries[registrar] = entry
		result[registrar] = entry
	}
	return result
}
