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
