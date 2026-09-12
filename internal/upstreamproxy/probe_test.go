package upstreamproxy

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestProbeSOCKS5Success(t *testing.T) {
	relay := startUDPProbeRelay(t, "udp4", "127.0.0.1:0")
	addr := startProbeServer(t, func(conn net.Conn) {
		defer conn.Close()
		readBytes(t, conn, 3)
		writeBytes(t, conn, []byte{0x05, 0x00})
		request := readBytes(t, conn, 10)
		if binary.BigEndian.Uint16(request[8:10]) == 0 {
			t.Error("UDP ASSOCIATE should announce the bound client port")
		}
		writeBytes(t, conn, buildUDPAssociateResponse(relay))
	})

	result, err := ProbeSOCKS5(context.Background(), ProbeConfig{
		ProxyAddr:       addr,
		Timeout:         2 * time.Second,
		UDPProbeTargets: []string{"192.0.2.53:53"},
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if !result.OK() {
		t.Fatalf("expected success result, got %+v", result)
	}
	if result.Stage != ProbeStageOK {
		t.Fatalf("stage mismatch: got=%q want=%q", result.Stage, ProbeStageOK)
	}
	if !result.UDPRelayOK || result.UDPProbeTarget != "192.0.2.53:53" {
		t.Fatalf("UDP relay evidence missing: %+v", result)
	}
}

func TestProbeSOCKS5HandshakeEOF(t *testing.T) {
	addr := startProbeServer(t, func(conn net.Conn) {
		_ = conn.Close()
	})

	result, err := ProbeSOCKS5(context.Background(), ProbeConfig{
		ProxyAddr: addr,
		Timeout:   2 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected handshake error, got success: %+v", result)
	}
	if result.Stage != ProbeStageHandshake {
		t.Fatalf("stage mismatch: got=%q want=%q", result.Stage, ProbeStageHandshake)
	}
	if result.HandshakeOK {
		t.Fatalf("handshake should not be marked ok: %+v", result)
	}
}

func TestProbeSOCKS5UDPAssociateEOF(t *testing.T) {
	addr := startProbeServer(t, func(conn net.Conn) {
		defer conn.Close()
		readBytes(t, conn, 3)
		writeBytes(t, conn, []byte{0x05, 0x00})
		readBytes(t, conn, 10)
	})

	result, err := ProbeSOCKS5(context.Background(), ProbeConfig{
		ProxyAddr: addr,
		Timeout:   2 * time.Second,
	})
	if err == nil {
		t.Fatalf("expected udp associate error, got success: %+v", result)
	}
	if result.Stage != ProbeStageUDPAssociate {
		t.Fatalf("stage mismatch: got=%q want=%q", result.Stage, ProbeStageUDPAssociate)
	}
	if !result.HandshakeOK {
		t.Fatalf("handshake should be marked ok: %+v", result)
	}
	if result.UDPAssociateOK {
		t.Fatalf("udp associate should not be marked ok: %+v", result)
	}
}

func TestProbeSOCKS5UsesIPv6UDPAssociateForIPv6Proxy(t *testing.T) {
	atypCh := make(chan byte, 1)
	relay := startUDPProbeRelay(t, "udp6", "[::1]:0")
	addr := startProbeServerOn(t, "tcp6", "[::1]:0", func(conn net.Conn) {
		defer conn.Close()
		readBytes(t, conn, 3)
		writeBytes(t, conn, []byte{0x05, 0x00})

		header := readBytes(t, conn, 4)
		atypCh <- header[3]
		switch header[3] {
		case socks5AtypIPv4:
			readBytes(t, conn, 6)
		case socks5AtypIPv6:
			readBytes(t, conn, 18)
		default:
			t.Fatalf("unexpected UDP ASSOCIATE ATYP: 0x%02x", header[3])
		}

		writeBytes(t, conn, buildUDPAssociateResponse(relay))
	})

	result, err := ProbeSOCKS5(context.Background(), ProbeConfig{
		ProxyAddr:       addr,
		Timeout:         2 * time.Second,
		UDPProbeTargets: []string{"192.0.2.53:53"},
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}
	if !result.OK() {
		t.Fatalf("expected success result, got %+v", result)
	}

	select {
	case atyp := <-atypCh:
		if atyp != socks5AtypIPv6 {
			t.Fatalf("UDP ASSOCIATE ATYP = 0x%02x, want IPv6 0x%02x", atyp, socks5AtypIPv6)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for UDP ASSOCIATE request")
	}
}

func TestProbeSOCKS5RejectsAssociateOnlyProxy(t *testing.T) {
	relay, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatalf("listen UDP relay failed: %v", err)
	}
	t.Cleanup(func() { _ = relay.Close() })

	addr := startProbeServer(t, func(conn net.Conn) {
		defer conn.Close()
		readBytes(t, conn, 3)
		writeBytes(t, conn, []byte{0x05, 0x00})
		readBytes(t, conn, 10)
		writeBytes(t, conn, buildUDPAssociateResponse(relay.LocalAddr().(*net.UDPAddr)))
	})

	result, probeErr := ProbeSOCKS5(context.Background(), ProbeConfig{
		ProxyAddr:       addr,
		Timeout:         150 * time.Millisecond,
		UDPProbeTargets: []string{"192.0.2.53:53"},
	})
	if probeErr == nil {
		t.Fatalf("expected UDP data timeout, got success: %+v", result)
	}
	if result.Stage != ProbeStageUDPRelay {
		t.Fatalf("stage mismatch: got=%q want=%q", result.Stage, ProbeStageUDPRelay)
	}
	if !result.UDPAssociationOK() || result.UDPRelayOK || result.OK() {
		t.Fatalf("associate-only proxy must not pass: %+v", result)
	}
	if result.Diagnosis != "代理接受 UDP Associate，但公共 DNS UDP 数据无法完成往返" {
		t.Fatalf("unexpected diagnosis: %q", result.Diagnosis)
	}
}

func TestDefaultUDPProbeTargetsCoverBothAddressFamilies(t *testing.T) {
	families := make(map[string]bool)
	for _, target := range defaultUDPProbeTargets() {
		host, _, err := net.SplitHostPort(target)
		if err != nil {
			t.Fatalf("invalid target %q: %v", target, err)
		}
		if net.ParseIP(host).To4() != nil {
			families["IPv4"] = true
		} else {
			families["IPv6"] = true
		}
	}
	if !families["IPv4"] || !families["IPv6"] {
		t.Fatalf("default targets must cover IPv4 and IPv6: %+v", families)
	}
}

func startProbeServer(t *testing.T, handler func(net.Conn)) string {
	return startProbeServerOn(t, "tcp", "127.0.0.1:0", handler)
}

func startProbeServerOn(t *testing.T, network, addr string, handler func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		handler(conn)
	}()

	return ln.Addr().String()
}

func startUDPProbeRelay(t *testing.T, network, address string) *net.UDPAddr {
	t.Helper()
	conn, err := net.ListenUDP(network, mustResolveUDPAddr(t, network, address))
	if err != nil {
		t.Fatalf("listen UDP relay failed: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buffer := make([]byte, 2048)
		length, client, readErr := conn.ReadFromUDP(buffer)
		if readErr != nil {
			return
		}
		target, payload, decodeErr := decodeSOCKS5UDPDatagram(buffer[:length])
		if decodeErr != nil || len(payload) < 4 {
			return
		}
		response := append([]byte(nil), payload...)
		response[2] |= 0x80
		_, _ = conn.WriteToUDP(buildSOCKS5UDPDatagram(target, response), client)
	}()
	return conn.LocalAddr().(*net.UDPAddr)
}

func mustResolveUDPAddr(t *testing.T, network, address string) *net.UDPAddr {
	t.Helper()
	resolved, err := net.ResolveUDPAddr(network, address)
	if err != nil {
		t.Fatalf("resolve UDP address failed: %v", err)
	}
	return resolved
}

func buildUDPAssociateResponse(relay *net.UDPAddr) []byte {
	if v4 := relay.IP.To4(); v4 != nil {
		response := []byte{socks5Version, socks5ReplySuccess, 0, socks5AtypIPv4, 0, 0, 0, 0, 0, 0}
		copy(response[4:8], v4)
		binary.BigEndian.PutUint16(response[8:10], uint16(relay.Port))
		return response
	}

	response := make([]byte, 4+net.IPv6len+2)
	response[0] = socks5Version
	response[1] = socks5ReplySuccess
	response[3] = socks5AtypIPv6
	copy(response[4:20], relay.IP.To16())
	binary.BigEndian.PutUint16(response[20:22], uint16(relay.Port))
	return response
}

func readBytes(t *testing.T, r io.Reader, size int) []byte {
	t.Helper()
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatalf("read failed: %v", err)
	}
	return buf
}

func writeBytes(t *testing.T, w io.Writer, payload []byte) {
	t.Helper()
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}
