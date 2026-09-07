package imscore

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPortSTimeoutEvidenceAndCancellationAreAtomic(t *testing.T) {
	for _, alternate := range []bool{false, true} {
		name := "single candidate"
		if alternate {
			name = "alternate available"
		}
		t.Run(name, func(t *testing.T) {
			assertPortSTimeoutAtomicity(t, alternate)
		})
	}
}

func assertPortSTimeoutAtomicity(t *testing.T, alternate bool) {
	t.Helper()
	s := newPortSTimeoutTestService(t)
	failPortSTimeoutValidation(t, s)
	if alternate {
		s.registrarCandidates = append(s.registrarCandidates, "pcscf-b.example:5060")
	}
	pending := s.pendingPortSTimeoutFailover()
	peer := newRecoveryCompletionPortS(t)
	s.registrationTCP, s.registrationTCPProtected = peer, true
	checkpoint := s.captureDownlinkCheckpoint()
	// Pause after request accounting, at the timeout-state lock. No observer
	// may acquire mu and see only half of the recovery state transition.
	s.portSSessionMu.Lock()
	locked := true
	defer func() {
		if locked {
			s.portSSessionMu.Unlock()
		}
	}()
	done := make(chan struct{})
	go func() {
		s.recordCurrentDownlinkRequest(peer, checkpoint)
		close(done)
	}()
	waitForTimeoutProofCriticalSection(t)
	assertNoPartialTimeoutProof(t, s, checkpoint)
	committed := make(chan bool, 1)
	go func() {
		s.registerMu.Lock()
		defer s.registerMu.Unlock()
		_, _, changed := s.commitPortSFailover(pending.registrar, portSFailoverCause{
			reason: portSTransportTimeoutFailure, generation: pending.generation,
		})
		committed <- changed
	}()
	s.portSSessionMu.Unlock()
	locked = false
	awaitTimeoutProofAndFailover(t, done, committed)
	if !s.portSTimeoutDownlinkProven() || s.RegState() != regRegistered ||
		len(s.registrarPenalties.states(time.Now())) != 0 {
		t.Fatal("proven downlink was penalized or its registration was interrupted")
	}
}

// The caller owns portSSessionMu while the request is waiting to update it.
func assertNoPartialTimeoutProof(t *testing.T, s *Service, checkpoint downlinkCheckpoint) {
	t.Helper()
	if !s.mu.TryRLock() {
		return
	}
	partial := s.downlinkRequests > checkpoint.requests && !s.portSSession.timeoutRecovery.downlinkProven
	s.mu.RUnlock()
	if partial {
		t.Error("published downlink evidence without canceling the pending timeout failover")
	}
}

func awaitTimeoutProofAndFailover(t *testing.T, done <-chan struct{}, committed <-chan bool) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("downlink evidence publication did not finish")
	}
	select {
	case changed := <-committed:
		if changed {
			t.Fatal("failover committed after the current downlink was proven")
		}
	case <-time.After(time.Second):
		t.Fatal("failover did not finish checking the recovered downlink")
	}
}

func TestPortSTimeoutProofNotificationCanReenterService(t *testing.T) {
	s := newPortSTimeoutTestService(t)
	failPortSTimeoutValidation(t, s)
	peer := newRecoveryCompletionPortS(t)
	s.registrationTCP, s.registrationTCPProtected = peer, true
	checkpoint := s.captureDownlinkCheckpoint()
	observed := make(chan bool, 1)
	s.SetOnSMSReadinessChanged(func(readiness SMSReadiness) {
		if readiness.Ready {
			now := s.captureDownlinkCheckpoint()
			observed <- now.requests > checkpoint.requests && s.portSTimeoutDownlinkProven()
		}
	})
	done := make(chan struct{})
	go func() {
		s.recordCurrentDownlinkRequest(peer, checkpoint)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SMS readiness callback deadlocked while reentering Service")
	}
	select {
	case proven := <-observed:
		if !proven {
			t.Fatal("callback observed only part of the recovery state transition")
		}
	default:
		t.Fatal("missing SMS readiness notification after proving the downlink")
	}
}

func waitForTimeoutProofCriticalSection(t *testing.T) {
	t.Helper()
	const stackBufferSize = 64 * 1024
	stacks := make([]byte, stackBufferSize)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		n := runtime.Stack(stacks, true)
		// The test owns portSSessionMu, so a request in this helper cannot
		// finish. Inspecting its stack avoids sleep-based scheduling guesses.
		if strings.Contains(string(stacks[:n]), "(*Service).confirmPortSTimeoutDownlink") {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("request did not reach the timeout-proof critical section")
}
