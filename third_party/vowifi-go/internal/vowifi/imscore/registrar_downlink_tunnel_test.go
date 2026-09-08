package imscore

import (
	"net"
	"testing"
	"time"
)

type downlinkRecoveryNetwork struct {
	*SystemIMSNetwork
	closed bool
}

func (network *downlinkRecoveryNetwork) Close() error { network.closed = true; return nil }

func TestDownlinkValidationSwitchesIMSWithoutClosingTunnel(t *testing.T) {
	registrar := listenProtectedTestUDP(t)
	seen := make(chan string, 1)
	go serveRegisterStatus(registrar, 200, seen)
	s := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, s)
	network := &downlinkRecoveryNetwork{SystemIMSNetwork: NewSystemIMSNetwork(net.IPv4(127, 0, 0, 1))}
	s.cfg.IMSNetwork = network
	s.mu.Lock()
	s.registrarCandidates = []string{s.registrar, registrar.LocalAddr().String()}
	s.mu.Unlock()
	s.replacementDownlinkWatchFired(expireReplacementWatchForTest(t, s))
	if network.closed || len(s.RegistrationErrors()) != 0 || s.RegState() != regRegistered {
		t.Fatal("successful alternate registration tore down the tunnel")
	}
	if s.currentPortSRecoveryRegistrar() != registrar.LocalAddr().String() {
		t.Fatal("recovery did not use the selected candidate")
	}
	select {
	case <-seen:
	case <-time.After(time.Second):
		t.Fatal("alternate never received REGISTER")
	}
}

func TestDownlinkValidationDeadTransportStillRequiresRuntimeRepair(t *testing.T) {
	s := newRecoveryCompletionTestService(t)
	startProtectedReplacementForTest(t, s)
	s.mu.Lock()
	s.registrationTCP = nil
	s.registrationTCPProtected = false
	s.mu.Unlock()
	plan, claimed := s.claimReplacementDownlinkRecovery(expireReplacementWatchForTest(t, s))
	if !claimed || plan.reuseTunnel {
		t.Fatal("dead registered transport was mistaken for a healthy tunnel path")
	}
}
