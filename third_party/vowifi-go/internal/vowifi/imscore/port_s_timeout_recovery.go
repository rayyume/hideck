package imscore

import (
	"net"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const portSTransportTimeoutFailure = "port_s_transport_timeout"

// Protected by portSSessionMu. A timeout alone never requests a proxy switch:
// the original REGISTER must succeed and its downlink validation must fail.
type portSTimeoutRecoveryState struct {
	registrar        string
	generation       uint64
	observedAt       time.Time
	validationFailed bool
	downlinkProven   bool
}

func (s *Service) recordPortSTimeoutLocked(registrar, kind string) {
	s.portSSession.timeoutRecovery = portSTimeoutRecoveryState{}
	if kind == portSCloseTimeout && usesVodafoneUKPortSResetRecovery(s.cfg) {
		s.portSSession.timeoutRecovery = portSTimeoutRecoveryState{
			registrar: registrar, generation: s.portSSession.generation,
			observedAt: s.portSSession.closedAt,
		}
	}
}

func (s *Service) armPortSTimeoutFailover() bool {
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	state := &s.portSSession.timeoutRecovery
	if state.registrar == "" || state.downlinkProven || s.portSPushReady.Load() {
		return false
	}
	state.validationFailed = true
	return true
}

func (s *Service) pendingPortSTimeoutFailover() portSTimeoutRecoveryState {
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	state := s.portSSession.timeoutRecovery
	if !state.validationFailed || state.downlinkProven {
		return portSTimeoutRecoveryState{}
	}
	return state
}

// Called by the recovery watchdog with registerMu held, after its retry deadline.
func (s *Service) recoverTimedOutPortSLocked() bool {
	state := s.pendingPortSTimeoutFailover()
	if state.registrar == "" {
		return false
	}
	if !s.pcscfRecoveryPending.CompareAndSwap(false, true) {
		return true
	}
	defer s.finishPCSCFRecovery()
	s.recoverPCSCFAfterPortSFailureLocked(state.registrar, portSFailoverCause{
		reason: portSTransportTimeoutFailure, observedAt: state.observedAt,
		generation: state.generation,
	})
	return true
}

// Called with mu held immediately before committing the proxy switch.
func (s *Service) consumePortSTimeoutFailoverLocked(registrar string, generation uint64) bool {
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	state := s.portSSession.timeoutRecovery
	if state.registrar != registrar || state.generation != generation ||
		!state.validationFailed || state.downlinkProven || s.portSPushReady.Load() ||
		s.portSSession.generation != generation || s.portSSession.lastCloseKind != portSCloseTimeout {
		return false
	}
	s.portSSession.timeoutRecovery = portSTimeoutRecoveryState{}
	return true
}

func (s *Service) clearPortSTimeoutRecovery() portSTimeoutRecoveryState {
	s.portSSessionMu.Lock()
	previous := s.portSSession.timeoutRecovery
	s.portSSession.timeoutRecovery = portSTimeoutRecoveryState{}
	s.portSSessionMu.Unlock()
	return previous
}

func (s *Service) portSTimeoutDownlinkProven() bool {
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	return s.portSSession.timeoutRecovery.downlinkProven
}

// A successfully handled request on the current protected connection can also
// prove reachability. Late traffic from detached connections must not cancel
// recovery of their replacement. REGISTER responses never enter this path.
func (s *Service) confirmPortSTimeoutDownlink(peer net.Conn) {
	if peer == nil {
		return
	}
	s.mu.RLock()
	s.portSSessionMu.Lock()
	state := &s.portSSession.timeoutRecovery
	connection, tracked := s.portSSession.connections[peer]
	current := (peer == s.registrationTCP && s.registrationTCPProtected) ||
		(tracked && !connection.localClosing && connection.registrar == s.registrar)
	proven := state.registrar != "" && !state.downlinkProven &&
		state.registrar == strings.TrimSpace(s.registrar) && current && !s.stopped()
	if proven {
		state.downlinkProven = true
		state.validationFailed = false
		s.portSRecoveryAwaitingFlow.Store(false)
		s.portSReconnectWaiting.Store(false)
		s.resetPortSRecoveryBackoff()
	}
	s.portSSessionMu.Unlock()
	s.mu.RUnlock()
	if !proven {
		return
	}
	s.notifySMSReadiness()
	logging.Info("IMS port-s timeout recovery validated by current inbound SIP request", "device", s.DeviceID())
}
