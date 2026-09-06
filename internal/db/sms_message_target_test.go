package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestSMSMessageTargetOutsideInboxPage(t *testing.T) {
	openTestDB(t)
	now := time.Now().Truncate(time.Second)
	for index := 0; index < 201; index++ {
		saveMessageTargetFixture(t, SMSRecord{Identity: SMSIdentity{ICCID: "card", IMSI: "imsi"}, Sender: fmt.Sprintf("peer-%d", index), Type: 1, Timestamp: now})
	}
	id := saveMessageTargetFixture(t, SMSRecord{Identity: SMSIdentity{ICCID: "card", IMSI: "imsi"}, Sender: "late", Type: 1, Timestamp: now.Add(-3 * time.Hour)})
	contacts, err := GetSMSContacts(200, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, contact := range contacts {
		if contact.Peer == "late" {
			t.Fatal("fixture is not outside the first page")
		}
	}
	target, err := GetSMSMessageTarget(context.Background(), id)
	if err != nil || target.Message.ID != id || target.Contact.Peer != "late" || len(target.Messages) != 1 {
		t.Fatalf("target=%+v err=%v", target, err)
	}
	if target.Contact.UnreadCount != 1 || target.Message.Status != 0 {
		t.Fatal("target read changed unread state")
	}
}

func TestSMSMessageTargetWindowKeepsLateMessageAndSIMScope(t *testing.T) {
	openTestDB(t)
	now := time.Now().Truncate(time.Second)
	identity := SMSIdentity{ICCID: "card-A", IMSI: "shared-imsi"}
	for index := 0; index < 81; index++ {
		saveMessageTargetFixture(t, SMSRecord{Identity: identity, Sender: "peer", Type: 1, Timestamp: now})
		saveMessageTargetFixture(t, SMSRecord{Identity: identity, Sender: "peer", Type: 1, Timestamp: now.Add(-4 * time.Hour)})
	}
	id := saveMessageTargetFixture(t, SMSRecord{Identity: identity, Sender: "peer", Type: 1, Timestamp: now.Add(-3 * time.Hour)})
	saveMessageTargetFixture(t, SMSRecord{Identity: SMSIdentity{ICCID: "card-B", IMSI: "shared-imsi"}, Sender: "peer", Type: 1, Timestamp: now.Add(-3 * time.Hour)})
	target, err := GetSMSMessageTarget(context.Background(), id)
	if err != nil || len(target.Messages) != 80 || !target.HasMore || target.Messages[0].ID != id {
		t.Fatalf("target=%+v err=%v", target, err)
	}
	for _, message := range target.Messages {
		if message.ICCID != "card-A" || message.Timestamp.After(target.Message.Timestamp) {
			t.Fatal("window crossed the SIM or target boundary")
		}
	}
	oldest := target.Messages[len(target.Messages)-1]
	older, err := GetSMSByICCIDAndPeer("card-A", "peer", 80, &oldest.Timestamp, oldest.ID)
	if err != nil || len(older) != 2 {
		t.Fatalf("older count=%d err=%v", len(older), err)
	}
}

func TestSMSMessageTargetMissingAndCanceled(t *testing.T) {
	openTestDB(t)
	if _, err := GetSMSMessageTarget(context.Background(), 999); !errors.Is(err, ErrSMSNotFound) {
		t.Fatalf("missing err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GetSMSMessageTarget(ctx, 1); err == nil {
		t.Fatal("canceled query succeeded")
	}
}

func saveMessageTargetFixture(t *testing.T, record SMSRecord) uint {
	t.Helper()
	if err := SaveSMSForIdentity(record); err != nil {
		t.Fatal(err)
	}
	var message SMS
	if err := DB.Order("id DESC").First(&message).Error; err != nil {
		t.Fatal(err)
	}
	return message.ID
}
