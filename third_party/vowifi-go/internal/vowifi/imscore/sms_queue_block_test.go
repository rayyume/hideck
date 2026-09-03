package imscore

import (
	"testing"
	"time"
)

// The SMSC allocates a fresh RP-MR per delivery attempt, so a redelivery is
// only recognisable by what survives it.
func TestMTIdentitySurvivesARedelivery(t *testing.T) {
	scts := time.Date(2026, 9, 3, 7, 35, 26, 0, time.UTC)
	first := inboundSMS{
		sender: "Vodafone", targetURI: "tel:+447700900123",
		content: "OTP is 'MVMNU'", timestamp: scts, rpMR: 153,
	}
	redelivery := first
	redelivery.rpMR = 208

	if mtSMSIdentity(first) != mtSMSIdentity(redelivery) {
		t.Fatal("a fresh RP-MR changed the identity, so redeliveries cannot be matched")
	}
	if buildMTSMSFingerprint(first, "") == buildMTSMSFingerprint(redelivery, "") {
		t.Fatal("expected the dedup fingerprint to differ; this test guards why identity exists")
	}

	other := first
	other.content = "OTP is '0F9M4'"
	if mtSMSIdentity(first) == mtSMSIdentity(other) {
		t.Fatal("different messages shared an identity")
	}
}

func TestRedeliveryOfARejectedReportIsReportedThenCleared(t *testing.T) {
	service, _, _ := newInboundSMSTestService(t)
	identity := "message-held-at-the-queue-head"

	// Nothing recorded yet: a first delivery must stay quiet.
	service.reportMTQueueBlocked(identity)
	if len(service.unackedMT) != 0 {
		t.Fatalf("unackedMT = %d, want the map untouched", len(service.unackedMT))
	}

	service.rememberRejectedMTReport(identity)
	service.rememberRejectedMTReport(identity)
	entry, held := service.unackedMT[identity]
	if !held {
		t.Fatal("a rejected report left no record of the held message")
	}
	if entry.rejections != 2 {
		t.Fatalf("rejections = %d, want 2", entry.rejections)
	}
	if entry.firstRejectedAt.IsZero() {
		t.Fatal("no timestamp to measure how long the queue has been blocked")
	}
	first := entry.firstRejectedAt
	service.rememberRejectedMTReport(identity)
	if got := service.unackedMT[identity].firstRejectedAt; !got.Equal(first) {
		t.Fatal("a later rejection moved the start of the block")
	}

	service.clearRejectedMTReport(identity)
	if _, still := service.unackedMT[identity]; still {
		t.Fatal("an accepted report left the message marked as held")
	}
}
