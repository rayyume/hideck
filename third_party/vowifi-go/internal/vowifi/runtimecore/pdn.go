package runtimecore

import (
	"context"
	"fmt"
	"strings"

	"github.com/iniwex5/vowifi-go/engine/swu"
	"github.com/iniwex5/vowifi-go/internal/vowifi/policy"
)

type pdnStarter interface {
	StartSlot(context.Context, string, string, *swu.Config) (*swu.Session, error)
	StopSlot(string, string) error
	GetSlot(string, string) (*swu.Session, bool)
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
		}
	}
	return first
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
