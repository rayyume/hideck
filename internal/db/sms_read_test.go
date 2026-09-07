package db

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMarkSMSHistoricalWindowReadPreservesUnseenMessages(t *testing.T) {
	openTestDB(t)
	const iccid, peer = "ICC-HISTORY", "+10086"
	base := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	save := func(at time.Time) uint {
		return saveMessageTargetFixture(t, SMSRecord{
			Identity: SMSIdentity{ICCID: iccid, IMSI: "IMSI-HISTORY"}, Sender: peer,
			Content: at.String(), Type: smsTypeIncoming, Status: smsStatusUnread, Timestamp: at,
		})
	}
	save(base.Add(time.Hour))
	targetID := save(base)
	window, err := GetSMSMessageTarget(context.Background(), targetID)
	if err != nil || len(window.Messages) != 1 {
		t.Fatalf("historical window=%+v err=%v", window, err)
	}
	// A concurrent arrival with an older SMSC timestamp is outside the snapshot too.
	save(base.Add(-time.Hour))
	result, err := MarkSMSMessagesReadByICCID(iccid, peer, []uint{window.Messages[0].ID})
	if err != nil || result.Marked != 1 || result.UnreadCount != 2 {
		t.Fatalf("read result=%+v err=%v", result, err)
	}
	// Repeated submissions remain idempotent, including duplicate IDs.
	result, err = MarkSMSMessagesReadByICCID(iccid, peer, []uint{targetID, targetID})
	if err != nil || result.Marked != 0 || result.UnreadCount != 2 {
		t.Fatalf("repeated read=%+v err=%v", result, err)
	}
	var messages []SMS
	if err := DB.Order("id").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if messages[0].Status != smsStatusUnread || messages[1].Status != smsStatusRead || messages[2].Status != smsStatusUnread {
		t.Fatalf("outside-window and later arrivals must remain unread: %+v", messages)
	}
	var contact SMSContact
	if err := DB.Where("iccid = ? AND peer = ?", iccid, peer).First(&contact).Error; err != nil {
		t.Fatal(err)
	}
	if contact.UnreadCount != 2 {
		t.Fatalf("unread count=%d, want 2", contact.UnreadCount)
	}
	notifications, err := GetSMSNotifications(context.Background(), nil)
	if err != nil || notifications.UnreadCount != 2 {
		t.Fatalf("global unread count=%d err=%v", notifications.UnreadCount, err)
	}
}

func TestMarkSMSMessagesReadRejectsInvalidScopeAtomically(t *testing.T) {
	openTestDB(t)
	for _, identity := range []SMSIdentity{{ICCID: "card-a", IMSI: "imsi-a"}, {ICCID: "card-b", IMSI: "imsi-b"}} {
		for _, peer := range []string{"sender", "other"} {
			if err := SaveSMSForIdentity(SMSRecord{Identity: identity, Sender: peer, Content: "unread", Type: 1, Timestamp: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}
	}
	var messages []SMS
	if err := DB.Order("id").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	for _, ids := range [][]uint{nil, {}, {0}, {messages[0].ID, messages[1].ID}, {messages[0].ID, messages[2].ID}, {messages[0].ID, messages[3].ID + 1}} {
		if _, err := MarkSMSMessagesReadByICCID("card-a", "sender", ids); !errors.Is(err, ErrSMSReadBoundaryInvalid) {
			t.Fatalf("ids=%v err=%v", ids, err)
		}
	}
	var unread int64
	if err := DB.Model(&SMS{}).Where("status = ?", smsStatusUnread).Count(&unread).Error; err != nil || unread != 4 {
		t.Fatalf("partial update: unread=%d err=%v", unread, err)
	}
}

func TestMarkSMSThreadReadPersistsAndPreservesNewerUnreadMessages(t *testing.T) {
	openTestDB(t)
	const (
		iccid = "ICC-READ"
		imsi  = "IMSI-READ"
		peer  = "+10086"
	)
	if err := DB.Create(&SIMCard{ICCID: iccid, IMSI: imsi}).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 13, 18, 0, 0, 0, time.UTC)
	for i, content := range []string{"first", "second", "newer"} {
		if err := SaveSMSForIdentity(SMSRecord{
			Identity: SMSIdentity{ICCID: iccid, IMSI: imsi}, Sender: peer,
			Content: content, Type: 1, Status: 0, Timestamp: base.Add(time.Duration(i) * time.Second),
		}); err != nil {
			t.Fatal(err)
		}
	}

	var boundary SMS
	if err := DB.Where("iccid = ? AND peer = ? AND content = ?", iccid, peer, "second").First(&boundary).Error; err != nil {
		t.Fatal(err)
	}
	result, err := MarkSMSThreadReadByICCID(iccid, peer, boundary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Marked != 2 || result.UnreadCount != 1 {
		t.Fatalf("result=%+v want marked=2 unread=1", result)
	}

	var messages []SMS
	if err := DB.Where("iccid = ? AND peer = ?", iccid, peer).Order("id").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if messages[0].Status != 1 || messages[1].Status != 1 || messages[2].Status != 0 {
		t.Fatalf("statuses=%d,%d,%d want 1,1,0", messages[0].Status, messages[1].Status, messages[2].Status)
	}
	var contact SMSContact
	if err := DB.Where("iccid = ? AND peer = ?", iccid, peer).First(&contact).Error; err != nil {
		t.Fatal(err)
	}
	if contact.UnreadCount != 1 {
		t.Fatalf("UnreadCount=%d want=1", contact.UnreadCount)
	}
}

func TestMarkSMSThreadReadRejectsUnknownOrInvalidScope(t *testing.T) {
	openTestDB(t)
	if _, err := MarkSMSThreadReadByICCID("", "+10086", 1); !errors.Is(err, ErrSMSReadBoundaryInvalid) {
		t.Fatalf("empty ICCID err=%v", err)
	}
	if _, err := MarkSMSThreadReadByICCID("missing", "+10086", 1); !errors.Is(err, ErrSMSNotFound) {
		t.Fatalf("missing thread err=%v", err)
	}

	const iccid = "ICC-READ-BOUNDARY"
	if err := DB.Create(&SIMCard{ICCID: iccid, IMSI: "IMSI-READ-BOUNDARY"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, peer := range []string{"+10010", "+10086"} {
		if err := SaveSMSForIdentity(SMSRecord{
			Identity: SMSIdentity{ICCID: iccid, IMSI: "IMSI-READ-BOUNDARY"},
			Sender:   peer, Content: peer, Type: 1, Status: 0, Timestamp: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var foreign SMS
	if err := DB.Where("iccid = ? AND peer = ?", iccid, "+10010").First(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := MarkSMSThreadReadByICCID(iccid, "+10086", foreign.ID); !errors.Is(err, ErrSMSReadBoundaryInvalid) {
		t.Fatalf("foreign boundary err=%v", err)
	}
}

func TestMarkSMSThreadReadUsesSnapshotIDForOutOfOrderTimestamps(t *testing.T) {
	openTestDB(t)
	const (
		iccid = "ICC-READ-ORDER"
		imsi  = "IMSI-READ-ORDER"
		peer  = "+10000"
	)
	if err := DB.Create(&SIMCard{ICCID: iccid, IMSI: imsi}).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 8, 13, 20, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		content string
		at      time.Time
	}{
		{content: "old", at: base},
		{content: "future-inserted-first", at: base.Add(2 * time.Second)},
		{content: "display-boundary", at: base.Add(time.Second)},
		{content: "older-concurrent-insert", at: base.Add(-time.Second)},
	} {
		if err := SaveSMSForIdentity(SMSRecord{
			Identity: SMSIdentity{ICCID: iccid, IMSI: imsi}, Sender: peer,
			Content: item.content, Type: 1, Status: 0, Timestamp: item.at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	var boundary SMS
	if err := DB.Where("content = ?", "display-boundary").First(&boundary).Error; err != nil {
		t.Fatal(err)
	}
	result, err := MarkSMSThreadReadByICCID(iccid, peer, boundary.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Marked != 3 || result.UnreadCount != 1 {
		t.Fatalf("result=%+v, want marked=3 unread=1", result)
	}
	var future SMS
	if err := DB.Where("content = ?", "future-inserted-first").First(&future).Error; err != nil {
		t.Fatal(err)
	}
	if future.Status != smsStatusRead {
		t.Fatalf("future status=%d, want read", future.Status)
	}
	var concurrent SMS
	if err := DB.Where("content = ?", "older-concurrent-insert").First(&concurrent).Error; err != nil {
		t.Fatal(err)
	}
	if concurrent.Status != smsStatusUnread {
		t.Fatalf("concurrent status=%d, want unread", concurrent.Status)
	}
}
