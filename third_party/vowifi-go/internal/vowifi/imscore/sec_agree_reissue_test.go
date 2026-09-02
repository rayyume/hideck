package imscore

import (
	"testing"
	"time"
)

func reissueTestMechanism(spiC, spiS uint32) *securityMechanism {
	return &securityMechanism{
		Name: "ipsec-3gpp", Auth: "hmac-sha-1-96", Encryption: "aes-cbc",
		Protocol: "esp", Mode: "trans",
		SPIC: spiC, SPIS: spiS, PortC: 50601, PortS: 50600,
	}
}

// A repeat 401 hands out fresh SPIs on the same client ports. The established
// flow then runs under SPIs the P-CSCF has replaced, so it has to be spotted
// rather than reused for the next REGISTER.
func TestProtectedRegistrationSAReplacedDetectsReissuedSPIs(t *testing.T) {
	session := &registerSession{
		security: &securityAgreement{server: reissueTestMechanism(0x1111, 0x2222)},
	}
	if protectedRegistrationSAReplaced(session, reissueTestMechanism(0x1111, 0x2222)) {
		t.Fatal("an unchanged offer was treated as a re-issued SA")
	}
	if !protectedRegistrationSAReplaced(session, reissueTestMechanism(0x3333, 0x4444)) {
		t.Fatal("a re-issued SA went unnoticed")
	}
	if protectedRegistrationSAReplaced(&registerSession{}, reissueTestMechanism(0x3333, 0x4444)) {
		t.Fatal("a session holding no SA was treated as a re-issue")
	}
	if protectedRegistrationSAReplaced(session, nil) {
		t.Fatal("a missing offer was treated as a re-issue")
	}
}

// Abandoning the attempt only pays off if it retries promptly: the ports it
// needs are only freed by a fresh attempt reserving new ones.
func TestReissuedSAAbandonsAttemptAsTransportFailure(t *testing.T) {
	service, err := New(registerTransportTestConfig("tcp", "127.0.0.1:5060"))
	if err != nil {
		t.Fatal(err)
	}
	defer service.StopCurrent()

	if !registerAttemptTransportFailure(errProtectedFlowNeedsFreshPorts) {
		t.Fatal("a re-issued SA was not treated as a transport failure")
	}

	service.reRegisterPending.Store(false)
	service.applyRegistrationFailureStatus(&registerAttemptError{
		result: registerAttemptResult{statusCode: 401, challengeCount: 1, secAgree: true},
		err:    errProtectedFlowNeedsFreshPorts,
	})

	if !service.reRegisterPending.Load() {
		t.Fatal("the attempt was not retried")
	}
	service.mu.Lock()
	next := service.nextRegister
	service.mu.Unlock()
	if wait := time.Until(next); wait > time.Minute {
		t.Fatalf("next REGISTER in %s, want a prompt retry with fresh ports", wait)
	}
}
