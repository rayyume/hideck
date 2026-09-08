package runtimecore

import (
	"errors"
	"math/rand/v2"
	"time"

	"github.com/iniwex5/vowifi-go/engine/ikev2"
	"github.com/iniwex5/vowifi-go/engine/swu"
)

// RFC 7296 section 3.15.4 recommends several minutes, not a fixed timer.
// Keep this local policy separate from IMS/RFC 5626 flow recovery.
const (
	ikeAddressRetryMin = 2 * time.Minute
	ikeAddressRetryMax = 4 * time.Minute
)

func ikeAddressRetryDelay(err error) (int64, bool) {
	if !isIKEAddressFailure(err) {
		return 0, false
	}
	jitter := rand.Int64N(int64(ikeAddressRetryMax - ikeAddressRetryMin))
	return int64(ikeAddressRetryMin) + jitter, true
}

func isIKEAddressFailure(err error) bool {
	var rejection *swu.IKEAuthError
	return errors.As(err, &rejection) && rejection.NotifyType == ikev2.INTERNAL_ADDRESS_FAILURE
}
