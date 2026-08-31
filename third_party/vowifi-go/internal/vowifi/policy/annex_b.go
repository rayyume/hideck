package policy

import (
	"fmt"
	"strings"
)

const (
	MediaRestrictionNone       = ""
	MediaRestrictionAudioOnly  = "audio-only"
	MediaRestrictionAudioVideo = "audio-video"

	AccessWLAN     = "wlan"
	AccessCellular = "cellular"
	AccessEPS      = "eps"
)

// AnnexB holds IR.51 Annex B static UE configuration. These values come from
// preset YAML or API overrides. There is no ANDSF, OMA-DM, or TS.32 client.
type AnnexB struct {
	MediaTypeRestrictionPolicy    string
	PreferredAccessNetworks       []string
	ToConRef                      string
	AllowHandoverPDNWLANAndEPS    bool
	AllowHandoverPDNWLANAndEPSSet bool
	Rejection                     string
}

func normalizeMediaTypeRestriction(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case MediaRestrictionNone, MediaRestrictionAudioOnly, MediaRestrictionAudioVideo:
		return value, nil
	default:
		return "", fmt.Errorf("invalid Media_type_restriction_policy %q", value)
	}
}

func normalizePreferredAccessNetworks(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		switch value {
		case AccessWLAN, AccessCellular, AccessEPS:
		default:
			return nil, fmt.Errorf("invalid PreferredAccessNetworks value %q", raw)
		}
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, nil
}

func applyAnnexB(config *EffectiveCarrierConfig, restriction string, preferred []string, toConRef string, handover *bool) {
	if config == nil {
		return
	}
	config.AnnexBRejection = ""
	if restriction != "" {
		normalized, err := normalizeMediaTypeRestriction(restriction)
		if err != nil {
			config.AnnexBRejection = err.Error()
			return
		}
		config.MediaTypeRestrictionPolicy = normalized
	}
	if len(preferred) > 0 {
		normalized, err := normalizePreferredAccessNetworks(preferred)
		if err != nil {
			config.AnnexBRejection = err.Error()
			return
		}
		config.PreferredAccessNetworks = normalized
	}
	if value := strings.TrimSpace(toConRef); value != "" {
		config.ToConRef = value
	}
	if handover != nil {
		config.AllowHandoverPDNWLANAndEPS = *handover
		config.AllowHandoverPDNWLANAndEPSSet = true
	}
}

// SelectPreferredAccess picks the first preferred access that is currently
// available. Empty preference keeps the current access.
func SelectPreferredAccess(preferred, available []string, current string) string {
	if len(preferred) == 0 {
		return strings.TrimSpace(current)
	}
	present := map[string]bool{}
	for _, item := range available {
		present[strings.ToLower(strings.TrimSpace(item))] = true
	}
	for _, item := range preferred {
		if present[item] {
			return item
		}
	}
	return strings.TrimSpace(current)
}

// AllowsMediaType reports whether the Annex B restriction admits this media.
// Unknown or empty policy admits every type so current behavior stays put.
func AllowsMediaType(restriction, media string) bool {
	restriction, err := normalizeMediaTypeRestriction(restriction)
	if err != nil || restriction == MediaRestrictionNone {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(media)) {
	case "audio":
		return true
	case "video":
		return restriction == MediaRestrictionAudioVideo
	default:
		return restriction == MediaRestrictionAudioVideo
	}
}

// AllowsWLANToEPSHandover reports the Annex B handover flag. Unset means the
// current implementation does not start a PDN handover.
func (config EffectiveCarrierConfig) AllowsWLANToEPSHandover() bool {
	return config.AllowHandoverPDNWLANAndEPSSet && config.AllowHandoverPDNWLANAndEPS
}
