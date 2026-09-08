package imscore

import (
	"errors"
	"fmt"
	"net"
)

// Reserve both protocols before the initial REGISTER picks its unrelated UDP
// port. The TCP listener owns the reservations until the whole path is retired.
type protectedSIPListener struct {
	net.Listener
	udpClient, udpServer net.PacketConn
}

func (listener *protectedSIPListener) Close() error {
	return errors.Join(listener.Listener.Close(), listener.udpServer.Close(), listener.udpClient.Close())
}

func (s *Service) reserveProtectedUDPPorts(server, client net.Listener) (net.Listener, error) {
	network, ok := s.cfg.IMSNetwork.(interface {
		ListenProtectedUDP(*net.UDPAddr) (net.PacketConn, error)
	})
	if !ok {
		// No receive socket is opened. Defer the capability error until IPsec
		// is negotiated; initial rejection or a plain registration still works.
		return server, nil
	}
	portC, err := network.ListenProtectedUDP(&net.UDPAddr{IP: s.cfg.LocalIP, Port: tcpPort(client.Addr())})
	if err != nil {
		return nil, fmt.Errorf("imscore: reserve protected UDP port-c: %w", err)
	}
	portS, err := network.ListenProtectedUDP(&net.UDPAddr{IP: s.cfg.LocalIP, Port: tcpPort(server.Addr())})
	if err != nil {
		_ = portC.Close()
		return nil, fmt.Errorf("imscore: reserve protected UDP port-s: %w", err)
	}
	return &protectedSIPListener{Listener: server, udpClient: portC, udpServer: portS}, nil
}
