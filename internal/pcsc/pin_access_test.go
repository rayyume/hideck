package pcsc

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func pinTestFCP(bitmap byte, fields ...byte) []byte {
	content := append([]byte{0x90, 0x01, bitmap}, fields...)
	return append([]byte{0x62, byte(len(content) + 2), 0xC6, byte(len(content))}, content...)
}

func TestActivePINReference(test *testing.T) {
	for _, scenario := range []struct {
		name string
		fcp  []byte
		want byte
	}{
		{"PIN1", pinTestFCP(0x80, 0x83, 0x01, 0x01), 0x01},
		{"application PIN reference 02", pinTestFCP(0x80, 0x83, 0x01, 0x02), 0x02},
		{"UPIN", pinTestFCP(0x80, 0x95, 0x01, 0x08, 0x83, 0x01, 0x11), 0x11},
		{"bitmap includes secondary PIN", pinTestFCP(0x20, 0x83, 0x01, 0x01, 0x83, 0x01, 0x81, 0x83, 0x01, 0x02), 0x02},
		{"disabled", pinTestFCP(0x00, 0x83, 0x01, 0x01), 0},
		{"not required", pinTestFCP(0x80, 0x95, 0x01, 0x00, 0x83, 0x01, 0x01), 0},
		{"unused UPIN", pinTestFCP(0x80, 0x95, 0x01, 0x00, 0x83, 0x01, 0x11), 0},
		{"UPIN missing qualifier", pinTestFCP(0x80, 0x83, 0x01, 0x11), 0},
		{"secondary only", pinTestFCP(0x80, 0x83, 0x01, 0x81), 0},
		{"ambiguous", pinTestFCP(0xC0, 0x83, 0x01, 0x01, 0x83, 0x01, 0x02), 0},
		{"duplicate", pinTestFCP(0x80, 0x83, 0x01, 0x01, 0x83, 0x01, 0x01), 0},
		{"invalid qualifier", pinTestFCP(0x80, 0x95, 0x01, 0x80, 0x83, 0x01, 0x01), 0},
		{"dangling qualifier", pinTestFCP(0x80, 0x83, 0x01, 0x01, 0x95, 0x01, 0x08), 0},
		{"empty", nil, 0},
		{"no PIN template", []byte{0x62, 0}, 0},
		{"truncated", []byte{0x62, 8, 0xC6}, 0},
		{"truncated PIN key", pinTestFCP(0x80, 0x83, 0x02, 0x01), 0},
	} {
		test.Run(scenario.name, func(test *testing.T) {
			actual, err := activePINReference(scenario.fcp)
			if scenario.want == 0 {
				if err == nil {
					test.Fatalf("unsafe FCP accepted with reference %02X", actual)
				}
			} else if err != nil || actual != scenario.want {
				test.Fatalf("reference=%02X err=%v, want %02X", actual, err, scenario.want)
			}
		})
	}
}

type accessTestCard struct {
	selected     uint16
	fcp          []byte
	imsiStatus   uint16
	authStatus   uint16
	queryStatus  uint16
	verifyStatus uint16
	verified     bool
	calls        [][]byte
}

func (card *accessTestCard) Close() error { return nil }

func (card *accessTestCard) Transmit(_ context.Context, command []byte) ([]byte, uint16, error) {
	card.calls = append(card.calls, append([]byte(nil), command...))
	switch command[1] {
	case 0xA4:
		if command[2] == 4 {
			card.selected = 0
			return card.fcp, 0x9000, nil
		}
		card.selected = uint16(command[5])<<8 | uint16(command[6])
		if card.selected != 0x3F00 && card.selected != 0x2FE2 && card.selected != 0x2F00 && card.selected != 0x6F07 {
			return nil, 0x6A82, nil
		}
		return nil, 0x9000, nil
	case 0xB2:
		return []byte{0x4F, 0x07, 0xA0, 0x00, 0x00, 0x00, 0x87, 0x10, 0x02}, 0x9000, nil
	case 0xB0:
		if card.selected == 0x2FE2 {
			return []byte{0x98, 0x10, 0x32, 0x54, 0x76, 0x98, 0x10, 0x32, 0x54, 0x76}, 0x9000, nil
		}
		if card.selected == 0x6F07 {
			if card.imsiStatus != 0 && !card.verified {
				return nil, card.imsiStatus, nil
			}
			return []byte{0x08, 0x19, 0x32, 0x54, 0x76, 0x98, 0x10, 0x32, 0x54}, 0x9000, nil
		}
	case 0x20:
		if len(command) == 5 {
			return nil, card.queryStatus, nil
		}
		card.verified = card.verifyStatus == 0x9000
		return nil, card.verifyStatus, nil
	case 0x88:
		if card.authStatus != 0 && !card.verified {
			return nil, card.authStatus, nil
		}
		data := []byte{0xDB, 4, 1, 2, 3, 4, 16}
		data = append(data, bytes.Repeat([]byte{0xAA}, 16)...)
		data = append(data, 16)
		data = append(data, bytes.Repeat([]byte{0xBB}, 16)...)
		return data, 0x9000, nil
	}
	return nil, 0, errors.New("unexpected test APDU")
}

type accessTestBackend struct{ card *accessTestCard }

func (backend *accessTestBackend) Readers(context.Context) ([]Reader, error) {
	return []Reader{{Name: "Reader A", USBPath: "1-2", CardPresent: true}}, nil
}

func (backend *accessTestBackend) Open(context.Context, Selector) (Card, error) {
	return backend.card, nil
}

func TestReadableIdentityNeverQueriesOrVerifiesPIN(test *testing.T) {
	for _, pin := range []string{"", "1234"} {
		card := &accessTestCard{fcp: pinTestFCP(0, 0x83, 1, 1), queryStatus: 0x63C3}
		service := NewWithBackend(&accessTestBackend{card: card})
		selector := Selector{ReaderName: "Reader A"}
		identity, err := service.ReadIdentity(context.Background(), selector, pin)
		if err != nil || identity.IMSI != "123456789012345" || identity.ICCID == "" || identity.PINRequired {
			test.Fatalf("readable identity rejected: %v", err)
		}
		if _, err := service.CheckReady(context.Background(), selector, identity.ICCID, pin); err != nil {
			test.Fatalf("readiness rejected: %v", err)
		}
		if _, err := service.Authenticate(context.Background(), selector, identity.ICCID, pin, AKAChallenge{}); err != nil {
			test.Fatalf("authentication rejected: %v", err)
		}
		for _, command := range card.calls {
			if command[1] == 0x20 {
				test.Fatal("unnecessary PIN operation despite readable identity and accepted authentication")
			}
		}
	}
}

func TestAccessDenialRequiresFCPBeforePIN(test *testing.T) {
	for _, fcp := range [][]byte{nil, pinTestFCP(0, 0x83, 1, 1), pinTestFCP(0xC0, 0x83, 1, 1, 0x83, 1, 2)} {
		card := &accessTestCard{fcp: fcp, imsiStatus: 0x6982, queryStatus: 0x63C3}
		service := NewWithBackend(&accessTestBackend{card: card})
		_, err := service.ReadIdentity(context.Background(), Selector{ReaderName: "Reader A"}, "1234")
		if !errors.Is(err, ErrPINStatusUnknown) || errors.Is(err, ErrPINRetryBlocked) || !strings.Contains(err.Error(), "read EF_IMSI") || !strings.Contains(err.Error(), "SW=6982") {
			test.Fatalf("missing access/FCP diagnostics: %v", err)
		}
		if _, retryErr := service.ReadIdentity(context.Background(), Selector{ReaderName: "Reader A"}, "1234"); errors.Is(retryErr, ErrPINRetryBlocked) {
			test.Fatalf("FCP failure blocked a later safe retry: %v", retryErr)
		}
		for _, command := range card.calls {
			if command[1] == 0x20 {
				test.Fatal("PIN operation sent without an unambiguous enabled PIN")
			}
		}
	}
}

func TestMissingPINCanRecoverAfterPINIsConfigured(test *testing.T) {
	card := &accessTestCard{
		fcp: pinTestFCP(0x80, 0x83, 1, 1), imsiStatus: 0x6982,
		queryStatus: 0x63C3, verifyStatus: 0x9000,
	}
	service := NewWithBackend(&accessTestBackend{card: card})
	selector := Selector{ReaderName: "Reader A"}
	identity, err := service.ReadIdentity(context.Background(), selector, "")
	if identity.ICCID == "" || !errors.Is(err, ErrPINRequired) || errors.Is(err, ErrPINRetryBlocked) {
		test.Fatalf("missing PIN was incorrectly latched: identity=%+v err=%v", identity, err)
	}
	identity, err = service.ReadIdentity(context.Background(), selector, "1234")
	if err != nil || identity.IMSI != "123456789012345" {
		test.Fatalf("configured PIN did not recover access: identity=%+v err=%v", identity, err)
	}
	submissions := 0
	for _, command := range card.calls {
		if command[1] == 0x20 && len(command) > 5 {
			submissions++
		}
	}
	if submissions != 1 {
		test.Fatalf("PIN submissions=%d want 1", submissions)
	}
}

func TestNonSecurityReadFailureNeverTriggersPIN(test *testing.T) {
	card := &accessTestCard{fcp: pinTestFCP(0x80, 0x83, 1, 1), imsiStatus: 0x6A82}
	service := NewWithBackend(&accessTestBackend{card: card})
	_, err := service.ReadIdentity(context.Background(), Selector{ReaderName: "Reader A"}, "1234")
	if err == nil || !strings.Contains(err.Error(), "SW=6A82") || errors.Is(err, ErrPINRequired) {
		test.Fatalf("incorrectly classified file error: %v", err)
	}
	for _, command := range card.calls {
		if command[1] == 0x20 {
			test.Fatal("PIN operation sent for a non-security file error")
		}
	}
}

func TestAccessDenialUsesFCPPINReferenceAndRetriesOnce(test *testing.T) {
	for _, reference := range []byte{0x01, 0x02, 0x11} {
		for _, authenticate := range []bool{false, true} {
			card := &accessTestCard{
				fcp:         pinTestFCP(0x80, 0x95, 1, 8, 0x83, 1, reference),
				queryStatus: 0x63C3, verifyStatus: 0x9000,
			}
			service := NewWithBackend(&accessTestBackend{card: card})
			selector := Selector{ReaderName: "Reader A"}
			var err error
			if authenticate {
				card.authStatus = 0x6982
				_, err = service.Authenticate(context.Background(), selector, "", "1234", AKAChallenge{})
			} else {
				card.imsiStatus = 0x6982
				_, err = service.ReadIdentity(context.Background(), selector, "1234")
			}
			if err != nil {
				test.Fatalf("reference %02X authenticate=%v: %v", reference, authenticate, err)
			}
			queries, submissions := 0, 0
			for _, command := range card.calls {
				if command[1] != 0x20 {
					continue
				}
				if command[3] != reference {
					test.Fatalf("used wrong PIN reference: %02X", command[3])
				}
				if len(command) == 5 {
					queries++
				} else {
					submissions++
				}
			}
			if queries != 1 || submissions != 1 {
				test.Fatalf("PIN operations: query=%d submit=%d", queries, submissions)
			}
		}
	}
}
