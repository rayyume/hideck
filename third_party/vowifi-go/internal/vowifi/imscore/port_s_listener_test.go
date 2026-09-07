package imscore

import (
	"errors"
	"net"
	"testing"
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
