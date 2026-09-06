package imscore

import (
	"context"
	"io"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestPortSFailureDuringPeriodicRegisterKeepsRecovery(t *testing.T) {
	s := newProtectedKeepaliveTestService(t)
	registration, peer := net.Pipe()
	t.Cleanup(func() { registration.Close(); peer.Close() })
	s.registrationTCP = registration
	s.registrationTCPProtected = true
	s.registrationTransport = "tcp"
	s.regState = regRegistering
	closeTestPortS(t, s, syscall.ETIMEDOUT)
	// Deliver the expired watchdog synchronously while REGISTER is in flight.
	s.portSReconnectGrace = time.Hour
	s.handleProtectedServerPushClosed()
	fireTestPortSWatch(t, s)
	if !s.portSReconnectWaiting.Load() {
		t.Fatal("in-flight REGISTER lost the outstanding downlink failure")
	}
	if !s.restoreRegistrationAfterFailedRefresh(registerResponseErrorWithRetryAfter(t, "600")) {
		t.Fatal("fixture did not preserve the registration")
	}
	s.portSWatchMu.Lock()
	pending := s.portSWatchTimer != nil
	s.portSWatchMu.Unlock()
	retryAt, waiting := s.portSRecoveryDeadline(time.Now())
	if !pending || !waiting || time.Until(retryAt) < 9*time.Minute {
		t.Fatal("failed periodic REGISTER did not resume recovery respecting Retry-After")
	}
	if s.reRegisterPending.Load() || s.pcscfRecoveryPending.Load() {
		t.Fatal("failed periodic REGISTER bypassed backoff or forced a proxy switch")
	}
}

func TestOldLocalPortSCloseDoesNotHideCurrentTimeout(t *testing.T) {
	s := newProtectedKeepaliveTestService(t)
	s.portSReconnectGrace = time.Hour
	old, oldPeer := net.Pipe()
	current, currentPeer := net.Pipe()
	t.Cleanup(func() { old.Close(); oldPeer.Close(); current.Close(); currentPeer.Close() })
	s.recordPortSOpened(old, time.Now())
	s.trackProtectedConnection(old)
	// Replacement detaches the old connection before its reader exits.
	s.untrackProtectedConnection(old)
	s.markPortSLocalClose(old)
	s.recordPortSOpened(current, time.Now())
	s.trackProtectedConnection(current)
	if !s.recordPortSClosed(current, syscall.ETIMEDOUT, time.Now()) {
		t.Fatal("current timeout did not qualify for recovery")
	}
	if s.recordPortSClosed(old, io.EOF, time.Now()) {
		t.Fatal("old local close incorrectly qualified for recovery")
	}
	s.untrackProtectedConnection(current)
	if got := s.capturePortSSession(); got.lastCloseKind != portSCloseTimeout {
		t.Fatalf("old cleanup overwrote current close state: %+v", got)
	}
	s.handleProtectedServerPushClosed()
	fireTestPortSWatch(t, s)
	if !s.portSRecoveryPending.Load() || !s.reRegisterPending.Load() {
		t.Fatal("old local close suppressed recovery of the current connection")
	}
}

func TestFailedPeriodicRegisterSupersedesPendingDownlinkValidation(t *testing.T) {
	s := newProtectedKeepaliveTestService(t)
	s.portSReconnectWaiting.Store(true)
	s.portSRecoveryAwaitingFlow.Store(true)
	s.completePortSRecovery(registerResponseErrorWithRetryAfter(t, "600"), true)
	if s.portSRecoveryAwaitingFlow.Load() {
		t.Fatal("stale validation would consume the next backoff expiry instead of retrying REGISTER")
	}
	if _, waiting := s.portSRecoveryDeadline(time.Now()); !waiting {
		t.Fatal("failed periodic REGISTER did not enter recovery backoff")
	}
}

func TestPeriodicRegisterCompletionPreservesPortSCompatibility(t *testing.T) {
	for _, outcome := range []string{"success", "binding lost", "clean EOF", "peer reconnected"} {
		t.Run(outcome, func(t *testing.T) {
			s := newProtectedKeepaliveTestService(t)
			s.portSReconnectGrace = time.Hour
			s.regState = regRegistering
			closeErr := error(syscall.ETIMEDOUT)
			if outcome == "clean EOF" {
				closeErr = io.EOF
				s.portSOnDemandObserved.Store(true)
			}
			closeTestPortS(t, s, closeErr)
			s.handleProtectedServerPushClosed()
			if outcome != "clean EOF" {
				fireTestPortSWatch(t, s)
			}
			if outcome == "peer reconnected" {
				conn, peer := net.Pipe()
				t.Cleanup(func() { conn.Close(); peer.Close() })
				s.recordPortSOpened(conn, time.Now())
				s.trackProtectedConnection(conn)
			}
			switch outcome {
			case "success":
				s.completePortSRecovery(nil, true)
			case "binding lost":
				s.completePortSRecovery(context.Canceled, false)
			default:
				s.completePortSRecovery(registerResponseErrorWithRetryAfter(t, "600"), true)
			}
			s.portSWatchMu.Lock()
			watching := s.portSWatchTimer != nil
			s.portSWatchMu.Unlock()
			if watching != (outcome == "success") || s.reRegisterPending.Load() {
				t.Fatalf("unexpected recovery watch %t for %s", watching, outcome)
			}
			if _, waiting := s.portSRecoveryDeadline(time.Now()); waiting {
				t.Fatal("non-failure or compatible peer state was incorrectly backed off")
			}
		})
	}
}

func TestLastRemainingOlderPortSConnectionStillRecovers(t *testing.T) {
	s := newProtectedKeepaliveTestService(t)
	s.portSReconnectGrace = time.Hour
	older, olderPeer := net.Pipe()
	newer, newerPeer := net.Pipe()
	t.Cleanup(func() { older.Close(); olderPeer.Close(); newer.Close(); newerPeer.Close() })
	s.recordPortSOpened(older, time.Now())
	s.trackProtectedConnection(older)
	s.recordPortSOpened(newer, time.Now())
	s.trackProtectedConnection(newer)
	s.recordPortSClosed(newer, syscall.ETIMEDOUT, time.Now())
	s.untrackProtectedConnection(newer)
	s.handleProtectedServerPushClosed()
	if s.portSReconnectWaiting.Load() {
		t.Fatal("failure of one connection ignored a surviving downlink")
	}
	if !s.recordPortSClosed(older, syscall.ETIMEDOUT, time.Now()) {
		t.Fatal("failure of the final surviving downlink was ignored due to its age")
	}
	s.untrackProtectedConnection(older)
	s.handleProtectedServerPushClosed()
	fireTestPortSWatch(t, s)
	if !s.reRegisterPending.Load() {
		t.Fatal("last surviving downlink did not request recovery")
	}
}

func TestPreviousRegistrarCloseCannotOverwritePortSFailure(t *testing.T) {
	s := newProtectedKeepaliveTestService(t)
	old, oldPeer := net.Pipe()
	current, currentPeer := net.Pipe()
	t.Cleanup(func() { old.Close(); oldPeer.Close(); current.Close(); currentPeer.Close() })
	s.registrar = "old.example:5060"
	s.recordPortSOpened(old, time.Now())
	s.registrar = "new.example:5060"
	s.recordPortSOpened(current, time.Now())
	s.recordPortSClosed(current, syscall.ETIMEDOUT, time.Now())
	if s.recordPortSClosed(old, syscall.ECONNRESET, time.Now()) {
		t.Fatal("old P-CSCF close qualified for recovery on the new P-CSCF")
	}
	if got := s.capturePortSSession(); got.lastCloseKind != portSCloseTimeout {
		t.Fatalf("old P-CSCF overwrote the new path's failure: %+v", got)
	}
}
