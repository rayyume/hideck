package imscore

import (
	"io"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestPortSWatchWaitsForRegisterSuccessFinalization(t *testing.T) {
	for _, phase := range []string{"before completion", "before registered", "before periodic schedule"} {
		t.Run(phase, func(t *testing.T) {
			s := newProtectedKeepaliveTestService(t)
			s.portSReconnectGrace = time.Hour
			conn := openFinalizationTestPortS(t, s)
			s.registerMu.Lock()
			defer s.registerMu.Unlock()
			s.regState = regRegistering
			if phase != "before completion" {
				s.completePortSRecovery(nil, true)
			}
			if phase == "before periodic schedule" {
				s.regState = regRegistered
			}
			closeFinalizationTestPortS(s, conn, syscall.ETIMEDOUT)
			done := queueFinalizationTestWatch(t, s)
			if phase == "before completion" {
				s.completePortSRecovery(nil, true)
			}
			s.mu.Lock()
			s.regState = regRegistered
			s.mu.Unlock()
			s.reRegisterPending.Store(false)
			s.scheduleRegistrationRefresh(time.Hour)
			finishFinalizationTestWatch(t, s, done)
			if phase == "before completion" {
				assertFinalizationTestWatch(t, s)
				return
			}
			if !s.portSRecoveryPending.Load() || !s.reRegisterPending.Load() ||
				s.nextIMSMaintenanceAction(time.Now()) != imsMaintenanceRefresh {
				t.Fatal("successful REGISTER lost or postponed the immediate downlink recovery")
			}
		})
	}
}

func TestPortSQueuedWatchRespectsFailedRegisterBackoff(t *testing.T) {
	s := newProtectedKeepaliveTestService(t)
	s.portSReconnectGrace = time.Hour
	registration, peer := net.Pipe()
	t.Cleanup(func() { registration.Close(); peer.Close() })
	s.registrationTCP = registration
	s.registrationTransport = "tcp"
	s.registerMu.Lock()
	defer s.registerMu.Unlock()
	s.regState = regRegistering
	closeTestPortS(t, s, syscall.ETIMEDOUT)
	s.handleProtectedServerPushClosed()
	done := queueFinalizationTestWatch(t, s)
	if !s.restoreRegistrationAfterFailedRefresh(registerResponseErrorWithRetryAfter(t, "600")) {
		t.Fatal("fixture did not preserve registration")
	}
	finishFinalizationTestWatch(t, s, done)
	assertFinalizationTestWatch(t, s)
	retryAt, waiting := s.portSRecoveryDeadline(time.Now())
	if !waiting || time.Until(retryAt) < 9*time.Minute || s.reRegisterPending.Load() {
		t.Fatal("queued callback bypassed the failed REGISTER's Retry-After")
	}
}

func TestPortSQueuedWatchIsCanceledByReconnectOrStop(t *testing.T) {
	for _, event := range []string{"reconnect", "stop"} {
		t.Run(event, func(t *testing.T) {
			s := newProtectedKeepaliveTestService(t)
			s.portSReconnectGrace = time.Hour
			s.registerMu.Lock()
			defer s.registerMu.Unlock()
			closeTestPortS(t, s, syscall.ETIMEDOUT)
			s.handleProtectedServerPushClosed()
			done := queueFinalizationTestWatch(t, s)
			if event == "reconnect" {
				openFinalizationTestPortS(t, s)
			} else {
				s.StopCurrent()
			}
			finishFinalizationTestWatch(t, s, done)
			if s.portSRecoveryPending.Load() || s.reRegisterPending.Load() {
				t.Fatal("obsolete watchdog requested another REGISTER")
			}
		})
	}
}

func TestRetiredPortSDoesNotHideFinalActiveFailure(t *testing.T) {
	for _, retiredRegistrar := range []string{"current.example:5060", "previous.example:5060"} {
		t.Run(retiredRegistrar, func(t *testing.T) {
			s := newProtectedKeepaliveTestService(t)
			s.portSReconnectGrace = time.Hour
			s.registrar = retiredRegistrar
			retired := openFinalizationTestPortS(t, s)
			s.untrackProtectedConnection(retired)
			s.markPortSLocalClose(retired)
			s.registrar = "current.example:5060"
			older := openFinalizationTestPortS(t, s)
			newer := openFinalizationTestPortS(t, s)
			closeFinalizationTestPortS(s, newer, io.EOF)
			closeFinalizationTestPortS(s, older, syscall.ETIMEDOUT)
			closeFinalizationTestPortS(s, retired, io.EOF)
			assertFinalizationTestWatch(t, s)
			if s.capturePortSSession().lastCloseKind != portSCloseTimeout {
				t.Fatal("retired connection hid the final live connection's timeout")
			}
			fireTestPortSWatch(t, s)
			if !s.reRegisterPending.Load() {
				t.Fatal("last active downlink did not request recovery")
			}
		})
	}
}

func TestConcurrentPortSUntrackingCannotLoseRecovery(t *testing.T) {
	s := newProtectedKeepaliveTestService(t)
	s.portSReconnectGrace = time.Hour
	older := openFinalizationTestPortS(t, s)
	newer := openFinalizationTestPortS(t, s)
	recoverOlder := s.recordPortSClosed(older, syscall.ETIMEDOUT, time.Now())
	// The other reader finishes before the first reader can untrack itself.
	closeFinalizationTestPortS(s, newer, syscall.ETIMEDOUT)
	s.untrackProtectedConnection(older)
	if recoverOlder {
		s.handleProtectedServerPushClosed()
	}
	assertFinalizationTestWatch(t, s)
	fireTestPortSWatch(t, s)
	if !s.reRegisterPending.Load() {
		t.Fatal("interleaved reader cleanup lost the last downlink's recovery")
	}
}

func openFinalizationTestPortS(t *testing.T, s *Service) net.Conn {
	t.Helper()
	conn, peer := net.Pipe()
	t.Cleanup(func() { conn.Close(); peer.Close() })
	s.recordPortSOpened(conn, time.Now())
	s.trackProtectedConnection(conn)
	return conn
}

func closeFinalizationTestPortS(s *Service, conn net.Conn, err error) {
	recoverFlow := s.recordPortSClosed(conn, err, time.Now())
	s.untrackProtectedConnection(conn)
	if recoverFlow {
		s.handleProtectedServerPushClosed()
	}
}

// The caller holds registerMu, just as registerLocked does in production.
func queueFinalizationTestWatch(t *testing.T, s *Service) <-chan struct{} {
	t.Helper()
	s.portSWatchMu.Lock()
	timer, generation := s.portSWatchTimer, s.portSWatchGeneration
	s.portSWatchMu.Unlock()
	if timer == nil {
		t.Fatal("missing recovery watchdog")
	}
	timer.Stop()
	registrar := s.currentPortSRecoveryRegistrar()
	done := make(chan struct{})
	go func() {
		s.portSReconnectWatchFired(generation, registrar)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("watchdog ran before REGISTER finalization released registerMu")
	case <-time.After(10 * time.Millisecond):
	}
	return done
}

func finishFinalizationTestWatch(t *testing.T, s *Service, done <-chan struct{}) {
	t.Helper()
	s.registerMu.Unlock()
	defer s.registerMu.Lock()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watchdog did not finish after REGISTER unlocked")
	}
}

func assertFinalizationTestWatch(t *testing.T, s *Service) {
	t.Helper()
	s.portSWatchMu.Lock()
	watching := s.portSWatchTimer != nil
	s.portSWatchMu.Unlock()
	if !watching {
		t.Fatal("missing recovery watch after the final live downlink failed")
	}
}
