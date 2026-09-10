package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestIMSSubscriptionRejectionPersistence(t *testing.T) {
	previousDB := DB
	if err := Init(filepath.Join(t.TempDir(), "subscription-rejections.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
	})

	identity := "wwan0\x00user@ims.example\x00ims.example"
	now := time.Now().UTC().Truncate(time.Second)
	expiresAt := now.Add(time.Hour)
	if err := SaveIMSSubscriptionRejection(identity, "reg", 489, expiresAt); err != nil {
		t.Fatal(err)
	}
	status, loadedExpiry, err := LoadIMSSubscriptionRejection(identity, "reg", now)
	if err != nil || status != 489 || !loadedExpiry.Equal(expiresAt) {
		t.Fatalf("loaded status=%d expiry=%s err=%v", status, loadedExpiry, err)
	}

	if err := SaveIMSSubscriptionRejection(identity, "reg", 405, now.Add(time.Minute)); err == nil {
		t.Fatal("ambiguous 405 response was persisted")
	}
	if err := DeleteIMSSubscriptionRejections(identity); err != nil {
		t.Fatal(err)
	}
	status, _, err = LoadIMSSubscriptionRejection(identity, "reg", now)
	if err != nil || status != 0 {
		t.Fatalf("deleted status=%d err=%v", status, err)
	}
}

func TestIMSSubscriptionRejectionExpiresAndValidatesBoundary(t *testing.T) {
	previousDB := DB
	if err := Init(filepath.Join(t.TempDir(), "subscription-rejections.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
	})

	now := time.Now().UTC().Truncate(time.Second)
	identityHash, eventPackage, err := normalizeIMSSubscriptionRejectionKey("identity", "message-summary")
	if err != nil {
		t.Fatal(err)
	}
	legacy := IMSSubscriptionRejection{
		IdentityHash: identityHash, EventPackage: eventPackage,
		StatusCode: 405, ExpiresAt: now.Add(time.Hour),
	}
	if err := DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	status, _, err := LoadIMSSubscriptionRejection("identity", "message-summary", now)
	if err != nil || status != 0 {
		t.Fatalf("legacy 405 status=%d err=%v", status, err)
	}
	var count int64
	if err := DB.Model(&IMSSubscriptionRejection{}).Where(
		"identity_hash = ? AND event_package = ?", identityHash, eventPackage,
	).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("legacy 405 row count=%d err=%v", count, err)
	}
	if err := SaveIMSSubscriptionRejection("identity", "reg", 503, now.Add(time.Hour)); err == nil {
		t.Fatal("transient status was persisted")
	}
	if _, _, err := LoadIMSSubscriptionRejection("identity", "unknown", now); err == nil {
		t.Fatal("unknown event package was accepted")
	}
}
