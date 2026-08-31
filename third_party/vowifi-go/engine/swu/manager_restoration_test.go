package swu

import (
	"context"
	"testing"
	"time"
)

func TestLegacySessionManagerLifecycle(t *testing.T) {
	manager := NewSessionManager()
	if _, err := manager.Start(context.Background(), "", &Config{}); err == nil {
		t.Fatal("Start accepted an empty session ID")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session, err := manager.Start(ctx, "device-1", &Config{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if got, ok := manager.Get("device-1"); !ok || got != session {
		t.Fatal("Get did not return the managed session")
	}
	if _, err := manager.Start(ctx, "device-1", &Config{}); err == nil {
		t.Fatal("Start accepted a duplicate session ID")
	}

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := session.WaitDoneContext(waitCtx); err != nil {
		t.Fatalf("failed managed session did not finish cleanup: %v", err)
	}
	if err := manager.Stop("device-1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, ok := manager.Get("device-1"); ok {
		t.Fatal("Stop retained the managed session")
	}
	if err := manager.Stop("device-1"); err == nil {
		t.Fatal("Stop accepted a missing session ID")
	}
}

func TestSessionManagerOverlappingSlotKeepsDefault(t *testing.T) {
	manager := NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	primary, err := manager.Start(ctx, "device-1", &Config{OmitInitialContact: false})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	successor, err := manager.StartSlot(ctx, "device-1", "ike-reauth", &Config{OmitInitialContact: true})
	if err != nil {
		t.Fatalf("StartSlot: %v", err)
	}
	if got, ok := manager.Get("device-1"); !ok || got != primary {
		t.Fatal("Get lost the forwarding default session")
	}
	if got, ok := manager.GetSlot("device-1", "ike-reauth"); !ok || got != successor {
		t.Fatal("GetSlot did not return the overlapping session")
	}
	if len(manager.Sessions("device-1")) != 2 {
		t.Fatalf("Sessions = %d, want 2 overlapping IKE runtimes", len(manager.Sessions("device-1")))
	}
	retired, err := manager.SwapDefault("device-1", "ike-reauth")
	if err != nil {
		t.Fatalf("SwapDefault: %v", err)
	}
	if retired != primary {
		t.Fatal("SwapDefault did not return the old default SA")
	}
	if got, ok := manager.Get("device-1"); !ok || got != successor {
		t.Fatal("Get did not return the successor after cutover")
	}
	if _, ok := manager.GetSlot("device-1", "ike-reauth"); ok {
		t.Fatal("reauth slot still occupied after promotion")
	}
	if err := manager.StopSlot("device-1", DefaultSessionSlot); err != nil {
		t.Fatalf("StopSlot successor: %v", err)
	}
	primary.Shutdown()
}
