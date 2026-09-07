package imscore

import (
	"testing"
	"time"
)

func TestMWIRejectionFollowsIdentityNotServiceOrContact(t *testing.T) {
	store := NewSubscriptionRegistrationStore()
	first := store.registered(subscriptionRegistration{identity: "sim-a/user", contact: "old-contact"}, time.Now().Add(time.Hour))
	store.rejectMWI(first, 405)
	replacement := store.registered(subscriptionRegistration{identity: first.identity, contact: "new-contact"}, time.Now().Add(time.Hour))
	if got := store.mwiRejected(replacement); got != 405 {
		t.Fatalf("replacement lost MWI rejection: %d", got)
	}
	store.deregistered(first, false)
	if got := store.mwiRejected(replacement); got != 405 {
		t.Fatalf("retiring old contact cleared active identity rejection: %d", got)
	}
	other := store.registered(subscriptionRegistration{identity: "sim-b/user", contact: "other"}, time.Now().Add(time.Hour))
	if store.mwiRejected(other) != 0 {
		t.Fatal("rejection leaked to another identity")
	}
	store.deregistered(replacement, false)
	next := store.registered(subscriptionRegistration{identity: first.identity, contact: "third-contact"}, time.Now().Add(time.Hour))
	store.rejectMWI(first, 489)
	if store.mwiRejected(next) != 0 {
		t.Fatal("late rejection crossed deregistration boundary")
	}
}

func TestMWIRejectionExpiresWithLastKnownRegistration(t *testing.T) {
	store := NewSubscriptionRegistrationStore()
	ref := store.registered(subscriptionRegistration{identity: "sim/user", contact: "contact"}, time.Now().Add(time.Hour))
	store.rejectMWI(ref, 489)
	store.mu.Lock()
	store.identities[ref.identity].bindings[ref.contact] = time.Now().Add(-time.Second)
	store.mu.Unlock()
	next := store.registered(ref, time.Now().Add(time.Hour))
	if next.epoch == ref.epoch || store.mwiRejected(next) != 0 {
		t.Fatal("expired registration retained rejection")
	}
}

func TestMWITransientFailuresAreNotUnsupported(t *testing.T) {
	store := NewSubscriptionRegistrationStore()
	ref := store.registered(subscriptionRegistration{identity: "sim/user", contact: "contact"}, time.Now().Add(time.Hour))
	for _, status := range []int{403, 408, 500, 503} {
		store.rejectMWI(ref, status)
		if store.mwiRejected(ref) != 0 {
			t.Fatalf("status %d became network unsupported", status)
		}
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
