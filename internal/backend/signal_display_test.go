package backend

import "testing"

func TestDisplaySignalDBMFallsBackToRSRPWhenRSSIIsQMIPlaceholder(t *testing.T) {
	t.Parallel()

	if got := DisplaySignalDBM(-125, -86); got != -86 {
		t.Fatalf("DisplaySignalDBM(-125, -86)=%d, want -86", got)
	}
	if got := DisplaySignalDBM(-128, -90); got != -90 {
		t.Fatalf("DisplaySignalDBM(-128, -90)=%d, want -90", got)
	}
	if got := DisplaySignalDBM(0, -80); got != -80 {
		t.Fatalf("DisplaySignalDBM(0, -80)=%d, want -80", got)
	}
	if got := DisplaySignalDBM(-72, -86); got != -72 {
		t.Fatalf("valid RSSI should win, got %d", got)
	}
	if got := DisplaySignalDBM(-125, 0); got != 0 {
		t.Fatalf("both placeholders should yield 0, got %d", got)
	}
	if got := DisplaySignalDBM(-125, -125); got != 0 {
		t.Fatalf("placeholder RSRP should not be reused, got %d", got)
	}
}

func TestIsPlaceholderRSSI(t *testing.T) {
	t.Parallel()

	for _, rssi := range []int{0, -125, -128, -999} {
		if !IsPlaceholderRSSI(rssi) {
			t.Fatalf("rssi=%d should be placeholder", rssi)
		}
	}
	if IsPlaceholderRSSI(-86) || IsPlaceholderRSSI(-105) {
		t.Fatal("real LTE values must not be placeholders")
	}
}
