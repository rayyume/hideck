package qmi

import (
	"context"
	"testing"
	"time"
)

func TestNormalizeClientOptionsZeroValueStillEnablesHandshake(t *testing.T) {
	got := normalizeClientOptions(ClientOptions{})
	if !got.SyncOnOpen {
		t.Fatal("SyncOnOpen=false, want true for zero-value ClientOptions")
	}
	if !got.QueryVersionOnOpen {
		t.Fatal("QueryVersionOnOpen=false, want true for zero-value ClientOptions")
	}
}

func TestNormalizeClientOptionsHonorsDisableOpenHandshake(t *testing.T) {
	opts := DefaultClientOptions()
	opts.SyncOnOpen = false
	opts.QueryVersionOnOpen = false
	opts.DisableOpenHandshake = true

	got := normalizeClientOptions(opts)
	if got.SyncOnOpen {
		t.Fatal("SyncOnOpen restored to true, want false when DisableOpenHandshake is set")
	}
	if got.QueryVersionOnOpen {
		t.Fatal("QueryVersionOnOpen restored to true, want false when DisableOpenHandshake is set")
	}
}

func TestOpenHandshakeContextCapsWhenParentHasDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	handshake, handshakeCancel := openHandshakeContext(parent)
	defer handshakeCancel()

	deadline, ok := handshake.Deadline()
	if !ok {
		t.Fatal("handshake context missing deadline")
	}
	remain := time.Until(deadline)
	if remain > openHandshakeTimeout+50*time.Millisecond {
		t.Fatalf("handshake remain=%s, want <= %s", remain, openHandshakeTimeout)
	}
	if remain < openHandshakeTimeout-50*time.Millisecond {
		t.Fatalf("handshake remain=%s, want about %s", remain, openHandshakeTimeout)
	}
}
