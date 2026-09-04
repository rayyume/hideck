package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestPhoneContactLookupMergesNameAndCarrier(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "contacts.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&PhoneContact{}); err != nil {
		t.Fatal(err)
	}
	prev := DB
	DB = database
	t.Cleanup(func() { DB = prev })

	ctx := context.Background()
	if _, err := UpsertPhoneContact(ctx, "10010", "联通客服"); err != nil {
		t.Fatal(err)
	}
	got := LookupPhoneIdentity(ctx, "10010")
	if got.Name != "联通客服" || got.Title != "联通客服" || got.Carrier != "中国联通" {
		t.Fatalf("%+v", got)
	}
	if _, err := GetPhoneContact(ctx, "+8610010"); err != nil {
		t.Fatalf("normalized lookup: %v", err)
	}
}
