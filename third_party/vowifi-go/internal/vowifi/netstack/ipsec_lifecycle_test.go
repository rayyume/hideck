package netstack

import (
	"context"
	"testing"
)

func TestRetiredIPSecCleanupCannotRemoveReplacement(t *testing.T) {
	network := newOriginalTestNetwork(t, newTestInnerEndpoint())
	t.Cleanup(func() { _ = network.Close() })
	oldCleanup, err := network.InstallIPSec3GPP(context.Background(), testIPSecPolicy())
	if err != nil {
		t.Fatal(err)
	}
	newCleanup, err := network.InstallIPSec3GPP(context.Background(), testIPSecPolicy())
	if err != nil {
		t.Fatal(err)
	}
	current := network.bridge.currentTransformer()
	if err := oldCleanup(); err != nil {
		t.Fatal(err)
	}
	if network.bridge.currentTransformer() != current || !network.IPSec3GPPPolicyInstalled() {
		t.Fatal("retired cleanup removed the replacement IPsec policy")
	}
	if err := newCleanup(); err != nil {
		t.Fatal(err)
	}
	if network.bridge.currentTransformer() != nil || network.IPSec3GPPPolicyInstalled() {
		t.Fatal("current cleanup retained the policy")
	}
}
