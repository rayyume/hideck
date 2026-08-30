package imscore

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imsheaders"
)

func (s *Service) originatingRouteSetLocked(serviceRoute string) string {
	pcscf := s.pcscfPreloadedRouteLocked()
	if pcscf == "" {
		return strings.TrimSpace(serviceRoute)
	}
	return imsheaders.PreloadedOriginatingRoute(pcscf, serviceRoute)
}

func (s *Service) pcscfPreloadedRouteLocked() string {
	host := ""
	port := 0
	if s.registrationTCP != nil {
		host, port = splitOutboundAddress(s.registrationTCP.RemoteAddr())
	}
	if (host == "" || net.ParseIP(strings.Trim(host, "[]")) == nil) &&
		s.registrationRemote != nil && s.registrationRemote.IP != nil {
		host = s.registrationRemote.IP.String()
		if port < 1 {
			port = s.registrationRemote.Port
		}
	}
	if s.regSession != nil && s.regSession.security != nil && s.regSession.security.server != nil {
		if negotiated := int(s.regSession.security.server.PortS); negotiated > 0 {
			port = negotiated
		}
	}
	transport := "tcp"
	if s.registrationTCP == nil && s.cfg != nil {
		transport = strings.ToLower(strings.TrimSpace(s.cfg.Transport))
		if transport == "" {
			transport = "udp"
		}
	}
	return formatPCSCFRoute(host, port, transport)
}

func formatPCSCFRoute(host string, port int, transport string) string {
	host = strings.Trim(strings.TrimSpace(host), "[]")
	if net.ParseIP(host) == nil || port < 1 {
		return ""
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	if transport = strings.ToLower(strings.TrimSpace(transport)); transport != "" {
		return fmt.Sprintf("<sip:%s;transport=%s;lr>", addr, transport)
	}
	return fmt.Sprintf("<sip:%s;lr>", addr)
}
