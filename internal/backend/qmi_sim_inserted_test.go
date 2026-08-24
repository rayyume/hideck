package backend

import (
	"context"
	"testing"

	qmimanager "github.com/iniwex5/quectel-qmi-go/pkg/manager"
	"github.com/iniwex5/quectel-qmi-go/pkg/qmi"
)

func TestIsSimInsertedIgnoresStaleInsertedWithoutIdentity(t *testing.T) {
	inserted := true
	snap := &qmimanager.DeviceSnapshot{}
	snap.UpdateIdentities(qmimanager.DeviceIdentities{
		IMEI:        "866069000000001",
		SimInserted: &inserted,
	})
	absent := qmi.SIMAbsent
	src := &qmiBackendSendSourceStub{snapshot: snap, simStatus: &absent}
	ok, err := (&QMIBackend{source: src}).IsSimInserted(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("无 IMSI/ICCID 的过期 inserted 快照应走实时 UIM")
	}
	if src.simStatusCalls != 1 {
		t.Fatalf("simStatusCalls=%d want 1", src.simStatusCalls)
	}
}

func TestIsSimInsertedUsesSnapshotWhenIMSIPresent(t *testing.T) {
	inserted := true
	snap := &qmimanager.DeviceSnapshot{}
	snap.UpdateIdentities(qmimanager.DeviceIdentities{
		IMEI:        "866069000000001",
		IMSI:        "460010000000001",
		SimInserted: &inserted,
	})
	absent := qmi.SIMAbsent
	src := &qmiBackendSendSourceStub{snapshot: snap, simStatus: &absent}
	ok, err := (&QMIBackend{source: src}).IsSimInserted(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("有 IMSI 的快照应直接认为已插卡")
	}
	if src.simStatusCalls != 0 {
		t.Fatalf("simStatusCalls=%d want 0", src.simStatusCalls)
	}
}
