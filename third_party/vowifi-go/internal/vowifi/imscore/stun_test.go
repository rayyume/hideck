package imscore

import (
	"net"
	"testing"
)

func TestSTUNBindingRoundTripXORMappedIPv4(t *testing.T) {
	var txID [12]byte
	copy(txID[:], []byte{0xb7, 0xe7, 0xa7, 0x01, 0xbc, 0x34, 0xd6, 0x86, 0xfa, 0x87, 0xdf, 0xae})
	want := &net.UDPAddr{IP: net.ParseIP("192.0.2.1").To4(), Port: 32853}
	pkt := buildSTUNBindingSuccess(txID, want)
	if !isSTUNMessage(pkt) {
		t.Fatal("success response is not STUN")
	}
	msg, err := parseSTUNMessage(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != stunBindingSuccess || msg.TxID != txID {
		t.Fatalf("message type=%d txid=%x", msg.Type, msg.TxID)
	}
	got, err := stunMappedAddress(msg)
	if err != nil {
		t.Fatal(err)
	}
	if !sameUDPAddr(got, want) {
		t.Fatalf("mapped = %v, want %v", got, want)
	}
}

func TestSTUNFingerprintRejectsTamperedPacket(t *testing.T) {
	var txID [12]byte
	pkt := buildSTUNBindingRequest(txID)
	pkt[len(pkt)-1] ^= 0xff
	if err := verifySTUNFingerprint(pkt); err == nil {
		t.Fatal("tampered FINGERPRINT accepted")
	}
}

func TestBuildSTUNBindingRequestHasFingerprint(t *testing.T) {
	var txID [12]byte
	copy(txID[:], []byte("txid1234abcd"))
	pkt := buildSTUNBindingRequest(txID)
	msg, err := parseSTUNMessage(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Type != stunBindingRequest {
		t.Fatalf("type = %d", msg.Type)
	}
	if len(pkt) != stunHeaderSize+8 {
		t.Fatalf("length = %d", len(pkt))
	}
}

func TestIsSTUNMessageRejectsSIP(t *testing.T) {
	if isSTUNMessage([]byte("OPTIONS sip:pcscf SIP/2.0\r\n\r\n")) {
		t.Fatal("SIP OPTIONS classified as STUN")
	}
	if isSTUNMessage([]byte("SIP/2.0 200 OK\r\n\r\n")) {
		t.Fatal("SIP response classified as STUN")
	}
}
