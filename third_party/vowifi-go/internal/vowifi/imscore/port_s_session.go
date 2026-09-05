package imscore

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"syscall"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const (
	portSClosePeerReset = "peer_reset"
	portSCloseEOF       = "eof"
	portSCloseLocal     = "local_close"
	portSCloseOther     = "other"
)

type portSConnectionState struct {
	openedAt     time.Time
	registrar    string
	localClosing bool
}

type portSSessionState struct {
	connections     map[net.Conn]portSConnectionState
	generation      uint64
	openedAt        time.Time
	closedAt        time.Time
	lastInboundAt   time.Time
	lastCloseKind   string
	lastCloseReason string
	peerResetCount  uint64
	resetRecovery   portSResetRecoveryState
}

type portSSessionSnapshot struct {
	connected       bool
	generation      uint64
	openedAt        time.Time
	closedAt        time.Time
	lastInboundAt   time.Time
	lastCloseKind   string
	lastCloseReason string
	peerResetCount  uint64
}

func (s *Service) recordPortSOpened(conn net.Conn, now time.Time) {
	if s == nil || conn == nil {
		return
	}
	registrar := s.currentPortSRecoveryRegistrar()
	s.portSSessionMu.Lock()
	if s.portSSession.connections == nil {
		s.portSSession.connections = make(map[net.Conn]portSConnectionState)
	}
	s.portSSession.connections[conn] = portSConnectionState{openedAt: now, registrar: registrar}
	s.portSSession.generation++
	s.portSSession.openedAt = now
	generation := s.portSSession.generation
	s.portSSessionMu.Unlock()
	logging.Info("IMS port-s lifecycle",
		"device", s.DeviceID(), "event", "opened", "generation", generation,
		"pcscf", registrar, "inner_ip", s.cfg.LocalAddr,
		"remote", conn.RemoteAddr(), "local", conn.LocalAddr())
}

func (s *Service) recordPortSInbound(now time.Time) {
	if s == nil {
		return
	}
	s.portSSessionMu.Lock()
	s.portSSession.lastInboundAt = now
	s.portSSessionMu.Unlock()
}

func (s *Service) recordPortSClosed(conn net.Conn, err error, now time.Time) {
	if s == nil || conn == nil {
		return
	}
	currentRegistrar := s.currentPortSRecoveryRegistrar()
	s.portSSessionMu.Lock()
	connection := s.portSSession.connections[conn]
	delete(s.portSSession.connections, conn)
	registrar := connection.registrar
	if registrar == "" {
		registrar = currentRegistrar
	}
	kind := classifyPortSClose(err)
	if connection.localClosing {
		kind = portSCloseLocal
	}
	s.portSSession.closedAt = now
	s.portSSession.lastCloseKind = kind
	s.portSSession.lastCloseReason = errorText(err)
	startFailover := false
	if kind == portSClosePeerReset {
		s.portSSession.peerResetCount++
		startFailover = s.armVodafoneUKResetRecoveryLocked(registrar, connection.openedAt, now)
	}
	generation := s.portSSession.generation
	lastInboundAt := s.portSSession.lastInboundAt
	s.portSSessionMu.Unlock()
	logging.Info("IMS port-s lifecycle",
		"device", s.DeviceID(), "event", "closed", "generation", generation,
		"pcscf", registrar, "inner_ip", s.cfg.LocalAddr,
		"close_kind", kind, "reason", errorText(err),
		"lifetime", elapsedSince(connection.openedAt, now),
		"last_inbound_at", lastInboundAt)
	if startFailover {
		s.startPendingPortSResetFailover()
	}
}

func (s *Service) markPortSLocalClose(conn net.Conn) {
	if s == nil || conn == nil {
		return
	}
	s.portSSessionMu.Lock()
	state, exists := s.portSSession.connections[conn]
	if exists {
		state.localClosing = true
		s.portSSession.connections[conn] = state
	}
	s.portSSessionMu.Unlock()
}

func (s *Service) portSRegistrar(conn net.Conn) string {
	if s == nil || conn == nil {
		return ""
	}
	s.portSSessionMu.Lock()
	registrar := s.portSSession.connections[conn].registrar
	s.portSSessionMu.Unlock()
	return strings.TrimSpace(registrar)
}

func (s *Service) capturePortSSession() portSSessionSnapshot {
	if s == nil {
		return portSSessionSnapshot{}
	}
	s.portSSessionMu.Lock()
	defer s.portSSessionMu.Unlock()
	state := s.portSSession
	return portSSessionSnapshot{
		connected: s.portSPushReady.Load(), generation: state.generation,
		openedAt: state.openedAt, closedAt: state.closedAt,
		lastInboundAt: state.lastInboundAt, lastCloseKind: state.lastCloseKind,
		lastCloseReason: state.lastCloseReason, peerResetCount: state.peerResetCount,
	}
}

func classifyPortSClose(err error) string {
	if err == nil || errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
		return portSCloseOther
	}
	if errors.Is(err, syscall.ECONNRESET) || strings.Contains(strings.ToLower(err.Error()), "connection reset by peer") {
		return portSClosePeerReset
	}
	if errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.EPIPE) {
		return portSCloseOther
	}
	if errors.Is(err, syscall.ECONNABORTED) {
		return portSCloseOther
	}
	if errors.Is(err, io.EOF) {
		return portSCloseEOF
	}
	return portSCloseOther
}

func elapsedSince(start, end time.Time) time.Duration {
	if start.IsZero() || end.Before(start) {
		return 0
	}
	return end.Sub(start).Round(time.Millisecond)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Service) recordIMSRegistrationSucceeded() {
	if s != nil {
		s.registrationGeneration.Add(1)
	}
}

func (s *Service) logIMSRegistrationAttempt(session *registerSession, request string) {
	if s == nil || session == nil {
		return
	}
	portS := s.capturePortSSession()
	regID, _ := contactParameterValue(rawSIPHeaderValue(request, "Contact"), "reg-id")
	logging.Info("IMS REGISTER attempt",
		"device", s.DeviceID(), "cseq", session.cseq,
		"pcscf", s.currentPortSRecoveryRegistrar(), "inner_ip", s.cfg.LocalAddr,
		"reg_id", strings.TrimSpace(regID), "port_s_generation", portS.generation,
		"port_s_connected", portS.connected, "port_s_last_inbound_at", portS.lastInboundAt)
}

func (s *Service) registeredFlowRegID() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return registeredFlowRegIDLocked(s)
}

func registeredFlowRegIDLocked(s *Service) int {
	if s != nil && s.outboundContactRegistered {
		return 1
	}
	return 0
}
