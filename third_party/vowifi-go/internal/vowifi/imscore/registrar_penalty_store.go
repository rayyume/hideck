package imscore

import (
	"math"
	"strings"
	"sync"
	"time"
)

// RegistrarPenaltyStore retains temporary P-CSCF exclusions and consecutive
// registration failures across service instances in one runtime reconnect loop.
type RegistrarPenaltyStore struct {
	mu      sync.Mutex
	entries map[string]registrarPenaltyEntry
}

type registrarPenaltyEntry struct {
	unavailableUntil    time.Time
	consecutiveFailures uint32
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
	if !entry.unavailableUntil.IsZero() && !until.After(entry.unavailableUntil) {
		return
	}
	entry.unavailableUntil = until
	store.entries[registrar] = entry
}

func (store *RegistrarPenaltyStore) recordFailure(registrar string) uint32 {
	registrar = strings.TrimSpace(registrar)
	if store == nil || registrar == "" {
		return 1
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.entries == nil {
		store.entries = make(map[string]registrarPenaltyEntry)
	}
	entry := store.entries[registrar]
	if entry.consecutiveFailures < math.MaxUint32 {
		entry.consecutiveFailures++
	}
	store.entries[registrar] = entry
	return entry.consecutiveFailures
}

func (store *RegistrarPenaltyStore) clearFailures(registrar string) {
	registrar = strings.TrimSpace(registrar)
	if store == nil || registrar == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, exists := store.entries[registrar]
	if !exists {
		return
	}
	entry.consecutiveFailures = 0
	if entry.unavailableUntil.IsZero() {
		delete(store.entries, registrar)
	} else {
		store.entries[registrar] = entry
	}
}

func (store *RegistrarPenaltyStore) unavailable(registrar string, now time.Time) bool {
	registrar = strings.TrimSpace(registrar)
	if store == nil || registrar == "" {
		return false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, exists := store.entries[registrar]
	if !exists {
		return false
	}
	if !entry.unavailableUntil.IsZero() && now.Before(entry.unavailableUntil) {
		return true
	}
	entry.unavailableUntil = time.Time{}
	if entry.consecutiveFailures == 0 {
		delete(store.entries, registrar)
	} else {
		store.entries[registrar] = entry
	}
	return false
}

func (store *RegistrarPenaltyStore) snapshot(now time.Time) map[string]time.Time {
	result := make(map[string]time.Time)
	if store == nil {
		return result
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for registrar, entry := range store.entries {
		if entry.unavailableUntil.IsZero() || !now.Before(entry.unavailableUntil) {
			entry.unavailableUntil = time.Time{}
			if entry.consecutiveFailures == 0 {
				delete(store.entries, registrar)
			} else {
				store.entries[registrar] = entry
			}
			continue
		}
		result[registrar] = entry.unavailableUntil
	}
	return result
}
