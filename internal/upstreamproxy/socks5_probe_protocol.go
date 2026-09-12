package upstreamproxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
)

const (
	socks5Version          = 0x05
	socks5AuthNone         = 0x00
	socks5AuthUserPassword = 0x02
	socks5AuthNoAcceptable = 0xFF
	socks5UserPassVersion  = 0x01
	socks5CmdUDPAssociate  = 0x03
	socks5AtypIPv4         = 0x01
	socks5AtypDomain       = 0x03
	socks5AtypIPv6         = 0x04
	socks5ReplySuccess     = 0x00
)

func probeHandshake(conn io.ReadWriter, username, password string) (byte, error) {
	methods := []byte{socks5AuthNone}
	if strings.TrimSpace(username) != "" {
		methods = []byte{socks5AuthUserPassword, socks5AuthNone}
	}

	req := make([]byte, 2+len(methods))
	req[0] = socks5Version
	req[1] = byte(len(methods))
	copy(req[2:], methods)
	if _, err := conn.Write(req); err != nil {
		return 0, fmt.Errorf("socks5 握手发送失败: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return 0, fmt.Errorf("socks5 握手响应读取失败: %w", err)
	}
	if resp[0] != socks5Version {
		return 0, fmt.Errorf("socks5 版本不匹配: 期望 0x05, 实际 0x%02x", resp[0])
	}
	return finishProbeHandshake(conn, username, password, resp[1])
}

func finishProbeHandshake(conn io.ReadWriter, username, password string, method byte) (byte, error) {
	switch method {
	case socks5AuthNone:
		return method, nil
	case socks5AuthUserPassword:
		if strings.TrimSpace(username) == "" {
			return 0, errors.New("socks5 服务器要求用户名密码鉴权但未提供凭据")
		}
		if err := probeUserPasswordAuth(conn, username, password); err != nil {
			return 0, err
		}
		return method, nil
	case socks5AuthNoAcceptable:
		return 0, errors.New("socks5 服务器拒绝了所有鉴权方法 (0xFF)")
	default:
		return 0, fmt.Errorf("socks5 服务器选择了不支持的鉴权方法: 0x%02x", method)
	}
}

func probeUserPasswordAuth(conn io.ReadWriter, username, password string) error {
	if len(username) > 255 || len(password) > 255 {
		return errors.New("socks5 用户名或密码过长 (>255 字节)")
	}

	req := make([]byte, 1+1+len(username)+1+len(password))
	req[0] = socks5UserPassVersion
	req[1] = byte(len(username))
	copy(req[2:2+len(username)], username)
	req[2+len(username)] = byte(len(password))
	copy(req[3+len(username):], password)

	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("socks5 鉴权请求发送失败: %w", err)
	}

	resp := make([]byte, 2)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return fmt.Errorf("socks5 鉴权响应读取失败: %w", err)
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("socks5 鉴权失败: 状态码 0x%02x", resp[1])
	}
	return nil
}

func probeUDPAssociate(ctx context.Context, conn net.Conn, clientAddr *net.UDPAddr) (*net.UDPAddr, error) {
	if _, err := conn.Write(buildProbeUDPAssociateRequest(clientAddr)); err != nil {
		return nil, fmt.Errorf("socks5 UDP ASSOCIATE 请求发送失败: %w", err)
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("socks5 UDP ASSOCIATE 响应解析失败: 读取响应头失败: %w", err)
	}
	if header[0] != socks5Version {
		return nil, fmt.Errorf("socks5 UDP ASSOCIATE 响应版本不匹配: 0x%02x", header[0])
	}
	if header[1] != socks5ReplySuccess {
		return nil, fmt.Errorf("socks5 UDP ASSOCIATE 被拒绝: 状态码 0x%02x", header[1])
	}

	host, err := readProbeReplyHost(conn, header[3])
	if err != nil {
		return nil, fmt.Errorf("socks5 UDP ASSOCIATE 响应解析失败: %w", err)
	}
	port, err := readProbeReplyPort(conn)
	if err != nil {
		return nil, err
	}
	return resolveProbeRelay(ctx, conn, clientAddr, host, port)
}

func buildProbeUDPAssociateRequest(clientAddr *net.UDPAddr) []byte {
	ip := net.IPv4zero
	port := 0
	if clientAddr != nil {
		ip = clientAddr.IP
		port = clientAddr.Port
	}
	if v4 := ip.To4(); v4 != nil {
		req := []byte{socks5Version, socks5CmdUDPAssociate, 0, socks5AtypIPv4, v4[0], v4[1], v4[2], v4[3], 0, 0}
		binary.BigEndian.PutUint16(req[8:], uint16(port))
		return req
	}

	v6 := ip.To16()
	if v6 == nil {
		v6 = net.IPv6zero
	}
	req := make([]byte, 4+net.IPv6len+2)
	req[0], req[1], req[3] = socks5Version, socks5CmdUDPAssociate, socks5AtypIPv6
	copy(req[4:20], v6)
	binary.BigEndian.PutUint16(req[20:], uint16(port))
	return req
}

func readProbeReplyHost(r io.Reader, atyp byte) (string, error) {
	switch atyp {
	case socks5AtypIPv4:
		return readProbeReplyIPHost(r, net.IPv4len, "IPv4")
	case socks5AtypIPv6:
		return readProbeReplyIPHost(r, net.IPv6len, "IPv6")
	case socks5AtypDomain:
		return readProbeReplyDomain(r)
	default:
		return "", fmt.Errorf("未知地址类型: 0x%02x", atyp)
	}
}

func readProbeReplyIPHost(r io.Reader, size int, label string) (string, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("读取 %s 地址失败: %w", label, err)
	}
	return net.IP(buf).String(), nil
}

func readProbeReplyDomain(r io.Reader) (string, error) {
	length := make([]byte, 1)
	if _, err := io.ReadFull(r, length); err != nil {
		return "", fmt.Errorf("读取域名长度失败: %w", err)
	}
	buf := make([]byte, length[0])
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("读取域名失败: %w", err)
	}
	return string(buf), nil
}

func readProbeReplyPort(r io.Reader) (int, error) {
	buf := make([]byte, 2)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, fmt.Errorf("socks5 UDP ASSOCIATE 响应解析失败: 读取端口失败: %w", err)
	}
	return int(binary.BigEndian.Uint16(buf)), nil
}

func socks5AuthMethodName(method byte) string {
	switch method {
	case socks5AuthNone:
		return "noauth"
	case socks5AuthUserPassword:
		return "username_password"
	default:
		return fmt.Sprintf("0x%02x", method)
	}
}
