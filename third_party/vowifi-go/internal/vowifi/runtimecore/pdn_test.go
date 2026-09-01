package runtimecore

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/engine/swu"
	"github.com/iniwex5/vowifi-go/internal/vowifi/netstack"
	"github.com/iniwex5/vowifi-go/internal/vowifi/policy"
	"github.com/iniwex5/vowifi-go/internal/vowifi/profile"
)

type recordingPDNStarter struct {
	started  []string
	stopped  []string
	waited   []string
	fail     string
	failWait string
	configs  []*swu.Config
}

func (r *recordingPDNStarter) StartSlot(_ context.Context, _, slot string, cfg *swu.Config) (*swu.Session, error) {
	r.started = append(r.started, slot+":"+cfg.APN)
	r.configs = append(r.configs, cfg)
	if slot == r.fail {
		return nil, errors.New("xcap tunnel down")
	}
	return swu.NewSession(cfg), nil
}

func (r *recordingPDNStarter) StopSlot(_, slot string) error {
	r.stopped = append(r.stopped, slot)
	return nil
}

func (r *recordingPDNStarter) GetSlot(string, string) (*swu.Session, bool) {
	return nil, false
}

func (r *recordingPDNStarter) WaitSlot(_ context.Context, _, slot string, _ time.Duration) (swu.SessionSnapshot, error) {
	r.waited = append(r.waited, slot)
	if slot == r.failWait {
		return swu.SessionSnapshot{}, errors.New("xcap wait failed")
	}
	return swu.SessionSnapshot{Established: true, IPv4: net.IPv4(10, 0, 0, 2)}, nil
}

func TestCloneSWUConfigForPDNOmitsInitialContact(t *testing.T) {
	base := &swu.Config{APN: "ims", TUNName: "ims0", LocalPort: 500, OmitInitialContact: false}
	cfg := cloneSWUConfigForPDN(base, "xcap")
	if cfg.APN != "xcap" || cfg.TUNName != "" || cfg.LocalPort != 0 || !cfg.OmitInitialContact {
		t.Fatalf("xcap clone = %+v", cfg)
	}
	if base.APN != "ims" || base.TUNName != "ims0" || base.LocalPort != 500 || base.OmitInitialContact {
		t.Fatalf("base mutated = %+v", base)
	}
	empty := cloneSWUConfigForPDN(nil, "xcap")
	if empty.APN != "xcap" || !empty.OmitInitialContact {
		t.Fatalf("nil base clone = %+v", empty)
	}
}

func TestStartAdditionalPDNsNoopsForSingleAPN(t *testing.T) {
	starter := &recordingPDNStarter{}
	cfg := &swu.Config{APN: "ims", EPDGAddr: "epdg.example"}
	if err := StartAdditionalPDNs(context.Background(), starter, "dev-1", cfg, policy.EffectiveCarrierConfig{APN: "ims"}); err != nil {
		t.Fatalf("StartAdditionalPDNs: %v", err)
	}
	if len(starter.started) != 0 {
		t.Fatalf("started = %v", starter.started)
	}
}

func TestStartAdditionalPDNsReusesEPDGAndIsolatesFailure(t *testing.T) {
	starter := &recordingPDNStarter{fail: policy.XCAPSessionSlot}
	base := &swu.Config{APN: "ims", EPDGAddr: "epdg.example"}
	err := StartAdditionalPDNs(context.Background(), starter, "dev-1", base, policy.EffectiveCarrierConfig{
		APN: "ims", XCAPAPN: "xcap",
	})
	if err == nil {
		t.Fatal("expected XCAP start error")
	}
	if len(starter.started) != 1 || starter.started[0] != "xcap:xcap" {
		t.Fatalf("started = %v", starter.started)
	}
	if len(starter.stopped) != 1 || starter.stopped[0] != policy.XCAPSessionSlot {
		t.Fatalf("stopped = %v", starter.stopped)
	}
	if base.APN != "ims" {
		t.Fatalf("base APN mutated = %q", base.APN)
	}
	if len(starter.configs) != 1 || !starter.configs[0].OmitInitialContact {
		t.Fatalf("xcap IKE still advertised INITIAL_CONTACT")
	}
}

func TestStartAdditionalPDNsWaitsThenStopsFailedSlot(t *testing.T) {
	starter := &recordingPDNStarter{failWait: policy.XCAPSessionSlot}
	base := &swu.Config{APN: "ims", EPDGAddr: "epdg.example"}
	err := StartAdditionalPDNs(context.Background(), starter, "dev-1", base, policy.EffectiveCarrierConfig{
		APN: "ims", XCAPAPN: "xcap",
	})
	if err == nil {
		t.Fatal("expected wait error")
	}
	if len(starter.waited) != 1 || starter.waited[0] != policy.XCAPSessionSlot {
		t.Fatalf("waited = %v", starter.waited)
	}
	if len(starter.stopped) != 1 || starter.stopped[0] != policy.XCAPSessionSlot {
		t.Fatalf("stopped = %v", starter.stopped)
	}
}

func TestXCAPDialContextDoesNotFallBackWhenXCAPRequired(t *testing.T) {
	result := &SessionResult{XCAPRequired: true, IMSNetwork: &netstack.Network{}}
	if result.XCAPDialContext() != nil {
		t.Fatal("distinct XCAP APN must not dial the IMS PDN")
	}
}

func TestXCAPDialContextUsesIMSNetworkWhenNoExtraAPN(t *testing.T) {
	result := &SessionResult{}
	if result.XCAPDialContext() != nil {
		t.Fatal("missing IMS network should yield no dialer")
	}
}

func TestXCAPDialContextUsesCountryProxy(t *testing.T) {
	result := &SessionResult{
		Proxy: &ProxyConfig{Enabled: true, Addr: "127.0.0.1:1080"},
	}
	if result.XCAPDialContext() == nil {
		t.Fatal("country proxy should dial XCAP")
	}
}

func TestDialXCAPUsesIMSHostnameBeforePublicLookup(t *testing.T) {
	var innerAddr, socksAddr string
	inner := func(_ context.Context, _, address string) (net.Conn, error) {
		innerAddr = address
		return nil, nil
	}
	socks := func(_ context.Context, _, address string) (net.Conn, error) {
		socksAddr = address
		return nil, errors.New("socks should not run")
	}
	host := "xcap.ims.mnc015.mcc234.3gppnetwork.org:443"
	if _, err := dialXCAP(context.Background(), "tcp", host, inner, socks); err != nil {
		t.Fatalf("dialXCAP: %v", err)
	}
	if innerAddr != host || socksAddr != "" {
		t.Fatalf("inner=%q socks=%q", innerAddr, socksAddr)
	}
}

func TestDialXCAPUsesIMSForPrivateAddress(t *testing.T) {
	original := lookupXCAPHost
	lookupXCAPHost = func(context.Context, string) []net.IP {
		return []net.IP{net.IPv4(10, 127, 164, 9)}
	}
	defer func() { lookupXCAPHost = original }()

	var seen []string
	inner := func(_ context.Context, _, address string) (net.Conn, error) {
		seen = append(seen, address)
		if strings.HasPrefix(address, "10.127.164.9:") {
			return nil, nil
		}
		return nil, errors.New("netstack: no DNS servers assigned by ePDG")
	}
	socks := func(_ context.Context, _, address string) (net.Conn, error) {
		return nil, errors.New("socks should not dial RFC1918")
	}
	if _, err := dialXCAP(context.Background(), "tcp", "xcap.ims.mnc015.mcc234.pub.3gppnetwork.org:443", inner, socks); err != nil {
		t.Fatalf("dialXCAP: %v", err)
	}
	if len(seen) != 2 || seen[0] != "xcap.ims.mnc015.mcc234.pub.3gppnetwork.org:443" || seen[1] != "10.127.164.9:443" {
		t.Fatalf("seen = %v", seen)
	}
}

func TestXCAPDialContextDoesNotUseProxyWhenXCAPAPNRequired(t *testing.T) {
	result := &SessionResult{
		XCAPRequired: true,
		IMSNetwork:   &netstack.Network{},
		Proxy:        &ProxyConfig{Enabled: true, Addr: "127.0.0.1:1080"},
	}
	if result.XCAPDialContext() != nil {
		t.Fatal("distinct XCAP APN must not fall back to SOCKS or IMS")
	}
}

func TestWithPublicHostLookupDialsResolvedIPAfterInnerDNSMiss(t *testing.T) {
	original := lookupXCAPHost
	lookupXCAPHost = func(context.Context, string) []net.IP {
		return []net.IP{net.IPv4(10, 127, 164, 9)}
	}
	defer func() { lookupXCAPHost = original }()

	var seen []string
	inner := func(_ context.Context, _, address string) (net.Conn, error) {
		seen = append(seen, address)
		if strings.HasPrefix(address, "10.127.164.9:") {
			return nil, nil
		}
		return nil, errors.New("netstack: no DNS servers assigned by ePDG")
	}
	conn, err := withPublicHostLookup(inner)(context.Background(), "tcp", "xcap.ims.mnc015.mcc234.pub.3gppnetwork.org:443")
	if err != nil || conn != nil {
		t.Fatalf("dial = %v, %v", conn, err)
	}
	if len(seen) != 2 || seen[0] != "xcap.ims.mnc015.mcc234.pub.3gppnetwork.org:443" || seen[1] != "10.127.164.9:443" {
		t.Fatalf("seen = %v", seen)
	}
}

func TestAttachAdditionalPDNsNoopsWithoutManager(t *testing.T) {
	result := &SessionResult{}
	attachAdditionalPDNs(context.Background(), SessionConfig{
		Prepared: profile.PreparedSession{
			CarrierPlan: policy.CarrierPlan{EPDG: policy.EPDGPlan{APN: "ims", XCAPAPN: "xcap"}},
		},
	}, result)
	if result.XCAPRequired {
		t.Fatal("nil ePDG manager must not start a second PDN")
	}
}
