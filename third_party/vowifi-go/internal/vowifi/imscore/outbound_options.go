package imscore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emiago/sipgo/sip"
	"github.com/iniwex5/vowifi-go/internal/vowifi/common"
	"github.com/iniwex5/vowifi-go/internal/vowifi/imsheaders"
	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
	"github.com/iniwex5/vowifi-go/internal/vowifi/sipkit"
)

const imsCapabilityDiscoveryFlow = "options_capability_discovery"

var errOPTIONSCapabilityDiscovery = errors.New("imscore: SIP OPTIONS capability discovery")

type peerCapabilitySnapshot struct {
	Allow []string
	ICSI  []string
}

func (s *Service) schedulePeerCapabilityDiscovery() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.peerCapabilityAfter = time.Now().Add(2 * time.Second)
	s.peerCapabilityDone = false
	s.mu.Unlock()
	s.signalIMSMaintenance()
}

func (s *Service) discoverPeerCapabilitiesOnce() {
	s.mu.Lock()
	if s.peerCapabilityDone {
		s.mu.Unlock()
		return
	}
	s.peerCapabilityDone = true
	s.mu.Unlock()
	if err := s.discoverPeerCapabilities(); err != nil {
		logging.WarnRate("ims-options-cap-"+s.DeviceID(), "IMS OPTIONS capability discovery failed",
			"device", s.DeviceID(), "err", err)
	}
}

func (s *Service) discoverPeerCapabilities() error {
	request, err := s.buildCapabilityDiscoveryOPTIONS()
	if err != nil {
		return err
	}
	timeout := s.keepaliveTimeout
	if timeout <= 0 {
		timeout = imsKeepaliveTransactionTimeout
	}
	response, _, err := s.dispatchOutboundRequest(
		context.Background(), imsCapabilityDiscoveryFlow, request, timeout, true,
	)
	if err != nil {
		return fmt.Errorf("%w: %w", errOPTIONSCapabilityDiscovery, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: rejected with status %d", errOPTIONSCapabilityDiscovery, response.StatusCode)
	}
	snapshot := parsePeerCapabilityResponse(
		sipkit.FirstHeaderValue(response, "Allow", true),
		sipkit.FirstHeaderValue(response, "Contact", true),
	)
	s.mu.Lock()
	s.peerAllow = snapshot.Allow
	s.peerICSI = snapshot.ICSI
	s.mu.Unlock()
	logging.Info("IMS peer capabilities discovered",
		"device", s.DeviceID(),
		"allow", strings.Join(snapshot.Allow, ","),
		"icsi", strings.Join(snapshot.ICSI, ","))
	return nil
}

func (s *Service) buildCapabilityDiscoveryOPTIONS() (*sip.Request, error) {
	profile, err := s.reserveRegisteredSIPProfile()
	if err != nil {
		return nil, fmt.Errorf("imscore: capability OPTIONS registered profile: %w", err)
	}
	recipient, err := parseKeepaliveURI("sip:" + profile.RemoteAddress)
	if err != nil {
		return nil, fmt.Errorf("imscore: capability OPTIONS P-CSCF endpoint: %w", err)
	}
	aor, err := parseKeepaliveURI(profile.LocalURI)
	if err != nil {
		return nil, err
	}
	securityMode := "disabled"
	supported := imsKeepaliveSupported
	if strings.TrimSpace(profile.SecurityVerify) != "" {
		securityMode = securityModeIPSec
		supported = imsProtectedKeepaliveSupported
	}
	supported = mergeSIPOptionTags(supported, "precondition")
	return sipkit.BuildIMSRequest(sip.OPTIONS, recipient, sipkit.IMSRequestOptions{
		Destination: profile.RemoteAddress,
		Transport:   profile.Transport, Branch: "z9hG4bK." + common.RandomHex(20),
		FromURI: aor, FromTag: profile.FromTag, ToURI: aor,
		CallID: common.RandomHex(20), CSeq: uint32(profile.InitialCSeq),
		Kind: sipkit.RequestKindOutOfDialog, SecurityMode: securityMode,
		AddRPort:          true,
		AddUserAgent:      strings.TrimSpace(profile.UserAgent) != "",
		AddSupported:      true,
		Supported:         supported,
		PreferredIdentity: imsheaders.PreferredIdentityHeaderValue(profile.LocalURI),
		Runtime: sipkit.IMSRuntimeSnapshot{
			ServiceRoute: profile.ServiceRoute, SecVerify: profile.SecurityVerify,
			PAccessNetworkInfo: profile.PANI, UserAgent: profile.UserAgent,
			LocalAddr: profile.LocalAddress, Transport: profile.Transport,
		},
	})
}

func parsePeerCapabilityResponse(allow, contact string) peerCapabilitySnapshot {
	return peerCapabilitySnapshot{
		Allow: splitSIPListTokens(allow),
		ICSI:  extractICSIs(contact),
	}
}

func splitSIPListTokens(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		key := strings.ToLower(token)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, token)
	}
	return out
}

func extractICSIs(contact string) []string {
	lower := strings.ToLower(contact)
	const marker = "+g.3gpp.icsi-ref="
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return nil
	}
	raw := contact[idx+len(marker):]
	if strings.HasPrefix(raw, `"`) {
		raw = strings.TrimPrefix(raw, `"`)
		if end := strings.Index(raw, `"`); end >= 0 {
			raw = raw[:end]
		}
	} else if end := strings.IndexAny(raw, ";,>"); end >= 0 {
		raw = raw[:end]
	}
	return splitSIPListTokens(raw)
}
