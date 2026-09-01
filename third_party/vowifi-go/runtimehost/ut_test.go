package runtimehost

import (
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/netstack"
	"github.com/iniwex5/vowifi-go/internal/vowifi/runtimecore"
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
