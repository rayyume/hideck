package runtimecore

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/engine/swu"
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
}

// XCAPDialContext returns a dialer on the XCAP PDN. If a distinct XCAP APN
// was requested, the IMS inner network is not used as a fallback.
func (r *SessionResult) XCAPDialContext() func(context.Context, string, string) (net.Conn, error) {
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

func cloneSWUConfigForPDN(base *swu.Config, apn string) *swu.Config {
	if base == nil {
		return &swu.Config{APN: strings.TrimSpace(apn)}
	}
	cfg := *base
	cfg.APN = strings.TrimSpace(apn)
	cfg.TUNName = ""
	return &cfg
}
