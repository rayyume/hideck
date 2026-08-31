package runtimecore

import (
	"strings"
	"sync"
)

// FastReauthStore keeps EAP-AKA' fast reauthentication material across a new
// IKE SA. RFC 7296 2.8.3 requires a fresh IKE_SA_INIT/IKE_AUTH; the identity
// is reused there, not injected onto the old SA.
type FastReauthStore struct {
	mu    sync.Mutex
	id    string
	mk    []byte
	kAut  []byte
	kEncr []byte
}

func (store *FastReauthStore) Capture() func(string, []byte, []byte, []byte) {
	return func(id string, mk, kAut, kEncr []byte) {
		if store == nil {
			return
		}
		store.mu.Lock()
		defer store.mu.Unlock()
		store.id = strings.TrimSpace(id)
		store.mk = append([]byte(nil), mk...)
		store.kAut = append([]byte(nil), kAut...)
		store.kEncr = append([]byte(nil), kEncr...)
	}
}

func (store *FastReauthStore) Apply(cfg *SessionConfig) {
	if store == nil || cfg == nil {
		return
	}
	store.mu.Lock()
	id := store.id
	mk := append([]byte(nil), store.mk...)
	kAut := append([]byte(nil), store.kAut...)
	kEncr := append([]byte(nil), store.kEncr...)
	store.mu.Unlock()
	if id != "" {
		cfg.FastReauthID = id
		cfg.FastReauthMK = mk
		cfg.FastReauthKAut = kAut
		cfg.FastReauthKEncr = kEncr
	}
	capture := store.Capture()
	if cfg.OnFastReauthUpdate == nil {
		cfg.OnFastReauthUpdate = capture
		return
	}
	previous := cfg.OnFastReauthUpdate
	cfg.OnFastReauthUpdate = func(nextID string, nextMK, nextKAut, nextKEncr []byte) {
		capture(nextID, nextMK, nextKAut, nextKEncr)
		previous(nextID, nextMK, nextKAut, nextKEncr)
	}
}
