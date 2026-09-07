package imscore

import (
	"fmt"
	"net"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

func (s *Service) handleProtectedListenerFailure(listener net.Listener, cause error) {
	err := fmt.Errorf("imscore: protected port-s listener failed: %w", cause)
	s.mu.Lock()
	// Normal teardown detaches the listener before closing it. A late Accept
	// error from that listener must not invalidate a replacement registration.
	if s.stopped() || s.securityServerIO != listener {
		s.mu.Unlock()
		return
	}
	s.securityServerIO = nil
	s.regState = regFailed
	s.signalingReady = false
	s.signalingFailureReason = err.Error()
	s.lastError = err.Error()
	s.abandonReplacementDownlinkWaitLocked()
	s.mu.Unlock()
	_ = listener.Close()
	logging.Info("IMS protected port-s listener failed; requesting runtime recovery",
		"device", s.DeviceID(), "local", listener.Addr(), "err", err)
	s.transitionRegStatus(registrationRejectedTemporary)
	s.notifySMSReadiness()
	s.logDownlinkDiagnostics("listener_failed")
	s.reportRegistrationRuntimeError(err)
}
