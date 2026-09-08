package runtimecore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/engine/ikev2"
	"github.com/iniwex5/vowifi-go/engine/swu"
)

func TestAddressFailureUsesIndependentMinutesRetry(t *testing.T) {
	cause := &swu.IKEAuthError{NotifyType: ikev2.INTERNAL_ADDRESS_FAILURE}
	err := fmt.Errorf("start IMS tunnel: %w", errors.Join(cause, errors.New("delete failed")))
	for attempt := 0; attempt < 100; attempt++ {
		delay, next := retryDecision(err, attempt, func(int) int64 { return int64(30 * time.Second) })
		if delay < int64(2*time.Minute) || delay > int64(4*time.Minute) || next != attempt+1 {
			t.Fatalf("attempt %d: delay=%v next=%d", attempt, time.Duration(delay), next)
		}
	}
	if delay, _ := retryDecision(err, 0, nil); delay < int64(2*time.Minute) {
		t.Fatalf("nil host delay bypassed address failure wait: %v", time.Duration(delay))
	}
}

func TestAddressFailureDoesNotShortenOtherRetryConstraints(t *testing.T) {
	cause := &swu.IKEAuthError{NotifyType: ikev2.INTERNAL_ADDRESS_FAILURE}
	longDelay := int64(10 * time.Minute)
	if delay, _ := retryDecision(cause, 2, func(int) int64 { return longDelay }); delay != longDelay {
		t.Fatalf("host delay shortened: %v", time.Duration(delay))
	}
	for _, deadline := range []time.Duration{time.Second, time.Hour} {
		err := errors.Join(cause, scheduledRetryTestError{retryAt: time.Now().Add(deadline)})
		delay, _ := retryDecision(err, 0, nil)
		if delay < int64(2*time.Minute) || (deadline == time.Hour && delay < int64(59*time.Minute)) {
			t.Fatalf("deadline %v: delay=%v", deadline, time.Duration(delay))
		}
	}
}

func TestAddressFailurePolicyDoesNotChangeOtherErrors(t *testing.T) {
	for _, err := range []error{
		&swu.IKEAuthError{NotifyType: ikev2.AUTHENTICATION_FAILED},
		context.DeadlineExceeded,
		errors.New("connection reset by peer"), errors.New("EOF"),
		errors.New("SIP 488"), errors.New("SIP 503"),
		errors.New("swu: IKE_AUTH rejected with INTERNAL_ADDRESS_FAILURE (36)"),
	} {
		delay, next := retryDecision(err, 1, func(int) int64 { return int64(30 * time.Second) })
		if delay != int64(30*time.Second) || next != 2 {
			t.Fatalf("%v: changed delay=%v next=%d", err, time.Duration(delay), next)
		}
	}
}

func TestRunLoopAddressFailureWaitIsCancelable(t *testing.T) {
	for _, cleanupErr := range []error{nil, context.Canceled, context.DeadlineExceeded} {
		t.Run(fmt.Sprint(cleanupErr), func(t *testing.T) { assertAddressFailureLoopCancelable(t, cleanupErr) })
	}
}

func assertAddressFailureLoopCancelable(t *testing.T, cleanupErr error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls, retries := 0, 0
	err := RunLoop(ctx, nil, nil, func(attempt int, delay int64) {
		retries++
		if attempt != 1 || delay < int64(2*time.Minute) {
			t.Errorf("retry attempt=%d delay=%v", attempt, time.Duration(delay))
		}
		cancel()
	}, func(context.Context) error {
		calls++
		if calls > 1 {
			cancel()
		}
		return errors.Join(&swu.IKEAuthError{NotifyType: ikev2.INTERNAL_ADDRESS_FAILURE}, cleanupErr)
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || retries != 1 {
		t.Fatalf("err=%v calls=%d retries=%d", err, calls, retries)
	}
}
