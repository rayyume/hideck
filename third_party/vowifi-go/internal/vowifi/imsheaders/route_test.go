package imsheaders

import (
	"reflect"
	"testing"
)

func TestEffectiveRoutePrefersServiceRoute(t *testing.T) {
	if got := EffectiveRoute(" <sip:scscf.example;lr> ", "<sip:pcscf.example;lr;ob>"); got != "<sip:scscf.example;lr>" {
		t.Fatalf("EffectiveRoute = %q", got)
	}
	if got := EffectiveRoute("", " <sip:pcscf.example;lr;ob> "); got != "<sip:pcscf.example;lr;ob>" {
		t.Fatalf("Path fallback = %q", got)
	}
	if got := EffectiveRoute("  ", ""); got != "" {
		t.Fatalf("empty route = %q", got)
	}
}

func TestRouteSetDoesNotStackEmptyProxy(t *testing.T) {
	if got := RouteSet("<sip:scscf.example;lr>", ""); !reflect.DeepEqual(got, []string{"<sip:scscf.example;lr>"}) {
		t.Fatalf("RouteSet = %v", got)
	}
}

func TestPreloadedOriginatingRouteStacksPCSCFThenServiceRoute(t *testing.T) {
	got := PreloadedOriginatingRoute(
		"<sip:10.128.120.17:50600;transport=tcp;lr>",
		"<sip:orig@scscf.ims.example;lr>,<sip:icscf.ims.example;lr>",
	)
	want := "<sip:10.128.120.17:50600;transport=tcp;lr>,<sip:orig@scscf.ims.example;lr>,<sip:icscf.ims.example;lr>"
	if got != want {
		t.Fatalf("PreloadedOriginatingRoute = %q", got)
	}
}

func TestPreloadedOriginatingRouteDoesNotDuplicatePCSCF(t *testing.T) {
	got := PreloadedOriginatingRoute(
		"<sip:10.128.120.17:50600;transport=tcp;lr>",
		"<sip:10.128.120.17:50600;lr>,<sip:orig@scscf.ims.example;lr>",
	)
	want := "<sip:10.128.120.17:50600;transport=tcp;lr>,<sip:orig@scscf.ims.example;lr>"
	if got != want {
		t.Fatalf("deduped Route = %q", got)
	}
}

func TestRecordRouteSetReversesCommaSeparatedValues(t *testing.T) {
	got := RecordRouteSet([]string{"<sip:edge-one.example;lr>, <sip:edge-two.example;lr>"})
	want := []string{"<sip:edge-two.example;lr>", "<sip:edge-one.example;lr>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RecordRouteSet = %#v, want %#v", got, want)
	}
	got = RecordRouteSet([]string{"<sip:edge-a.example;lr>", "<sip:edge-b.example;lr>"})
	want = []string{"<sip:edge-b.example;lr>", "<sip:edge-a.example;lr>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("multi-header RecordRouteSet = %#v, want %#v", got, want)
	}
}

func TestPreloadedOriginatingRouteWithoutServiceRouteIsPCSCF(t *testing.T) {
	got := PreloadedOriginatingRoute("<sip:10.128.120.17:50600;transport=tcp;lr>", "")
	if got != "<sip:10.128.120.17:50600;transport=tcp;lr>" {
		t.Fatalf("P-CSCF-only Route = %q", got)
	}
}
