package db

import (
	"context"
	"testing"
	"time"
)

func TestSMSNotificationsBaselineAndLateDelivery(t *testing.T) {
	openTestDB(t)
	old := createNotificationTestSMS(t, SMS{Type: 1, Status: 0, Timestamp: time.Now()})
	initial, err := GetSMSNotifications(context.Background(), nil)
	if err != nil || initial.Cursor != old.ID || initial.NewCount != 0 || initial.Latest != nil || initial.UnreadCount != 1 {
		t.Fatalf("baseline=%+v err=%v", initial, err)
	}
	createNotificationTestSMS(t, SMS{Type: 2, Status: 2})
	late := createNotificationTestSMS(t, SMS{Type: 1, Status: 0, Content: "late", Timestamp: time.Now().Add(-3 * time.Hour)})
	result, err := GetSMSNotifications(context.Background(), &initial.Cursor)
	if err != nil || result.NewCount != 1 || result.UnreadCount != 2 || result.Latest == nil || result.Latest.ID != late.ID {
		t.Fatalf("late notification=%+v err=%v", result, err)
	}
	repeated, err := GetSMSNotifications(context.Background(), &result.Cursor)
	if err != nil || repeated.NewCount != 0 || repeated.Latest != nil {
		t.Fatalf("repeated=%+v err=%v", repeated, err)
	}
}

func TestSMSNotificationsSummarizesAllAndDoesNotMarkRead(t *testing.T) {
	openTestDB(t)
	const total = 205
	for index := 0; index < total; index++ {
		createNotificationTestSMS(t, SMS{Type: 1, Status: 0})
	}
	cursor := uint(0)
	result, err := GetSMSNotifications(context.Background(), &cursor)
	if err != nil || result.NewCount != total || result.UnreadCount != total || result.Latest == nil {
		t.Fatalf("summary=%+v err=%v", result, err)
	}
	if err := DB.Model(&SMS{}).Where("id = ?", result.Latest.ID).Update("status", 1).Error; err != nil {
		t.Fatal(err)
	}
	read, err := GetSMSNotifications(context.Background(), &result.Cursor)
	if err != nil || read.UnreadCount != total-1 || read.NewCount != 0 {
		t.Fatalf("read state=%+v err=%v", read, err)
	}
	if err := DB.Delete(&SMS{}, result.Latest.ID).Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := GetSMSNotifications(context.Background(), &result.Cursor)
	if err != nil || deleted.Cursor != result.Cursor || deleted.NewCount != 0 {
		t.Fatalf("deleted cursor=%+v err=%v", deleted, err)
	}
}

func TestSMSNotificationsEmptyInboxAndCancellation(t *testing.T) {
	openTestDB(t)
	initial, err := GetSMSNotifications(context.Background(), nil)
	if err != nil || initial.Cursor != 0 || initial.UnreadCount != 0 {
		t.Fatalf("empty=%+v err=%v", initial, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetSMSNotifications(ctx, nil); err == nil {
		t.Fatal("canceled query returned success")
	}
}

func createNotificationTestSMS(t *testing.T, message SMS) SMS {
	t.Helper()
	if err := DB.Create(&message).Error; err != nil {
		t.Fatal(err)
	}
	return message
}
