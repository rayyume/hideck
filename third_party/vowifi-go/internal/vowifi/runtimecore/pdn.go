package runtimecore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/engine/ipsec"
	"github.com/iniwex5/vowifi-go/engine/swu"
	vowifidns "github.com/iniwex5/vowifi-go/internal/vowifi/dns"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/netstack"
	"github.com/iniwex5/vowifi-go/internal/vowifi/policy"
)

type pdnStarter interface {
	StartSlot(context.Context, string, string, *swu.Config) (*swu.Session, error)
	StopSlot(string, string) error
	GetSlot(string, string) (*swu.Session, bool)
}

type pdnWaiter interface {
	WaitSlot(context.Context, string, string, time.Duration) (swu.SessionSnapshot, error)
}

// StartAdditionalPDNs opens extra SWu sessions for the same device on the
// same ePDG. A failed extra PDN is stopped without touching the default IMS
// session. No extra APN means this is a no-op.
func StartAdditionalPDNs(
	ctx context.Context,
	manager pdnStarter,
	deviceID string,
	base *swu.Config,
	config policy.EffectiveCarrierConfig,
) error {
	if manager == nil || base == nil {
		return nil
	}
	var first error
	for _, spec := range policy.AdditionalPDNs(config) {
		cfg := cloneSWUConfigForPDN(base, spec.APN)
		if _, err := manager.StartSlot(ctx, deviceID, spec.Slot, cfg); err != nil {
			_ = manager.StopSlot(deviceID, spec.Slot)
			if first == nil {
				first = fmt.Errorf("runtimecore: start PDN %s: %w", spec.Slot, err)
			}
			continue
		}
		if waiter, ok := manager.(pdnWaiter); ok {
			snapshot, err := waiter.WaitSlot(ctx, deviceID, spec.Slot, epdgEstablishmentTimeout)
			if err != nil || !snapshot.Established {
				_ = manager.StopSlot(deviceID, spec.Slot)
				if first == nil {
					if err == nil {
						err = fmt.Errorf("runtimecore: PDN %s did not establish", spec.Slot)
					}
					first = fmt.Errorf("runtimecore: wait PDN %s: %w", spec.Slot, err)
				}
			}
		}
	}
	return first
}

func attachAdditionalPDNs(ctx context.Context, cfg SessionConfig, result *SessionResult) {
	if result == nil || result.EPDGMgr == nil {
		return
	}
	carrier := policy.EffectiveCarrierConfigFromCarrierPlan(cfg.Prepared.CarrierPlan)
	base := BuildSWUConfig(cfg)
	if strings.TrimSpace(carrier.APN) == "" {
		carrier.APN = strings.TrimSpace(base.APN)
	}
	extra := policy.AdditionalPDNs(carrier)
	if len(extra) == 0 {
		return
	}
	result.XCAPRequired = true
	if err := StartAdditionalPDNs(ctx, result.EPDGMgr, cfg.DeviceID, base, carrier); err != nil {
		logging.Info("XCAP PDN start failed; IMS session kept", "device", cfg.DeviceID, "err", err)
		return
	}
	session, ok := result.EPDGMgr.GetSlot(cfg.DeviceID, policy.XCAPSessionSlot)
	if !ok || session == nil {
		return
	}
	result.XCAPSession = session
	snapshot, _ := result.EPDGMgr.SnapshotSlot(cfg.DeviceID, policy.XCAPSessionSlot)
	network, err := NewUserspaceIMSNetwork(ctx, session, snapshot)
	if err != nil {
		logging.Info("XCAP PDN netstack unavailable", "device", cfg.DeviceID, "err", err)
		return
	}
	result.XCAPNetwork = network
	logging.Info("XCAP PDN established", "device", cfg.DeviceID, "ipv4", snapshot.IPv4 != nil, "ipv6", snapshot.IPv6 != nil)
}

var publicXCAPResolvers = []net.IP{
	net.ParseIP("1.1.1.1"),
	net.ParseIP("8.8.8.8"),
}

var lookupXCAPHost = func(ctx context.Context, host string) []net.IP {
	return vowifidns.LookupHostIPViaDNSServers(ctx, host, false, nil, publicXCAPResolvers)
}

// XCAPDialContext returns a dialer for Ut/XCAP. A distinct XCAP APN stays on
// that PDN. Otherwise a pub.3gppnetwork.org name that resolves to RFC1918
// is dialed on the IMS SWu stack; a public address uses the country SOCKS
// CONNECT so DNS and TCP leave from the home network.
func (r *SessionResult) XCAPDialContext() func(context.Context, string, string) (net.Conn, error) {
	if r == nil {
		return nil
	}
	if r.XCAPRequired {
		inner := r.innerRawDialContext()
		if inner == nil {
			return nil
		}
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialXCAP(ctx, network, address, inner, nil)
		}
	}
	inner := r.innerRawDialContext()
	socks := socksXCAPDialContext(r.Proxy)
	if inner == nil && socks == nil {
		return nil
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialXCAP(ctx, network, address, inner, socks)
	}
}

func (r *SessionResult) innerRawDialContext() func(context.Context, string, string) (net.Conn, error) {
	if r == nil {
		return nil
	}
	network := r.XCAPNetwork
	if network == nil && !r.XCAPRequired {
		network = r.IMSNetwork
	}
	if network == nil {
		return nil
	}
	adapter := netstack.AdaptIMSNetwork(network)
	if adapter == nil {
		return nil
	}
	return adapter.DialContext
}

func (r *SessionResult) innerXCAPDialContext() func(context.Context, string, string) (net.Conn, error) {
	return withPublicHostLookup(r.innerRawDialContext())
}

const xcapInnerDialTimeout = 5 * time.Second

func dialXCAP(
	ctx context.Context,
	network, address string,
	inner func(context.Context, string, string) (net.Conn, error),
	socks func(context.Context, string, string) (net.Conn, error),
) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	host = strings.Trim(host, "[]")
	if inner != nil && net.ParseIP(host) == nil {
		conn, innerErr := dialXCAPWithTimeout(ctx, xcapInnerDialTimeout, inner, network, address)
		if innerErr == nil {
			logging.Info("XCAP dialed via IMS DNS", "host", host, "port", port)
			return conn, nil
		}
		logging.Info("XCAP IMS hostname dial failed", "host", host, "port", port, "err", innerErr)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		if ips := lookupXCAPHost(ctx, host); len(ips) > 0 {
			ip = ips[0]
		}
	}
	if ip != nil && ip.IsPrivate() {
		if inner == nil {
			return nil, fmt.Errorf("netstack: private XCAP address %s needs IMS tunnel", ip)
		}
		conn, innerErr := dialXCAPWithTimeout(ctx, xcapInnerDialTimeout, inner, network, net.JoinHostPort(ip.String(), port))
		if innerErr != nil {
			logging.Info("XCAP IMS private dial failed", "ip", ip.String(), "port", port, "err", innerErr)
			return nil, innerErr
		}
		logging.Info("XCAP dialed via IMS private address", "ip", ip.String(), "port", port)
		return conn, nil
	}
	if socks != nil {
		return socks(ctx, network, address)
	}
	if inner == nil {
		return nil, errUtDialUnavailable
	}
	if ip != nil {
		return inner(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	return inner(ctx, network, address)
}

func dialXCAPWithTimeout(
	ctx context.Context,
	timeout time.Duration,
	dial func(context.Context, string, string) (net.Conn, error),
	network, address string,
) (net.Conn, error) {
	if dial == nil {
		return nil, errUtDialUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return dial(ctx, network, address)
}

var errUtDialUnavailable = errors.New("XCAP PDN is not established")

func socksXCAPDialContext(proxy *ProxyConfig) func(context.Context, string, string) (net.Conn, error) {
	if proxy == nil || !proxy.Enabled || strings.TrimSpace(proxy.Addr) == "" {
		return nil
	}
	cfg := ipsec.Socks5Config{
		ProxyAddr: strings.TrimSpace(proxy.Addr),
		Username:  proxy.Username,
		Password:  proxy.Password,
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return ipsec.DialSOCKS5(ctx, cfg, network, address)
	}
}

func withPublicHostLookup(
	inner func(context.Context, string, string) (net.Conn, error),
) func(context.Context, string, string) (net.Conn, error) {
	if inner == nil {
		return nil
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		host = strings.Trim(host, "[]")
		if net.ParseIP(host) != nil {
			return inner(ctx, network, address)
		}
		conn, err := inner(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		ips := lookupXCAPHost(ctx, host)
		if len(ips) == 0 {
			return nil, err
		}
		last := err
		for _, ip := range ips {
			if ip == nil {
				continue
			}
			conn, dialErr := inner(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			last = dialErr
		}
		return nil, last
	}
}

func cloneSWUConfigForPDN(base *swu.Config, apn string) *swu.Config {
	if base == nil {
		return &swu.Config{APN: strings.TrimSpace(apn), OmitInitialContact: true}
	}
	cfg := *base
	cfg.APN = strings.TrimSpace(apn)
	cfg.TUNName = ""
	cfg.LocalPort = 0
	cfg.OmitInitialContact = true
	return &cfg
}
