package device

import "testing"

func TestPhoneNumberRegionUsesCachedSIMHomeMCC(t *testing.T) {
	worker := &Worker{ID: "wwan0"}
	worker.state.Identity.NativeMCC = "234"
	worker.state.Identity.Ready = true
	pool := &Pool{workers: map[string]*Worker{"wwan0": worker}}

	if got := pool.PhoneNumberRegion("wwan0"); got != "GB" {
		t.Fatalf("PhoneNumberRegion()=%q want GB", got)
	}
}
