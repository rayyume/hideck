package pcsc

import (
	"context"
	"errors"
	"fmt"
)

type pinAccessRequest struct {
	iccid     string
	aid       []byte
	pin       string
	operation func() error
}

func (session *Session) withPINAccess(ctx context.Context, request pinAccessRequest) error {
	if err := session.lock.pinRetryError(request.iccid); err != nil {
		return err
	}
	accessErr := request.operation()
	if !errors.Is(accessErr, ErrSecurityStatus) {
		return accessErr
	}
	fcp, err := selectApplication(ctx, session.card, request.aid)
	if err != nil {
		return errors.Join(accessErr, err)
	}
	reference, err := activePINReference(fcp)
	if err != nil {
		return errors.Join(accessErr, fmt.Errorf("%w: %v", ErrPINStatusUnknown, err))
	}
	verification := pinVerificationRequest{pin: request.pin, reference: reference}
	if err := session.verifyPINReference(ctx, request.iccid, verification); err != nil {
		return errors.Join(accessErr, fmt.Errorf("pcsc: FCP PIN reference=%02X enabled/required: %w", reference, err))
	}
	if _, err := selectApplication(ctx, session.card, request.aid); err != nil {
		return err
	}
	return request.operation()
}

type pinTLV struct {
	tag   byte
	value []byte
}

func parsePINTLVs(data []byte) ([]pinTLV, error) {
	var result []pinTLV
	for len(data) != 0 {
		if len(data) < 2 || data[0]&0x1F == 0x1F {
			return nil, errors.New("unsupported or truncated FCP/PIN template tag")
		}
		length, consumed, ok := decodeTLVLength(data[1:])
		if !ok || 1+consumed+length > len(data) {
			return nil, errors.New("truncated FCP/PIN template value")
		}
		end := 1 + consumed + length
		result = append(result, pinTLV{tag: data[0], value: data[1+consumed : end]})
		data = data[end:]
	}
	return result, nil
}

func activePINReference(fcp []byte) (byte, error) {
	outer, err := parsePINTLVs(fcp)
	if err != nil {
		return 0, err
	}
	if len(outer) != 1 || outer[0].tag != 0x62 {
		return 0, errors.New("USIM selection did not return an FCP template")
	}
	fields, err := parsePINTLVs(outer[0].value)
	if err != nil {
		return 0, err
	}
	var template []byte
	for _, field := range fields {
		if field.tag == 0xC6 {
			if template != nil {
				return 0, errors.New("duplicate PIN status template")
			}
			template = field.value
		}
	}
	entries, err := parsePINTLVs(template)
	if err != nil {
		return 0, err
	}
	if len(entries) < 2 || entries[0].tag != 0x90 || len(entries[0].value) == 0 {
		return 0, errors.New("missing PIN status bitmap or key references")
	}
	bitmap := entries[0].value
	index := 0
	var candidate byte
	var usage []byte
	seen := make(map[byte]bool)
	for _, entry := range entries[1:] {
		switch entry.tag {
		case 0x95:
			if usage != nil || len(entry.value) != 1 || (entry.value[0] != 0 && entry.value[0] != 8) {
				return 0, errors.New("invalid PIN usage qualifier")
			}
			usage = entry.value
		case 0x83:
			if len(entry.value) != 1 || index >= len(bitmap)*8 {
				return 0, errors.New("PIN key reference does not match status bitmap")
			}
			reference := entry.value[0]
			primary := (reference >= 1 && reference <= 8) || reference == 0x11
			secondary := reference >= 0x81 && reference <= 0x88
			if (!primary && !secondary) || seen[reference] || (reference == 0x11 && usage == nil) {
				return 0, errors.New("unsupported or ambiguous PIN key reference")
			}
			seen[reference] = true
			enabled := bitmap[index/8]&(0x80>>uint(index%8)) != 0
			required := usage == nil || usage[0] == 8
			if primary && enabled && required {
				if candidate != 0 {
					return 0, errors.New("multiple enabled primary PINs; refusing to choose one")
				}
				candidate = reference
			}
			index++
			usage = nil
		default:
			return 0, errors.New("unexpected PIN status template field")
		}
	}
	if usage != nil || candidate == 0 {
		return 0, errors.New("no unambiguous enabled/required user PIN in FCP; refusing verification")
	}
	return candidate, nil
}
