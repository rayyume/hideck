package pcsc

import (
	"context"
	"fmt"
	"strings"
)

func pinCardKey(iccid string) string {
	return strings.ToUpper(strings.TrimSpace(iccid))
}

func (gate *readerGate) pinRetryError(iccid string) error {
	gate.pinMu.RLock()
	defer gate.pinMu.RUnlock()
	failure := gate.pinFailures[pinCardKey(iccid)]
	if failure == nil {
		return nil
	}
	return fmt.Errorf("%w: %w", ErrPINRetryBlocked, failure)
}

func (gate *readerGate) blockPINRetry(iccid string, failure error) error {
	gate.pinMu.Lock()
	key := pinCardKey(iccid)
	if gate.pinFailures[key] == nil {
		gate.pinFailures[key] = failure
	}
	gate.pinMu.Unlock()
	return gate.pinRetryError(iccid)
}

func (gate *readerGate) allowPINRetry(iccid string) {
	gate.pinMu.Lock()
	delete(gate.pinFailures, pinCardKey(iccid))
	gate.pinMu.Unlock()
}

func (service *Service) PINFailure(reader Reader, iccid string) error {
	if service == nil {
		return ErrUnavailable
	}
	return service.readerLock(canonicalSelector(reader)).pinRetryError(iccid)
}

func (session *Session) verifyPIN(ctx context.Context, iccid, pin string) error {
	request := pinVerificationRequest{pin: pin, reference: 0x01}
	return session.verifyPINReference(ctx, iccid, request)
}

func (session *Session) verifyPINReference(ctx context.Context, iccid string, request pinVerificationRequest) error {
	if err := session.lock.pinRetryError(iccid); err != nil {
		return err
	}
	submitted, err := verifyPINReference(ctx, session.card, request)
	if err != nil && submitted {
		return session.lock.blockPINRetry(iccid, err)
	}
	return err
}

// AllowPINRetry clears the safety latch for the card currently inserted in a
// reader. Callers must invoke this only for an explicit user-requested retry.
func (service *Service) AllowPINRetry(ctx context.Context, selector Selector) (string, error) {
	session, err := service.OpenSession(ctx, selector)
	if err != nil {
		return "", err
	}
	defer session.Close()
	iccid, err := readICCID(ctx, session.card)
	if err != nil {
		return "", err
	}
	session.lock.allowPINRetry(iccid)
	return iccid, nil
}
