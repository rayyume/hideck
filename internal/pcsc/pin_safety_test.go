package pcsc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPINUnknownStateNeverSubmitsConfiguredPIN(test *testing.T) {
	for _, status := range []uint16{0x0000, 0x6700, 0x6982, 0x6983, 0x6984, 0x6985, 0x9804, 0x6A86, 0x6D00, 0x6F00} {
		test.Run(fmt.Sprintf("%04X", status), func(test *testing.T) {
			card := &scriptedCard{replies: []scriptedReply{{sw: status}}}
			err := verifyPIN(context.Background(), card, "1234")
			var pinError *PINError
			if !errors.Is(err, ErrPINStatusUnknown) || !errors.As(err, &pinError) || pinError.Status != status {
				test.Fatalf("unexpected status error: %v", err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("SW=%04X", status)) {
				test.Fatalf("raw status lost: %v", err)
			}
			if len(card.calls) != 1 {
				test.Fatalf("submitted PIN with unknown state: %d APDUs", len(card.calls))
			}
		})
	}
}

func TestPINLowRetriesNeverSubmitsPIN(test *testing.T) {
	for _, status := range []uint16{0x63C0, 0x63C1, 0x63C2} {
		card := &scriptedCard{replies: []scriptedReply{{sw: status}}}
		if err := verifyPIN(context.Background(), card, "1234"); !errors.Is(err, ErrPINTriesLow) {
			test.Fatalf("status %04X: unexpected error %v", status, err)
		}
		if len(card.calls) != 1 {
			test.Fatalf("status %04X: submitted PIN", status)
		}
	}
}

func TestPINVerificationPreservesFailureStatus(test *testing.T) {
	for _, status := range []uint16{0x63C2, 0x6983, 0x6984, 0x6A80, 0x6D00, 0x6F00} {
		card := &scriptedCard{replies: []scriptedReply{{sw: 0x63C3}, {sw: status}}}
		err := verifyPIN(context.Background(), card, "1234")
		var pinError *PINError
		if !errors.As(err, &pinError) || pinError.Status != status || !strings.Contains(err.Error(), fmt.Sprintf("SW=%04X", status)) {
			test.Fatalf("status %04X lost from error: %v", status, err)
		}
		if errors.Is(err, ErrPINRejected) != (status == 0x63C2) {
			test.Fatalf("status %04X incorrectly classified: %v", status, err)
		}
		if len(card.calls) != 2 || strings.Contains(err.Error(), "1234") {
			test.Fatal("unexpected command count or PIN leaked in error")
		}
	}
}

func TestPINSuccessfulVerificationRemainsAvailable(test *testing.T) {
	for _, replies := range [][]scriptedReply{
		{{sw: 0x9000}},
		{{sw: 0x63C3}, {sw: 0x9000}},
	} {
		card := &scriptedCard{replies: replies}
		service := NewWithBackend(&scriptedBackend{card: card})
		session, err := service.OpenSession(context.Background(), Selector{ReaderName: "Reader A"})
		if err != nil {
			test.Fatal(err)
		}
		if err := session.verifyPIN(context.Background(), "1234"); err != nil {
			test.Fatal(err)
		}
		if err := session.Close(); err != nil {
			test.Fatal(err)
		}
		if err := service.PINFailure(Reader{Name: "Reader A", USBPath: "1-2"}); err != nil {
			test.Fatalf("successful verification disabled retries: %v", err)
		}
		if len(card.calls) != len(replies) {
			test.Fatalf("unexpected APDU count: %d", len(card.calls))
		}
	}
}

func TestPINFailureBlocksLaterSessionsAndAliases(test *testing.T) {
	transportError := errors.New("native transport failed")
	for _, scenario := range []struct {
		name    string
		pin     string
		replies []scriptedReply
	}{
		{name: "missing PIN", replies: []scriptedReply{{sw: 0x63C3}}},
		{name: "unknown state", pin: "1234", replies: []scriptedReply{{sw: 0x6983}}},
		{name: "rejected", pin: "1234", replies: []scriptedReply{{sw: 0x63C3}, {sw: 0x63C2}}},
		{name: "unknown verification result", pin: "1234", replies: []scriptedReply{{sw: 0x63C3}, {sw: 0x6984}}},
		{name: "status transport error", pin: "1234", replies: []scriptedReply{{err: transportError}}},
		{name: "verification transport error", pin: "1234", replies: []scriptedReply{{sw: 0x63C3}, {err: transportError}}},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			card := &scriptedCard{replies: scenario.replies}
			service := NewWithBackend(&scriptedBackend{card: card})
			for attempt, selector := range []Selector{{ReaderName: "Reader A"}, {USBPath: "1-2"}} {
				session, err := service.OpenSession(context.Background(), selector)
				if err != nil {
					test.Fatal(err)
				}
				pin := scenario.pin
				if attempt > 0 {
					pin = "5678"
				}
				err = session.verifyPIN(context.Background(), pin)
				if closeErr := session.Close(); closeErr != nil {
					test.Fatal(closeErr)
				}
				if !errors.Is(err, ErrPINRetryBlocked) {
					test.Fatalf("attempt %d did not block retries: %v", attempt, err)
				}
				if len(card.calls) != len(scenario.replies) {
					test.Fatalf("attempt %d submitted extra APDUs: %d", attempt, len(card.calls))
				}
			}
			if err := service.PINFailure(Reader{Name: "Reader A", USBPath: "1-2"}); !errors.Is(err, ErrPINRetryBlocked) {
				test.Fatalf("missing reader-level failure: %v", err)
			}
			if err := service.PINFailure(Reader{Name: "Reader B", USBPath: "1-3"}); err != nil {
				test.Fatalf("failure leaked into unrelated reader: %v", err)
			}
		})
	}
}

func TestPINFailureGuardCoversIdentityReadinessAndAuthentication(test *testing.T) {
	selection := []scriptedReply{
		{sw: 0x9000},
		{sw: 0x9000},
		{data: []byte{0x98, 0x10, 0x32, 0x54, 0x76, 0x98, 0x10, 0x32, 0x54, 0x76}, sw: 0x9000},
		{sw: 0x9000},
		{sw: 0x9000},
		{data: []byte{0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}, sw: 0x9000},
		{sw: 0x9000},
	}
	card := &scriptedCard{}
	card.replies = append(card.replies, selection...)
	card.replies = append(card.replies,
		scriptedReply{sw: 0x9000},
		scriptedReply{sw: 0x6982},
		scriptedReply{data: []byte{0x62, 0x08, 0xC6, 0x06, 0x90, 0x01, 0x80, 0x83, 0x01, 0x01}, sw: 0x9000},
	)
	card.replies = append(card.replies, scriptedReply{sw: 0x63C3}, scriptedReply{sw: 0x6984})
	card.replies = append(card.replies, selection...)
	card.replies = append(card.replies, scriptedReply{sw: 0x9000}, scriptedReply{sw: 0x6982})
	card.replies = append(card.replies, selection...)
	service := NewWithBackend(&scriptedBackend{card: card})
	selector := Selector{ReaderName: "Reader A"}
	identity, err := service.ReadIdentity(context.Background(), selector, "1234")
	if identity.ICCID == "" || !errors.Is(err, ErrPINVerification) || !errors.Is(err, ErrPINRetryBlocked) {
		test.Fatalf("unexpected identity result: %v", err)
	}
	if _, err := service.CheckReady(context.Background(), selector, identity.ICCID, "1234"); !errors.Is(err, ErrPINRetryBlocked) {
		test.Fatalf("readiness check bypassed guard: %v", err)
	}
	if _, err := service.Authenticate(context.Background(), selector, identity.ICCID, "1234", AKAChallenge{}); !errors.Is(err, ErrPINRetryBlocked) {
		test.Fatalf("authentication bypassed guard: %v", err)
	}
	queries, submissions := 0, 0
	for _, command := range card.calls {
		if command[1] == 0x88 {
			test.Fatal("authentication APDU was sent despite PIN failure")
		}
		if command[1] == 0x20 {
			if len(command) == 5 {
				queries++
			} else {
				submissions++
			}
		}
	}
	if queries != 1 || submissions != 1 {
		test.Fatalf("PIN retried: queries=%d submissions=%d", queries, submissions)
	}
}
