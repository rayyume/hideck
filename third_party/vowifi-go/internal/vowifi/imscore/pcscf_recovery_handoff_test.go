package imscore

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPortSTimeoutWatchSurvivesStaleRecoveryOwner(t *testing.T) {
	s := newPortSTimeoutTestService(t)
	failPortSTimeoutValidation(t, s)
	// A stale report handler has claimed ownership, but is queued on registerMu.
	s.pcscfRecoveryPending.Store(true)
	expirePortSTimeoutBackoff(t, s)
	s.requestFreshRuntimeAfterMTReportReject("retired.example:5060", 488, time.Now().Add(time.Minute))
	select {
	case err := <-s.RegistrationErrors():
		if !strings.Contains(err.Error(), "fresh runtime required") {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending timeout recovery was not resumed after the stale owner exited")
	}
}

func TestRecoveryHandoffPreservesTimeoutBackoff(t *testing.T) {
	s := newPortSTimeoutTestService(t)
	failPortSTimeoutValidation(t, s)
	retryAt, _ := s.portSRecoveryDeadline(time.Now())
	s.pcscfRecoveryPending.Store(true)
	s.finishPCSCFRecovery()
	if got, _ := s.portSRecoveryDeadline(time.Now()); !got.Equal(retryAt) {
		t.Fatalf("handoff changed retry deadline: %v != %v", got, retryAt)
	}
	fireTestPortSWatch(t, s)
	if s.pcscfRecoveryPending.Load() || s.reRegisterPending.Load() || len(s.RegistrationErrors()) != 0 {
		t.Fatal("handoff bypassed the existing backoff")
	}
}

func TestPortSTimeoutCloseIsScheduledWhileAnotherRecoveryOwnsFlag(t *testing.T) {
	s := newPortSTimeoutTestService(t)
	s.pcscfRecoveryPending.Store(true)
	closeTestPortS(t, s, syscall.ETIMEDOUT)
	s.handleProtectedServerPushClosed()
	s.portSWatchMu.Lock()
	hasWatch := s.portSWatchTimer != nil
	s.portSWatchMu.Unlock()
	if !hasWatch {
		t.Fatal("concurrent recovery owner swallowed the initial port-s failure")
	}
}
