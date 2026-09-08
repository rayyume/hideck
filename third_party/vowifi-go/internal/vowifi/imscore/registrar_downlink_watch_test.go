package imscore

import (
	"context"
	"strings"
	"testing"
	"time"
)

func startProtectedReplacementForTest(t *testing.T, s *Service) {
	t.Helper()
	portC := newRecoveryCompletionPortS(t)
	s.transport.SetSendFn(func(request string) error {
		if strings.HasPrefix(request, "REGISTER ") {
			// Test the post-AKA finalization independently of the challenge exchange.
			s.mu.Lock()
			s.externalTransport = false
			s.registrationTransport = "tcp"
			s.registrationTCP, s.registrationTCPProtected = portC, true
			s.regSession.publicID = "sip:+447840844894@ims.example"
			s.regSession.security = &securityAgreement{
				verifyHeader: "ipsec-3gpp;alg=hmac-sha-1-96",
				server:       &securityMechanism{PortS: 6060},
			}
			s.mu.Unlock()
		}
		s.transport.DeliverResponse(registerResponseForRequest(request, 200, nil))
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}
}

func expireReplacementWatchForTest(t *testing.T, s *Service) *replacementDownlinkWatch {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	watch := s.replacementDownlinkWatch
	if watch == nil || watch.timer == nil {
		t.Fatal("missing replacement downlink validation timer")
	}
	watch.timer.Stop()
	watch.deadline = time.Now().Add(-time.Second)
	return watch
}

func TestReplacementRegisterWithoutDownlinkSchedulesRecovery(t *testing.T) {
	s := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, s)
	if !s.protectedDownlinkValidationRequired() || !s.registrarPenalties.recoveryInProgress() {
		t.Fatal("REGISTER 200 prematurely completed recovery")
	}
	watch := expireReplacementWatchForTest(t, s)
	s.replacementDownlinkWatchFired(watch)
	select {
	case err := <-s.RegistrationErrors():
		if !strings.Contains(err.Error(), "initial registration failed") {
			t.Fatal(err)
		}
	default:
		t.Fatal("replacement without port-s never escalated recovery")
	}
	entry := s.registrarPenalties.states(time.Now())[watch.path.registrar]
	if entry.reason != "downlink_unverified" || entry.consecutiveFailures != 1 {
		t.Fatalf("unverified downlink was counted as another REGISTER failure: %+v", entry)
	}
}

func TestReplacementDownlinkWatchSurvivesBusyRecoveryOwner(t *testing.T) {
	s := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, s)
	watch := expireReplacementWatchForTest(t, s)
	s.pcscfRecoveryPending.Store(true)
	s.replacementDownlinkWatchFired(watch)
	s.recoverPCSCFAfterMTReportReject("retired.example:5060", 488, time.Now().Add(time.Minute))
	select {
	case <-s.RegistrationErrors():
	case <-time.After(time.Second):
		t.Fatal("replacement validation was lost when the stale recovery owner exited")
	}
}

func TestReplacementDownlinkWatchCanceledByProofOrLifecycle(t *testing.T) {
	for _, event := range []string{"port-s", "current request", "stop", "transport reset"} {
		t.Run(event, func(t *testing.T) {
			s := newRecoveryCompletionTestService(t)
			startProtectedReplacementForTest(t, s)
			watch := expireReplacementWatchForTest(t, s)
			switch event {
			case "port-s":
				s.trackProtectedConnection(newRecoveryCompletionPortS(t))
			case "current request":
				s.mu.RLock()
				peer := s.registrationTCP
				s.mu.RUnlock()
				s.recordCurrentDownlinkRequest(peer, s.captureDownlinkCheckpoint())
			case "stop":
				s.StopCurrent()
			case "transport reset":
				s.resetPortSRecoveryKnowledge()
			}
			s.replacementDownlinkWatchFired(watch)
			s.mu.RLock()
			pending := s.replacementDownlinkWatch != nil
			s.mu.RUnlock()
			if pending || len(s.RegistrationErrors()) != 0 {
				t.Fatal("stale validation timer interrupted a proven or retired path")
			}
		})
	}
}

func TestReplacementDownlinkWatchIsCarrierAndIncidentScoped(t *testing.T) {
	for _, preset := range []string{vodafoneUKCarrierPresetID, "2degrees_nz", "ctexcel", ""} {
		t.Run(preset, func(t *testing.T) {
			s := newRecoveryCompletionTestService(t)
			s.cfg.CarrierPresetID = preset
			startProtectedReplacementForTest(t, s)
			s.mu.RLock()
			pending := s.replacementDownlinkWatch != nil
			s.mu.RUnlock()
			if pending != (preset == vodafoneUKCarrierPresetID) {
				t.Fatal("replacement downlink policy leaked across carrier presets")
			}
		})
	}
	s := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	startProtectedReplacementForTest(t, s)
	s.mu.RLock()
	pending := s.replacementDownlinkWatch != nil
	s.mu.RUnlock()
	if pending {
		t.Fatal("ordinary cold start was treated as an incident replacement")
	}
}

func TestPeriodicRegisterCannotPostponeReplacementDownlinkDeadline(t *testing.T) {
	s := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, s)
	s.mu.RLock()
	watch := s.replacementDownlinkWatch
	deadline := watch.deadline
	s.mu.RUnlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Register(ctx); err != nil {
		t.Fatal(err)
	}
	s.mu.RLock()
	unchanged := s.replacementDownlinkWatch == watch && watch.deadline.Equal(deadline)
	s.mu.RUnlock()
	if !unchanged {
		t.Fatal("periodic REGISTER changed the downlink verification deadline")
	}
}

func TestReplacementValidationFailureRetainsRegisterRetryAfter(t *testing.T) {
	s := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, s)
	before := time.Now()
	s.completePortSRecovery(registerResponseErrorWithRetryAfter(t, "3600"), true)
	watch := expireReplacementWatchForTest(t, s)
	s.replacementDownlinkWatchFired(watch)
	entry := s.registrarPenalties.states(time.Now())[watch.path.registrar]
	if entry.retryNotBefore.Before(before.Add(time.Hour)) {
		t.Fatalf("validation failure lost REGISTER Retry-After: %+v", entry)
	}
}
