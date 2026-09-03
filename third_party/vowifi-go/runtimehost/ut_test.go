package runtimehost

import (
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/netstack"
	"github.com/iniwex5/vowifi-go/internal/vowifi/runtimecore"
	"github.com/iniwex5/vowifi-go/xcap"
)

func TestUtAccessMissingSession(t *testing.T) {
	if _, err := (*Instance)(nil).UtAccess(); err == nil {
		t.Fatal("nil instance")
	}
	if _, err := (&Instance{}).UtAccess(); err == nil || !strings.Contains(err.Error(), "XCAP PDN") {
		t.Fatalf("empty instance err=%v", err)
	}
}

func TestUtAccessDoesNotUseIMSWhenXCAPRequired(t *testing.T) {
	inst := &Instance{}
	inst.setSession(&runtimecore.SessionResult{
		XCAPRequired: true, IMSNetwork: &netstack.Network{},
	})
	if _, err := inst.UtAccess(); err == nil || !strings.Contains(err.Error(), "XCAP PDN") {
		t.Fatalf("err=%v", err)
	}
}

func TestUtAccessRequiresPublicIdentity(t *testing.T) {
	inst := &Instance{}
	inst.setSession(&runtimecore.SessionResult{IMSNetwork: &netstack.Network{}})
	if _, err := inst.UtAccess(); err == nil || !strings.Contains(err.Error(), "public identity") {
		t.Fatalf("err=%v", err)
	}
}

func TestUtUsesPublicHostWhenXCAPPDNIsUp(t *testing.T) {
	if utUsesOnNetHost(&runtimecore.SessionResult{
		XCAPNetwork: &netstack.Network{}, IMSNetwork: &netstack.Network{},
	}) {
		t.Fatal("dedicated XCAP PDN should use the public XCAP name")
	}
	if !utUsesOnNetHost(&runtimecore.SessionResult{IMSNetwork: &netstack.Network{}}) {
		t.Fatal("IMS-only Ut should use the on-net XCAP name")
	}
}

func TestDomainFromXUI(t *testing.T) {
	if got := domainFromXUI("sip:user@ims.example.com"); got != "ims.example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestUtDocumentMapsAndMutatesServices(t *testing.T) {
	raw := xcap.Document{XUI: "sip:user@ims.example.com", ETag: "v1"}
	raw.SetCFU(true, "tel:+441111")
	raw.SetOIR(true, true)
	raw.SetBarring(true, false)

	doc := utDocumentFromXCAP(raw)
	if doc.CommunicationDiversion != (UtToggle{Active: true, Target: "tel:+441111"}) {
		t.Fatalf("communication diversion = %+v", doc.CommunicationDiversion)
	}
	if doc.IdentityRestriction != (UtIdentityRestriction{Active: true, Restricted: true}) {
		t.Fatalf("identity restriction = %+v", doc.IdentityRestriction)
	}
	if !doc.IncomingBarring.Active || doc.OutgoingBarring.Active {
		t.Fatalf("barring = incoming:%t outgoing:%t", doc.IncomingBarring.Active, doc.OutgoingBarring.Active)
	}

	doc.SetCommunicationDiversion(false, "tel:+442222")
	doc.SetIdentityRestriction(false, false)
	doc.SetBarring(false, true)
	if doc.document.CDIV == nil || doc.document.CDIV.Active || doc.document.CFUTarget() != "tel:+442222" {
		t.Fatalf("underlying diversion = %+v", doc.document.CDIV)
	}
	if doc.document.OIR == nil || doc.document.OIR.Active || doc.document.ICB.Active || !doc.document.OCB.Active {
		t.Fatalf("underlying services were not updated")
	}
}
