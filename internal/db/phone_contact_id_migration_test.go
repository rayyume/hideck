package db

import (
	"context"
	"errors"
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

func TestPhoneContactStrictBatchIsAtomicAndSharesGroupID(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "strict-batch.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&PhoneContact{}); err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = database
	t.Cleanup(func() { DB = previous })

	rows, err := UpsertPhoneContactBatchWithRegion(t.Context(), []PhoneContactInput{
		{Number: "10000", Name: "客服", GroupKey: "manual"},
		{Number: "10001", Name: "客服", GroupKey: "manual"},
	}, "")
	if err != nil || len(rows) != 2 || rows[0].ContactID == "" || rows[0].ContactID != rows[1].ContactID {
		t.Fatalf("strict batch rows=%+v err=%v", rows, err)
	}

	_, err = UpsertPhoneContactBatchWithRegion(t.Context(), []PhoneContactInput{
		{Number: "10002", Name: "有效"},
		{Number: "invalid", Name: "无效"},
	}, "")
	if !errors.Is(err, ErrInvalidPhoneContact) {
		t.Fatalf("strict batch error = %v", err)
	}
	if _, err := GetPhoneContact(t.Context(), "10002"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("rolled-back contact lookup error = %v", err)
	}
}

func TestPhoneContactGroupUpdateAndDeleteAffectEveryNumber(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "contact-group.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&PhoneContact{}); err != nil {
		t.Fatal(err)
	}
	previous := DB
	DB = database
	t.Cleanup(func() { DB = previous })

	rows, err := UpsertPhoneContactBatchWithRegion(t.Context(), []PhoneContactInput{
		{Number: "10000", Name: "旧名字", GroupKey: "person"},
		{Number: "10001", Name: "旧名字", GroupKey: "person"},
		{Number: "10002", Name: "其他联系人"},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := UpdatePhoneContactGroupName(t.Context(), rows[0].ContactID, "新名字")
	if err != nil || len(updated) != 2 || updated[0].Name != "新名字" || updated[1].Name != "新名字" {
		t.Fatalf("updated contacts=%+v err=%v", updated, err)
	}
	deleted, err := DeletePhoneContactGroup(t.Context(), rows[0].ContactID)
	if err != nil || len(deleted) != 2 {
		t.Fatalf("deleted numbers=%v err=%v", deleted, err)
	}
	remaining, err := ListPhoneContacts(t.Context())
	if err != nil || len(remaining) != 1 || remaining[0].Number != "10002" {
		t.Fatalf("remaining contacts=%+v err=%v", remaining, err)
	}
}
