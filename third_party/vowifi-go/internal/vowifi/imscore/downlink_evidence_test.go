package imscore

import (
	"testing"
	"time"
)

func TestRetiredPeerCannotValidateReplacementDownlink(t *testing.T) {
	for _, change := range []string{"different registrar", "same registrar new transport", "retired peer"} {
		t.Run(change, func(t *testing.T) {
			s := newPortSTimeoutTestService(t)
			old := openFinalizationTestPortS(t, s)
			baseline := s.captureDownlinkCheckpoint()
			raw := optionsRequest("late-old-peer")
			message, err := parseSIPMessage(raw)
			if err != nil {
				t.Fatal(err)
			}
			reply := func(string) error {
				s.markPortSLocalClose(old)
				s.untrackProtectedConnection(old)
				if change == "same registrar new transport" {
					s.resetPortSRecoveryKnowledge()
				}
				s.mu.Lock()
				if change == "different registrar" {
					s.registrar = "pcscf-b.example:5060"
				}
				recordTestRegistrarFailure(s.registrarPenalties, s.registrar, time.Now().Add(-2*time.Minute))
				s.registrarRecoveryAttempt = registrarRecoveryAttempt{
					registrar: s.registrar, generation: s.registrarPenalties.recoveryGeneration(),
				}
				s.mu.Unlock()
				return nil
			}
			if err := s.dispatchInboundSIPMessageWithPeer(message, raw, reply, old); err != nil {
				t.Fatal(err)
			}
			s.portSFailoverVerifyWait = time.Millisecond
			if via, ok := s.waitForPortSFailoverValidation(baseline); ok {
				t.Fatalf("retired peer validated replacement via %s", via)
			}
			if s.inboundSIPHandledRequest.Load() != 1 {
				t.Fatal("late request was dropped instead of merely excluding its recovery evidence")
			}
			if !s.registrarPenalties.recoveryInProgress() {
				t.Fatal("late old-peer completion cleared the replacement failure history")
			}
		})
	}
}

func TestCurrentProtectedRequestValidatesDownlink(t *testing.T) {
	s := newPortSTimeoutTestService(t)
	conn := newRecoveryCompletionPortS(t)
	s.registrationTCP, s.registrationTCPProtected = conn, true
	baseline := s.captureDownlinkCheckpoint()
	raw := optionsRequest("current-protected-peer")
	message, err := parseSIPMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.dispatchInboundSIPMessageWithPeer(message, raw, func(string) error { return nil }, conn); err != nil {
		t.Fatal(err)
	}
	if via, ok := s.waitForPortSFailoverValidation(baseline); !ok || via != "inbound_sip_request" {
		t.Fatalf("current request did not validate downlink: %s, %t", via, ok)
	}
}
