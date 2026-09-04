package imscore

import (
	"strings"
	"sync"
	"time"
)

// RegistrarPenaltyStore retains temporary P-CSCF exclusions across IMS
// service instances that belong to the same runtime reconnect loop.
type RegistrarPenaltyStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

func NewRegistrarPenaltyStore() *RegistrarPenaltyStore {
	return &RegistrarPenaltyStore{entries: make(map[string]time.Time)}
}

func (store *RegistrarPenaltyStore) mark(registrar string, until time.Time) {
	registrar = strings.TrimSpace(registrar)
	if store == nil || registrar == "" {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.entries == nil {
		store.entries = make(map[string]time.Time)
	}
	current, exists := store.entries[registrar]
	if exists && (current.IsZero() || (!until.IsZero() && !until.After(current))) {
		return
	}
	store.entries[registrar] = until
}

func (store *RegistrarPenaltyStore) unavailable(registrar string, now time.Time) bool {
	registrar = strings.TrimSpace(registrar)
	if store == nil || registrar == "" {
		return false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	until, exists := store.entries[registrar]
	if !exists {
		return false
	}
	if until.IsZero() || now.Before(until) {
		return true
	}
	delete(store.entries, registrar)
	return false
}

func (store *RegistrarPenaltyStore) snapshot(now time.Time) map[string]time.Time {
	result := make(map[string]time.Time)
	if store == nil {
		return result
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	for registrar, until := range store.entries {
		if !until.IsZero() && !now.Before(until) {
			delete(store.entries, registrar)
			continue
		}
		result[registrar] = until
	}
	return result
}
