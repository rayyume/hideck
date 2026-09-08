package imscore

import (
	"testing"
	"time"
)

type persistedSubscriptionRejection struct {
	status    int
	expiresAt time.Time
}

type memorySubscriptionRejectionStore struct {
	values map[string]persistedSubscriptionRejection
}

func (store *memorySubscriptionRejectionStore) LoadIMSSubscriptionRejection(
	identity, eventPackage string,
	_ time.Time,
) (int, time.Time, error) {
	value := store.values[identity+"\x00"+eventPackage]
	return value.status, value.expiresAt, nil
}

func (store *memorySubscriptionRejectionStore) SaveIMSSubscriptionRejection(
	identity, eventPackage string,
	status int,
	expiresAt time.Time,
) error {
	store.values[identity+"\x00"+eventPackage] = persistedSubscriptionRejection{
		status: status, expiresAt: expiresAt,
	}
	return nil
}

func (store *memorySubscriptionRejectionStore) DeleteIMSSubscriptionRejections(identity string) error {
	delete(store.values, identity+"\x00"+registrationEventPackage)
	delete(store.values, identity+"\x00"+mwiEventPackage)
	return nil
}

func TestMWIRejectionFollowsIdentityNotServiceOrContact(t *testing.T) {
	store := NewSubscriptionRegistrationStore()
	first := store.registered(subscriptionRegistration{identity: "sim-a/user", contact: "old-contact"}, time.Now().Add(time.Hour))
	store.reject(first, mwiEventPackage, 405)
	replacement := store.registered(subscriptionRegistration{identity: first.identity, contact: "new-contact"}, time.Now().Add(time.Hour))
	if got := store.rejected(replacement, mwiEventPackage); got != 405 {
		t.Fatalf("replacement lost MWI rejection: %d", got)
	}
	store.deregistered(first, false)
	if got := store.rejected(replacement, mwiEventPackage); got != 405 {
		t.Fatalf("retiring old contact cleared active identity rejection: %d", got)
	}
	other := store.registered(subscriptionRegistration{identity: "sim-b/user", contact: "other"}, time.Now().Add(time.Hour))
	if store.rejected(other, mwiEventPackage) != 0 {
		t.Fatal("rejection leaked to another identity")
	}
	store.deregistered(replacement, false)
	next := store.registered(subscriptionRegistration{identity: first.identity, contact: "third-contact"}, time.Now().Add(time.Hour))
	store.reject(first, mwiEventPackage, 489)
	if store.rejected(next, mwiEventPackage) != 0 {
		t.Fatal("late rejection crossed deregistration boundary")
	}
}

func TestMWIRejectionExpiresWithLastKnownRegistration(t *testing.T) {
	store := NewSubscriptionRegistrationStore()
	ref := store.registered(subscriptionRegistration{identity: "sim/user", contact: "contact"}, time.Now().Add(time.Hour))
	store.reject(ref, mwiEventPackage, 489)
	store.mu.Lock()
	store.identities[ref.identity].bindings[ref.contact] = time.Now().Add(-time.Second)
	store.mu.Unlock()
	next := store.registered(ref, time.Now().Add(time.Hour))
	if next.epoch == ref.epoch || store.rejected(next, mwiEventPackage) != 0 {
		t.Fatal("expired registration retained rejection")
	}
}

func TestMWITransientFailuresAreNotUnsupported(t *testing.T) {
	store := NewSubscriptionRegistrationStore()
	ref := store.registered(subscriptionRegistration{identity: "sim/user", contact: "contact"}, time.Now().Add(time.Hour))
	for _, status := range []int{403, 408, 500, 503} {
		store.reject(ref, mwiEventPackage, status)
		if store.rejected(ref, mwiEventPackage) != 0 {
			t.Fatalf("status %d became network unsupported", status)
		}
	}
}

func TestExplicitRejectionsSurviveProcessStoreReplacement(t *testing.T) {
	persistence := &memorySubscriptionRejectionStore{
		values: make(map[string]persistedSubscriptionRejection),
	}
	identity := "device\x00impi\x00domain\x00impu"
	expiresAt := time.Now().Add(time.Hour)
	firstStore := NewPersistentSubscriptionRegistrationStore(persistence)
	first := firstStore.registered(subscriptionRegistration{
		identity: identity, contact: "first",
	}, expiresAt)
	firstStore.reject(first, registrationEventPackage, 489)
	firstStore.reject(first, mwiEventPackage, 405)

	restartedStore := NewPersistentSubscriptionRegistrationStore(persistence)
	restarted := restartedStore.registered(subscriptionRegistration{
		identity: identity, contact: "replacement",
	}, expiresAt.Add(time.Hour))
	if got := restartedStore.rejected(restarted, registrationEventPackage); got != 489 {
		t.Fatalf("registration rejection after restart = %d", got)
	}
	if got := restartedStore.rejected(restarted, mwiEventPackage); got != 405 {
		t.Fatalf("MWI rejection after restart = %d", got)
	}
	for _, eventPackage := range []string{registrationEventPackage, mwiEventPackage} {
		value := persistence.values[identity+"\x00"+eventPackage]
		if !value.expiresAt.Equal(expiresAt.Add(time.Hour)) {
			t.Fatalf("%s expiration was not extended: %s", eventPackage, value.expiresAt)
		}
	}
}

func TestExplicitDeregistrationClearsPersistedRejections(t *testing.T) {
	persistence := &memorySubscriptionRejectionStore{
		values: make(map[string]persistedSubscriptionRejection),
	}
	store := NewPersistentSubscriptionRegistrationStore(persistence)
	binding := store.registered(subscriptionRegistration{
		identity: "sim/user", contact: "contact",
	}, time.Now().Add(time.Hour))
	store.reject(binding, mwiEventPackage, 405)
	store.deregistered(binding, false)
	if len(persistence.values) != 0 {
		t.Fatalf("deregistration left persisted rejections: %#v", persistence.values)
	}
}

func TestSubscriptionRefreshUsesNegotiatedLifetime(t *testing.T) {
	for _, test := range []struct{ expires, want time.Duration }{
		{time.Minute, 30 * time.Second},
		{10 * time.Minute, 5 * time.Minute},
		{20 * time.Minute, 10 * time.Minute},
		{time.Hour, 50 * time.Minute},
	} {
		if got := subscriptionRefreshDelay(test.expires); got != test.want {
			t.Fatalf("expires=%s delay=%s want=%s", test.expires, got, test.want)
		}
	}
}
