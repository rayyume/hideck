package imscore

import (
	"errors"
	"net"
	"testing"
	"time"
)

type failedPortSListener struct{ err error }

func (l *failedPortSListener) Accept() (net.Conn, error) { return nil, l.err }
func (l *failedPortSListener) Close() error              { return nil }
func (l *failedPortSListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(192, 0, 2, 1), Port: 41001}
}

func TestPortSListenerFailureRequestsRecovery(t *testing.T) {
	s := singleCandidateReplacement(t)
	listener := &failedPortSListener{err: errors.New("listener endpoint failed")}
	s.securityServerIO = listener
	s.networkDone.Add(1)
	s.acceptProtectedSIP(listener)
	select {
	case err := <-s.RegistrationErrors():
		if !errors.Is(err, listener.err) {
			t.Fatalf("listener cause lost: %v", err)
		}
	default:
		t.Fatal("port-s listener exited without requesting recovery")
	}
	if s.RegState() == regRegistered || s.securityServerIO != nil || s.replacementDownlinkWatch != nil {
		t.Fatal("failed listener left its registration/watch active")
	}
	if status := s.StatusCurrent(); status.Registered || status.SignalingReady {
		t.Fatal("failed listener still advertised a healthy registration")
	}
}

func TestRetiredPortSListenerCannotFailReplacement(t *testing.T) {
	s := singleCandidateReplacement(t)
	current := &failedPortSListener{err: net.ErrClosed}
	s.securityServerIO = current
	s.networkDone.Add(1)
	s.acceptProtectedSIP(&failedPortSListener{err: net.ErrClosed})
	if s.securityServerIO != current || s.RegState() != regRegistered || len(s.RegistrationErrors()) != 0 {
		t.Fatal("retired listener damaged the replacement")
	}
}

type delayedClosePortSListener struct {
	failedPortSListener
	closing chan struct{}
	resume  chan struct{}
}

func (l *delayedClosePortSListener) Close() error {
	close(l.closing)
	<-l.resume
	return nil
}

func TestPortSListenerFailureRechecksOwnershipAfterClose(t *testing.T) {
	s := singleCandidateReplacement(t)
	listener := &delayedClosePortSListener{
		failedPortSListener: failedPortSListener{err: net.ErrClosed},
		closing:             make(chan struct{}), resume: make(chan struct{}),
	}
	s.securityServerIO = listener
	done := make(chan struct{})
	go func() {
		s.handleProtectedListenerFailure(listener, listener.err)
		close(done)
	}()
	<-listener.closing
	// A P-CSCF switch can finish while closing the retired socket takes time.
	current := &failedPortSListener{err: net.ErrClosed}
	s.registerMu.Lock()
	s.mu.Lock()
	s.securityServerIO = current
	s.regState = regRegistered
	s.regStatus.Store(registrationRegistered)
	s.mu.Unlock()
	s.registerMu.Unlock()
	close(listener.resume)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("listener failure handler did not finish")
	}
	if s.securityServerIO != current || s.RegState() != regRegistered ||
		s.regStatus.Load() != registrationRegistered || len(s.RegistrationErrors()) != 0 {
		t.Fatal("retired listener completion rejected the replacement registration")
	}
}

func TestPortSListenerFailureDoesNotBlockStopWhileRegisterOwnsLock(t *testing.T) {
	// Isolate the listener from subscription workers that also use networkDone.
	s := newPortSSessionTestService(t, vodafoneUKCarrierPresetID)
	listener := &failedPortSListener{err: net.ErrClosed}
	s.securityServerIO = listener
	s.registerMu.Lock()
	locked := true
	defer func() {
		if locked {
			s.registerMu.Unlock()
		}
	}()
	s.networkDone.Add(1)
	done := make(chan struct{})
	go func() {
		s.acceptProtectedSIP(listener)
		close(done)
	}()
	retired := make(chan struct{})
	go func() { s.networkDone.Wait(); close(retired) }()
	select {
	case <-retired:
	case <-time.After(time.Second):
		t.Fatal("listener recovery kept the stopped receiver in networkDone")
	}
	if len(s.RegistrationErrors()) != 0 {
		t.Fatal("listener failure raced the in-progress REGISTER")
	}
	s.StopCurrent()
	s.registerMu.Unlock()
	locked = false
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("listener recovery did not retire after shutdown")
	}
	if len(s.RegistrationErrors()) != 0 {
		t.Fatal("stopped service published the queued listener failure")
	}
}
