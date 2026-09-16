package upstreamproxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	udpProbeDNSQueryIDBase = 0x4844
	udpProbeDNSPort        = 53
)

type udpProbeRequest struct {
	id     uint16
	target *net.UDPAddr
}

func openProbeUDPClient(control net.Conn) (*net.UDPConn, error) {
	tcpLocal, ok := control.LocalAddr().(*net.TCPAddr)
	if !ok || tcpLocal.IP == nil {
		return nil, errors.New("创建 UDP 探测套接字失败: SOCKS5 控制连接没有本地 IP")
	}

	network := "udp6"
	if tcpLocal.IP.To4() != nil {
		network = "udp4"
	}
	conn, err := net.ListenUDP(network, &net.UDPAddr{IP: tcpLocal.IP, Zone: tcpLocal.Zone})
	if err != nil {
		return nil, fmt.Errorf("创建 UDP 探测套接字失败: %w", err)
	}
	return conn, nil
}

func resolveProbeRelay(
	ctx context.Context,
	control net.Conn,
	clientAddr *net.UDPAddr,
	host string,
	port int,
) (*net.UDPAddr, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("socks5 UDP ASSOCIATE 返回无效 relay 端口: %d", port)
	}

	ip := net.ParseIP(strings.TrimSpace(host))
	zone := ""
	if ip != nil && ip.IsUnspecified() {
		remote, ok := control.RemoteAddr().(*net.TCPAddr)
		if !ok || remote.IP == nil {
			return nil, errors.New("socks5 UDP ASSOCIATE 返回未指定 relay 地址，且无法取得代理 IP")
		}
		ip = remote.IP
		zone = remote.Zone
	}
	if ip != nil {
		return &net.UDPAddr{IP: ip, Port: port, Zone: zone}, nil
	}

	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, strings.TrimSpace(host))
	if err != nil {
		return nil, fmt.Errorf("解析 SOCKS5 UDP relay 域名 %q 失败: %w", host, err)
	}
	wantIPv4 := clientAddr != nil && clientAddr.IP.To4() != nil
	for _, address := range addresses {
		if (address.IP.To4() != nil) == wantIPv4 {
			return &net.UDPAddr{IP: address.IP, Port: port, Zone: address.Zone}, nil
		}
	}
	return nil, fmt.Errorf("SOCKS5 UDP relay 域名 %q 没有匹配控制连接地址族的地址", host)
}

func probeUDPRelay(
	ctx context.Context,
	conn *net.UDPConn,
	relay *net.UDPAddr,
	targets []string,
	deadline time.Time,
) (string, error) {
	requests, err := prepareUDPProbeRequests(ctx, targets)
	if err != nil {
		return "", err
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("设置 UDP 数据探测超时失败: %w", err)
	}

	sent, err := sendUDPProbeRequests(conn, relay, requests)
	if err != nil {
		return "", err
	}

	buffer := make([]byte, 4096)
	var lastInvalidResponse error
	for {
		length, source, readErr := conn.ReadFromUDP(buffer)
		if readErr != nil {
			if lastInvalidResponse != nil {
				return "", fmt.Errorf("SOCKS5 UDP relay 返回无效探测响应: %w", lastInvalidResponse)
			}
			return "", udpProbeReadError(ctx, readErr)
		}
		if !sameUDPAddr(source, relay) {
			continue
		}
		target, matchErr := matchUDPProbeResponse(buffer[:length], sent)
		if matchErr == nil {
			return target, nil
		}
		lastInvalidResponse = matchErr
	}
}

func sendUDPProbeRequests(
	conn *net.UDPConn,
	relay *net.UDPAddr,
	requests []udpProbeRequest,
) (map[uint16]*net.UDPAddr, error) {
	sent := make(map[uint16]*net.UDPAddr, len(requests))
	var lastWriteError error
	for _, request := range requests {
		packet := buildSOCKS5UDPDatagram(request.target, buildDNSProbeQuery(request.id))
		if _, err := conn.WriteToUDP(packet, relay); err != nil {
			lastWriteError = err
			continue
		}
		sent[request.id] = request.target
	}
	if len(sent) == 0 {
		return nil, fmt.Errorf("SOCKS5 UDP 数据探测发送失败: %w", lastWriteError)
	}
	return sent, nil
}

func matchUDPProbeResponse(
	packet []byte,
	sent map[uint16]*net.UDPAddr,
) (string, error) {
	origin, payload, err := decodeSOCKS5UDPDatagram(packet)
	if err != nil {
		return "", err
	}
	id, valid := validDNSProbeResponse(payload)
	if !valid {
		return "", errors.New("DNS 探测响应缺少匹配的响应标志")
	}
	target, expected := sent[id]
	if !expected {
		return "", fmt.Errorf("DNS 探测响应包含未知事务 ID 0x%04x", id)
	}
	if !sameUDPAddr(origin, target) {
		return "", fmt.Errorf("DNS 探测响应来源 %s 与目标 %s 不一致", origin, target)
	}
	return target.String(), nil
}

func prepareUDPProbeRequests(
	ctx context.Context,
	targets []string,
) ([]udpProbeRequest, error) {
	if len(targets) == 0 {
		targets = defaultUDPProbeTargets()
	}
	requests := make([]udpProbeRequest, 0, len(targets))
	for index, target := range targets {
		address, err := resolveUDPProbeTarget(ctx, target)
		if err != nil {
			return nil, err
		}
		requests = append(requests, udpProbeRequest{
			id:     uint16(udpProbeDNSQueryIDBase + index),
			target: address,
		})
	}
	return requests, nil
}

func defaultUDPProbeTargets() []string {
	return []string{
		"1.1.1.1:53",
		"8.8.8.8:53",
		"9.9.9.9:53",
		"[2606:4700:4700::1111]:53",
		"[2001:4860:4860::8888]:53",
		"[2620:fe::fe]:53",
	}
}

func resolveUDPProbeTarget(ctx context.Context, target string) (*net.UDPAddr, error) {
	host, portText, err := net.SplitHostPort(strings.TrimSpace(target))
	if err != nil {
		return nil, fmt.Errorf("无效的 UDP 探测目标 %q: %w", target, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port != udpProbeDNSPort {
		return nil, fmt.Errorf("UDP 探测目标 %q 必须使用 DNS 端口 53", target)
	}

	ip := net.ParseIP(host)
	if ip != nil {
		return &net.UDPAddr{IP: ip, Port: port}, nil
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("解析 UDP 探测目标 %q 失败: %w", target, err)
	}
	if len(addresses) > 0 {
		return &net.UDPAddr{IP: addresses[0].IP, Port: port, Zone: addresses[0].Zone}, nil
	}
	return nil, fmt.Errorf("UDP 探测目标 %q 没有可用地址", target)
}

func buildSOCKS5UDPDatagram(target *net.UDPAddr, payload []byte) []byte {
	headerLength := 4 + net.IPv6len + 2
	if target.IP.To4() != nil {
		headerLength = 4 + net.IPv4len + 2
	}
	packet := make([]byte, headerLength+len(payload))
	if v4 := target.IP.To4(); v4 != nil {
		packet[3] = socks5AtypIPv4
		copy(packet[4:8], v4)
		binary.BigEndian.PutUint16(packet[8:10], uint16(target.Port))
	} else {
		packet[3] = socks5AtypIPv6
		copy(packet[4:20], target.IP.To16())
		binary.BigEndian.PutUint16(packet[20:22], uint16(target.Port))
	}
	copy(packet[headerLength:], payload)
	return packet
}

func decodeSOCKS5UDPDatagram(packet []byte) (*net.UDPAddr, []byte, error) {
	if len(packet) < 4 || packet[0] != 0 || packet[1] != 0 {
		return nil, nil, errors.New("SOCKS5 UDP relay 返回无效保留字段")
	}
	if packet[2] != 0 {
		return nil, nil, errors.New("SOCKS5 UDP relay 返回了不支持的分片报文")
	}

	addressLength := 0
	switch packet[3] {
	case socks5AtypIPv4:
		addressLength = net.IPv4len
	case socks5AtypIPv6:
		addressLength = net.IPv6len
	default:
		return nil, nil, fmt.Errorf("SOCKS5 UDP relay 返回未知地址类型: 0x%02x", packet[3])
	}
	headerLength := 4 + addressLength + 2
	if len(packet) < headerLength {
		return nil, nil, errors.New("SOCKS5 UDP relay 返回报文过短")
	}
	ip := append(net.IP(nil), packet[4:4+addressLength]...)
	port := int(binary.BigEndian.Uint16(packet[4+addressLength : headerLength]))
	return &net.UDPAddr{IP: ip, Port: port}, packet[headerLength:], nil
}

func buildDNSProbeQuery(id uint16) []byte {
	query := make([]byte, 0, 29)
	header := make([]byte, 12)
	binary.BigEndian.PutUint16(header[0:2], id)
	binary.BigEndian.PutUint16(header[2:4], 0x0100)
	binary.BigEndian.PutUint16(header[4:6], 1)
	query = append(query, header...)
	query = append(query, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0)
	query = append(query, 0, 1, 0, 1)
	return query
}

func validDNSProbeResponse(packet []byte) (uint16, bool) {
	if len(packet) < 12 {
		return 0, false
	}
	return binary.BigEndian.Uint16(packet[0:2]), binary.BigEndian.Uint16(packet[2:4])&0x8000 != 0
}

func udpProbeReadError(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("SOCKS5 UDP 数据探测失败: %w", ctxErr)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("SOCKS5 UDP 数据转发超时: %w", err)
	}
	return fmt.Errorf("读取 SOCKS5 UDP relay 响应失败: %w", err)
}

func sameUDPAddr(left, right *net.UDPAddr) bool {
	return left != nil && right != nil && left.Port == right.Port && left.IP.Equal(right.IP)
}
