package imscore

import "github.com/iniwex5/vowifi-go/internal/vowifi/logging"

// Optional to preserve system-network and externally supplied transport APIs.
type IMSNetworkDiagnosticsProvider interface {
	IMSNetworkDiagnostics() map[string]any
}

func (s *Service) networkDiagnostics() map[string]any {
	if provider, ok := s.cfg.IMSNetwork.(IMSNetworkDiagnosticsProvider); ok {
		return provider.IMSNetworkDiagnostics()
	}
	return map[string]any{"available": false}
}

func (s *Service) logDownlinkDiagnostics(event string) {
	s.mu.RLock()
	registrar := s.registrar
	listener := s.securityServerIO
	udp := s.protectedUDP
	s.mu.RUnlock()
	local := ""
	if listener != nil {
		local = listener.Addr().String()
	}
	udpLocal := ""
	if udp != nil {
		udpLocal = udp.server.LocalAddr().String()
	}
	logging.Info("IMS downlink path diagnostics",
		"device", s.DeviceID(), "event", event, "pcscf", registrar,
		"port_s_listener", local, "port_s_udp_listener", udpLocal, "network", s.networkDiagnostics())
}
