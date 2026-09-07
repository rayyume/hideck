package imscore

import (
	"sync"
	"time"
)

// SubscriptionRegistrationStore follows IMS identity registration lifetimes,
// not TCP or Service lifetimes. It is shared across runtime recovery attempts.
type SubscriptionRegistrationStore struct {
	mu         sync.Mutex
	nextEpoch  uint64
	identities map[string]*subscriptionRegistrationEntry
}

type subscriptionRegistrationEntry struct {
	epoch        uint64
	bindings     map[string]time.Time
	mwiRejection int
}

type subscriptionRegistration struct {
	identity string
	contact  string
	epoch    uint64
}

func NewSubscriptionRegistrationStore() *SubscriptionRegistrationStore {
	return &SubscriptionRegistrationStore{identities: make(map[string]*subscriptionRegistrationEntry)}
}

func (store *SubscriptionRegistrationStore) registered(binding subscriptionRegistration, expiresAt time.Time) subscriptionRegistration {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.identities == nil {
		store.identities = make(map[string]*subscriptionRegistrationEntry)
	}
	entry := store.activeLocked(binding.identity, time.Now())
	if entry == nil {
		store.nextEpoch++
		entry = &subscriptionRegistrationEntry{epoch: store.nextEpoch, bindings: make(map[string]time.Time)}
		store.identities[binding.identity] = entry
	}
	entry.bindings[binding.contact] = expiresAt
	binding.epoch = entry.epoch
	return binding
}

func (store *SubscriptionRegistrationStore) activeLocked(identity string, now time.Time) *subscriptionRegistrationEntry {
	entry := store.identities[identity]
	if entry == nil {
		return nil
	}
	for contact, expiresAt := range entry.bindings {
		if !now.Before(expiresAt) {
			delete(entry.bindings, contact)
		}
	}
	if len(entry.bindings) == 0 {
		delete(store.identities, identity)
		return nil
	}
	return entry
}

func (store *SubscriptionRegistrationStore) rejectMWI(binding subscriptionRegistration, status int) {
	if status != 405 && status != 489 {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.activeLocked(binding.identity, time.Now())
	if entry != nil && entry.epoch == binding.epoch {
		entry.mwiRejection = status
	}
}

func (store *SubscriptionRegistrationStore) mwiRejected(binding subscriptionRegistration) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.activeLocked(binding.identity, time.Now())
	if entry == nil || entry.epoch != binding.epoch {
		return 0
	}
	return entry.mwiRejection
}

func (store *SubscriptionRegistrationStore) deregistered(binding subscriptionRegistration, all bool) {
	store.mu.Lock()
	defer store.mu.Unlock()
	entry := store.identities[binding.identity]
	if entry == nil || entry.epoch != binding.epoch {
		return
	}
	delete(entry.bindings, binding.contact)
	if all || len(entry.bindings) == 0 {
		delete(store.identities, binding.identity)
	}
}
