package db

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestLegacySameNameContactsReceiveIndependentIDs(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(`CREATE TABLE phone_contacts (
		number TEXT PRIMARY KEY, name TEXT NOT NULL, created_at DATETIME, updated_at DATETIME
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Exec(
		"INSERT INTO phone_contacts (number, name) VALUES (?, ?), (?, ?)",
		"10000", "张伟", "10001", "张伟",
	).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&PhoneContact{}); err != nil {
		t.Fatal(err)
	}
	if err := MigratePhoneContactIDs(database); err != nil {
		t.Fatal(err)
	}
	var rows []PhoneContact
	if err := database.Order("number ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ContactID == "" || rows[1].ContactID == "" || rows[0].ContactID == rows[1].ContactID {
		t.Fatalf("migrated contacts = %+v", rows)
	}
}

func TestImportedGroupSharesIDWithoutMergingSameNameRows(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "groups.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&PhoneContact{}); err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = database
	t.Cleanup(func() { DB = previous })

	inputs := []PhoneContactInput{
		{Number: "10000", Name: "张伟", GroupKey: "card:1"},
		{Number: "10001", Name: "张伟", GroupKey: "card:1"},
		{Number: "10002", Name: "张伟", GroupKey: "card:2"},
	}
	imported, skipped, err := UpsertPhoneContactsWithRegion(context.Background(), inputs, "")
	if err != nil || imported != 3 || skipped != 0 {
		t.Fatalf("batch result imported=%d skipped=%d err=%v", imported, skipped, err)
	}
	var rows []PhoneContact
	if err := database.Order("number ASC").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if rows[0].ContactID != rows[1].ContactID || rows[0].ContactID == rows[2].ContactID {
		t.Fatalf("group IDs = %+v", rows)
	}
}

func TestPhoneContactRejectsInvalidContactID(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "invalid-id.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&PhoneContact{}); err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = database
	t.Cleanup(func() { DB = previous })

	_, err = UpsertPhoneContactWithRegion(context.Background(), PhoneContactInput{
		Number: "10000", Name: "测试", ContactID: "arbitrary-group",
	})
	if err != ErrInvalidPhoneContact {
		t.Fatalf("invalid contact ID error = %v", err)
	}
}
