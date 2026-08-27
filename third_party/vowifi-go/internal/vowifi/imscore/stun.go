package imscore

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"net"
)

const (
	stunHeaderSize             = 20
	stunMagicCookie     uint32 = 0x2112A442
	stunBindingRequest  uint16 = 0x0001
	stunBindingSuccess  uint16 = 0x0101
	stunBindingError    uint16 = 0x0111
	stunAttrMappedAddr  uint16 = 0x0001
	stunAttrXORMapped   uint16 = 0x0020
	stunAttrFingerprint uint16 = 0x8028
	stunFingerprintXOR  uint32 = 0x5354554e
	stunFamilyIPv4             = 0x01
	stunFamilyIPv6             = 0x02
)

var (
	errSTUNMessage          = errors.New("imscore: invalid STUN message")
	errSTUNFingerprint      = errors.New("imscore: STUN FINGERPRINT mismatch")
	errSTUNMappedAddress    = errors.New("imscore: STUN mapped address is unavailable")
	errSTUNKeepalive        = errors.New("imscore: STUN keepalive")
	errSTUNKeepaliveTimeout = errors.New("imscore: STUN keepalive timeout")
	errSTUNMappedChanged    = errors.New("imscore: STUN XOR-MAPPED-ADDRESS changed")
)

type stunMessage struct {
	Type  uint16
	TxID  [12]byte
	raw   []byte
	attrs []stunAttribute
}

type stunAttribute struct {
	Type  uint16
	Value []byte
}

func isSTUNMessage(pkt []byte) bool {
	if len(pkt) < stunHeaderSize {
		return false
	}
	if pkt[0]&0xC0 != 0 {
		return false
	}
	if binary.BigEndian.Uint32(pkt[4:8]) != stunMagicCookie {
		return false
	}
	length := int(binary.BigEndian.Uint16(pkt[2:4]))
	return length >= 0 && len(pkt) >= stunHeaderSize+length
}

func stunTransactionID(pkt []byte) ([12]byte, bool) {
	var txID [12]byte
	if len(pkt) < stunHeaderSize {
		return txID, false
	}
	copy(txID[:], pkt[8:20])
	return txID, true
}

func buildSTUNBindingRequest(txID [12]byte) []byte {
	msg := make([]byte, stunHeaderSize)
	binary.BigEndian.PutUint16(msg[0:2], stunBindingRequest)
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID[:])
	return appendSTUNFingerprint(msg)
}

func buildSTUNBindingSuccess(txID [12]byte, mapped *net.UDPAddr) []byte {
	value := encodeXORMappedAddress(txID, mapped)
	attr := encodeSTUNTLV(stunAttrXORMapped, value)
	msg := make([]byte, stunHeaderSize+len(attr))
	binary.BigEndian.PutUint16(msg[0:2], stunBindingSuccess)
	binary.BigEndian.PutUint16(msg[2:4], uint16(len(attr)))
	binary.BigEndian.PutUint32(msg[4:8], stunMagicCookie)
	copy(msg[8:20], txID[:])
	copy(msg[20:], attr)
	return appendSTUNFingerprint(msg)
}

func appendSTUNFingerprint(msg []byte) []byte {
	out := append(msg, make([]byte, 8)...)
	binary.BigEndian.PutUint16(out[len(msg):], stunAttrFingerprint)
	binary.BigEndian.PutUint16(out[len(msg)+2:], 4)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(out)-stunHeaderSize))
	crc := crc32.ChecksumIEEE(out[:len(out)-4])
	binary.BigEndian.PutUint32(out[len(out)-4:], crc^stunFingerprintXOR)
	return out
}

func encodeSTUNTLV(attrType uint16, value []byte) []byte {
	padded := (len(value) + 3) &^ 3
	out := make([]byte, 4+padded)
	binary.BigEndian.PutUint16(out[0:2], attrType)
	binary.BigEndian.PutUint16(out[2:4], uint16(len(value)))
	copy(out[4:], value)
	return out
}

func encodeXORMappedAddress(txID [12]byte, addr *net.UDPAddr) []byte {
	if addr == nil {
		return nil
	}
	port := uint16(addr.Port) ^ uint16(stunMagicCookie>>16)
	if ip := addr.IP.To4(); ip != nil {
		out := make([]byte, 8)
		out[1] = stunFamilyIPv4
		binary.BigEndian.PutUint16(out[2:4], port)
		binary.BigEndian.PutUint32(out[4:8], binary.BigEndian.Uint32(ip)^stunMagicCookie)
		return out
	}
	ip := addr.IP.To16()
	if ip == nil {
		return nil
	}
	out := make([]byte, 20)
	out[1] = stunFamilyIPv6
	binary.BigEndian.PutUint16(out[2:4], port)
	var mask [16]byte
	binary.BigEndian.PutUint32(mask[0:4], stunMagicCookie)
	copy(mask[4:], txID[:])
	for i := 0; i < 16; i++ {
		out[4+i] = ip[i] ^ mask[i]
	}
	return out
}

func parseSTUNMessage(pkt []byte) (*stunMessage, error) {
	if !isSTUNMessage(pkt) {
		return nil, errSTUNMessage
	}
	length := int(binary.BigEndian.Uint16(pkt[2:4]))
	raw := pkt[:stunHeaderSize+length]
	if err := verifySTUNFingerprint(raw); err != nil {
		return nil, err
	}
	msg := &stunMessage{
		Type: binary.BigEndian.Uint16(raw[0:2]),
		raw:  raw,
	}
	copy(msg.TxID[:], raw[8:20])
	offset := stunHeaderSize
	end := stunHeaderSize + length
	for offset+4 <= end {
		attrType := binary.BigEndian.Uint16(raw[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(raw[offset+2 : offset+4]))
		valueStart := offset + 4
		if valueStart+attrLen > end {
			return nil, errSTUNMessage
		}
		msg.attrs = append(msg.attrs, stunAttribute{
			Type:  attrType,
			Value: append([]byte(nil), raw[valueStart:valueStart+attrLen]...),
		})
		offset = valueStart + attrLen
		offset = (offset + 3) &^ 3
	}
	return msg, nil
}

func verifySTUNFingerprint(raw []byte) error {
	offset := stunHeaderSize
	end := len(raw)
	lastType, lastValueStart := uint16(0), -1
	for offset+4 <= end {
		attrType := binary.BigEndian.Uint16(raw[offset : offset+2])
		attrLen := int(binary.BigEndian.Uint16(raw[offset+2 : offset+4]))
		valueStart := offset + 4
		if valueStart+attrLen > end {
			return errSTUNMessage
		}
		lastType, lastValueStart = attrType, valueStart
		offset = valueStart + attrLen
		offset = (offset + 3) &^ 3
	}
	if lastType != stunAttrFingerprint {
		return nil
	}
	if lastValueStart < 0 || lastValueStart+4 > len(raw) {
		return errSTUNFingerprint
	}
	got := binary.BigEndian.Uint32(raw[lastValueStart : lastValueStart+4])
	want := crc32.ChecksumIEEE(raw[:lastValueStart]) ^ stunFingerprintXOR
	if got != want {
		return errSTUNFingerprint
	}
	return nil
}

func stunMappedAddress(msg *stunMessage) (*net.UDPAddr, error) {
	if msg == nil {
		return nil, errSTUNMappedAddress
	}
	for _, attr := range msg.attrs {
		switch attr.Type {
		case stunAttrXORMapped:
			addr, err := decodeXORMappedAddress(msg.TxID, attr.Value)
			if err == nil {
				return addr, nil
			}
		case stunAttrMappedAddr:
			addr, err := decodeMappedAddress(attr.Value)
			if err == nil {
				return addr, nil
			}
		}
	}
	return nil, errSTUNMappedAddress
}

func decodeXORMappedAddress(txID [12]byte, value []byte) (*net.UDPAddr, error) {
	addr, err := decodeMappedAddress(value)
	if err != nil {
		return nil, err
	}
	addr.Port ^= int(uint16(stunMagicCookie >> 16))
	if ip := addr.IP.To4(); ip != nil {
		x := binary.BigEndian.Uint32(ip) ^ stunMagicCookie
		out := make(net.IP, 4)
		binary.BigEndian.PutUint32(out, x)
		addr.IP = out
		return addr, nil
	}
	ip := addr.IP.To16()
	if ip == nil {
		return nil, errSTUNMappedAddress
	}
	var mask [16]byte
	binary.BigEndian.PutUint32(mask[0:4], stunMagicCookie)
	copy(mask[4:], txID[:])
	out := make(net.IP, 16)
	for i := 0; i < 16; i++ {
		out[i] = ip[i] ^ mask[i]
	}
	addr.IP = out
	return addr, nil
}

func decodeMappedAddress(value []byte) (*net.UDPAddr, error) {
	if len(value) < 4 {
		return nil, errSTUNMappedAddress
	}
	family := value[1]
	port := int(binary.BigEndian.Uint16(value[2:4]))
	switch family {
	case stunFamilyIPv4:
		if len(value) < 8 {
			return nil, errSTUNMappedAddress
		}
		ip := make(net.IP, 4)
		copy(ip, value[4:8])
		return &net.UDPAddr{IP: ip, Port: port}, nil
	case stunFamilyIPv6:
		if len(value) < 20 {
			return nil, errSTUNMappedAddress
		}
		ip := make(net.IP, 16)
		copy(ip, value[4:20])
		return &net.UDPAddr{IP: ip, Port: port}, nil
	default:
		return nil, errSTUNMappedAddress
	}
}

func sameUDPAddr(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Port == b.Port && a.IP.Equal(b.IP)
}
