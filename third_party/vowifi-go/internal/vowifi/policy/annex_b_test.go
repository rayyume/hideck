package policy

import "testing"

func TestMergeAnnexBAppliesValidFields(t *testing.T) {
	enabled := true
	config := GetGlobalDefaultConfig("234", "15")
	config.MergeFromPreset(CarrierPreset{
		XCAPAPN:                    "xcap",
		MediaTypeRestrictionPolicy: "audio-only",
		PreferredAccessNetworks:    []string{"wlan", "cellular"},
		ToConRef:                   "ims-pdn",
		AllowHandoverPDNWLANAndEPS: &enabled,
	})
	if config.XCAPAPN != "xcap" || config.MediaTypeRestrictionPolicy != MediaRestrictionAudioOnly {
		t.Fatalf("annex B / XCAP APN = %+v", config)
	}
	if got := SelectPreferredAccess(config.PreferredAccessNetworks, []string{"cellular", "wlan"}, "cellular"); got != AccessWLAN {
		t.Fatalf("preferred access = %q", got)
	}
	if !config.AllowsWLANToEPSHandover() || AllowsMediaType(config.MediaTypeRestrictionPolicy, "video") {
		t.Fatalf("handover/media = %+v", config)
	}
	plan := CarrierPlanFromEffectiveConfig(config)
	round := EffectiveCarrierConfigFromCarrierPlan(plan)
	if round.XCAPAPN != "xcap" || round.ToConRef != "ims-pdn" || !round.AllowHandoverPDNWLANAndEPSSet {
		t.Fatalf("round trip = %+v", round)
	}
}

func TestMergeAnnexBRejectsInvalidValuesWithoutChangingDefaults(t *testing.T) {
	config := GetGlobalDefaultConfig("234", "15")
	config.MergeFromPreset(CarrierPreset{MediaTypeRestrictionPolicy: "srtp-only"})
	if config.MediaTypeRestrictionPolicy != "" || config.AnnexBRejection == "" {
		t.Fatalf("invalid restriction was applied: %+v", config)
	}
	config = GetGlobalDefaultConfig("234", "15")
	config.MergeFromPreset(CarrierPreset{PreferredAccessNetworks: []string{"satellite"}})
	if len(config.PreferredAccessNetworks) != 0 || config.AnnexBRejection == "" {
		t.Fatalf("invalid access list was applied: %+v", config)
	}
}

func TestAdditionalPDNsOnlyWhenXCAPAPNDiffers(t *testing.T) {
	config := GetGlobalDefaultConfig("234", "15")
	if extra := AdditionalPDNs(config); extra != nil {
		t.Fatalf("single APN extra PDNs = %+v", extra)
	}
	config.XCAPAPN = "ims"
	if extra := AdditionalPDNs(config); extra != nil {
		t.Fatalf("same APN extra PDNs = %+v", extra)
	}
	config.XCAPAPN = "xcap"
	extra := AdditionalPDNs(config)
	if len(extra) != 1 || extra[0].Slot != XCAPSessionSlot || extra[0].APN != "xcap" {
		t.Fatalf("XCAP PDN = %+v", extra)
	}
}

func TestSelectPreferredAccessKeepsCurrentWhenNoneMatch(t *testing.T) {
	got := SelectPreferredAccess([]string{AccessWLAN}, []string{AccessCellular}, AccessCellular)
	if got != AccessCellular {
		t.Fatalf("got %q", got)
	}
}
