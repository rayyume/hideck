package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yibaiba/hideck/internal/config"
	"github.com/yibaiba/hideck/internal/device"
	"github.com/yibaiba/hideck/internal/esim"
	"github.com/yibaiba/hideck/internal/modem"
)

func TestHandleEsimRecoverLebaraIdentityRejectsICCIDMismatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := newTestEsimManager()
	mgrTestSetOverviewCache(t, mgr, &esim.EsimOverview{Profiles: []esim.EUICCProfiles{{
		Profiles: []esim.ProfileItem{{ICCID: "8944000000000000087", Name: "Lebara UK", State: 1}},
	}}})
	pool := device.NewPool(&config.Config{})
	w := &device.Worker{ID: "dev-esim", EsimMgr: mgr}
	setNestedPrivateField(t, w, []string{"state", "Identity", "ICCID"}, "8944000000000000087")
	setNestedPrivateField(t, w, []string{"state", "Identity", "IMSI"}, "204040000000001")
	setNestedPrivateField(t, pool, []string{"workers"}, map[string]*device.Worker{"dev-esim": w})
	server := &Server{pool: pool}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "device_id", Value: "dev-esim"},
		{Key: "iccid", Value: "8944000000000000099"},
	}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/devices/dev-esim/esim/profiles/8944000000000000099/actions/recover-lebara-identity", bytes.NewBuffer(nil))
	server.handleEsimRecoverLebaraIdentity(ctx)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandleEsimRecoverLebaraIdentityStartsWhenFlipped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mgr := newTestEsimManager()
	mgrTestSetOverviewCache(t, mgr, &esim.EsimOverview{Profiles: []esim.EUICCProfiles{{
		AIDHex: "A000",
		Profiles: []esim.ProfileItem{
			{ICCID: "8944000000000000087", Name: "Lebara UK", State: 1},
			{ICCID: "8944000000000000001", Name: "Bootstrap", State: 0, ClassText: "provisioning"},
		},
	}}})
	pool := device.NewPool(&config.Config{})
	w := &device.Worker{ID: "dev-esim", EsimMgr: mgr}
	setNestedPrivateField(t, w, []string{"state", "Identity", "ICCID"}, "8944000000000000087")
	setNestedPrivateField(t, w, []string{"state", "Identity", "IMSI"}, "204040000000001")
	setNestedPrivateField(t, pool, []string{"workers"}, map[string]*device.Worker{"dev-esim": w})
	server := &Server{pool: pool}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{
		{Key: "device_id", Value: "dev-esim"},
		{Key: "iccid", Value: "8944000000000000087"},
	}
	ctx.Request = httptest.NewRequest(http.MethodPost, "/devices/dev-esim/esim/profiles/8944000000000000087/actions/recover-lebara-identity", bytes.NewBuffer(nil))
	server.handleEsimRecoverLebaraIdentity(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	snap := waitLebaraUKRecoverSnapshot(t, pool, "dev-esim")
	if !snap.InFlight && snap.Status != device.LebaraUKIdentityRecovering &&
		snap.Status != device.LebaraUKIdentityWaiting && snap.Status != device.LebaraUKIdentityFailed {
		t.Fatalf("snapshot=%+v", snap)
	}
}

func TestBuildOverviewLiteItemIncludesLebaraIdentityRecover(t *testing.T) {
	mgr := newTestEsimManager()
	mgrTestSetOverviewCache(t, mgr, &esim.EsimOverview{Profiles: []esim.EUICCProfiles{{
		Profiles: []esim.ProfileItem{{ICCID: "8944000000000000087", Name: "Lebara UK", State: 1}},
	}}})
	p := device.NewPool(&config.Config{})
	w := &device.Worker{ID: "dev-lebara", EsimMgr: mgr}
	setNestedPrivateField(t, w, []string{"state", "Identity", "ICCID"}, "8944000000000000087")
	setNestedPrivateField(t, w, []string{"state", "Identity", "IMSI"}, "204040000000001")
	if err := p.ScheduleLebaraUKIdentityRecover(w, "8944000000000000087", true); err != nil {
		t.Fatal(err)
	}
	server := &Server{pool: p}
	item := server.buildOverviewLiteItemFromWorker(
		w,
		config.DeviceConfig{ID: "dev-lebara", Name: "Lebara"},
		modem.DeviceStatus{IMSI: "204040000000001", ICCID: "8944000000000000087"},
		nil,
	)
	if item.RFLock != device.RFLockLebaraUKNextGen {
		t.Fatalf("RFLock=%q", item.RFLock)
	}
	if item.LebaraIdentityStatus != device.LebaraUKIdentityRecovering &&
		item.LebaraIdentityStatus != device.LebaraUKIdentityWaiting &&
		item.LebaraIdentityStatus != device.LebaraUKIdentityFailed {
		t.Fatalf("LebaraIdentityStatus=%q", item.LebaraIdentityStatus)
	}
}

func waitLebaraUKRecoverSnapshot(t *testing.T, pool *device.Pool, deviceID string) device.LebaraUKIdentityRecoverSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := pool.LebaraUKIdentityRecoverSnapshot(deviceID)
		if snap.InFlight || snap.Status != "" {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("recover snapshot stayed empty")
	return device.LebaraUKIdentityRecoverSnapshot{}
}
