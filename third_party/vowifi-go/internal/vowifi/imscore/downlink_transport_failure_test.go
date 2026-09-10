package imscore

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestDeadRegisteredFlowIsNotBlockedByPassiveDownlinkWait(t *testing.T) {
	s := singleCandidateReplacement(t)
	// Preserve a real server deadline independently of passive observations.
	hardDeadline := time.Now().Add(time.Hour)
	s.registrarPenalties.mark("other.example:5060", hardDeadline)
	s.clearClosedRegistrationTCP(s.registrationTCP, io.EOF)
	if s.replacementDownlinkWatch != nil || s.RegState() != regFailed {
		t.Fatal("dead current flow kept the passive watch")
	}
	replacement := replacementUsingStore(t, s.registrarPenalties)
	replacement.cfg.Registrar = s.cfg.Registrar
	if _, err := replacement.selectRegistrarCandidate(context.Background(), "tcp"); err != nil {
		t.Fatalf("dead flow recovery is waiting on passive validation: %v", err)
	}
	entry := s.registrarPenalties.states(time.Now())["other.example:5060"]
	if !entry.retryNotBefore.Equal(hardDeadline) {
		t.Fatal("clearing passive wait erased an actual Retry-After")
	}
}

func TestRetiredRegisteredFlowDoesNotCancelPassiveDownlinkWait(t *testing.T) {
	s := singleCandidateReplacement(t)
	watch := s.replacementDownlinkWatch
	old, peer := net.Pipe()
	t.Cleanup(func() { _ = peer.Close() })
	s.clearClosedRegistrationTCP(old, io.EOF)
	if s.replacementDownlinkWatch != watch || s.RegState() != regRegistered {
		t.Fatal("retired connection reset the current recovery round")
	}
}

func TestListenerFailureCancelsPassiveButPreservesServerDeadline(t *testing.T) {
	s := singleCandidateReplacement(t)
	deadline := time.Now().Add(time.Hour)
	s.registrarPenalties.mark(s.registrar, deadline)
	listener := &failedPortSListener{err: errors.New("listener failed")}
	s.securityServerIO = listener
	s.handleProtectedListenerFailure(listener, listener.err)
	s.registrarPenalties.mu.Lock()
	round := s.registrarPenalties.downlinkRound
	s.registrarPenalties.mu.Unlock()
	if round != nil || !s.registrarPenalties.states(time.Now())[s.registrar].retryNotBefore.Equal(deadline) {
		t.Fatal("listener recovery did not separate passive cadence from server backoff")
	}
}

func TestOldServiceFailurePreservesOverlappingReplacementRound(t *testing.T) {
	old := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, old)
	replacement := replacementUsingStore(t, old.registrarPenalties)
	if _, err := replacement.selectRegistrarCandidate(context.Background(), "tcp"); err != nil {
		t.Fatal(err)
	}
	startProtectedReplacementForTest(t, replacement)
	replacement.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, replacement))
	store := old.registrarPenalties
	store.mu.Lock()
	round := store.downlinkRound
	rediscoveryRequested := round != nil && round.rediscoveryRequested
	store.mu.Unlock()
	if !rediscoveryRequested || replacement.RegState() != regFailed {
		t.Fatal("replacement did not preserve the exhausted candidate set")
	}
	old.clearClosedRegistrationTCP(old.registrationTCP, io.EOF)
	store.mu.Lock()
	preserved := store.downlinkRound == round && round.rediscoveryRequested
	store.mu.Unlock()
	if !preserved || old.replacementDownlinkWatch != nil {
		t.Fatal("old service failure reset the overlapping replacement's recovery round")
	}
}
