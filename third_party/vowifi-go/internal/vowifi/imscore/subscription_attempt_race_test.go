package imscore

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/emiago/sipgo/sip"
)

func TestSubscriptionTerminationInvalidatesBuiltRefresh(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		for _, reason := range []string{"probation", "giveup", "rejected", "noresource", "invariant", "deactivated", "timeout"} {
			for _, stage := range []string{"before_build", "before_record", "before_send"} {
				t.Run(fmt.Sprintf("mwi=%t/%s/%s", mwi, reason, stage), func(t *testing.T) {
					testSubscriptionTerminationDuringAttempt(t, subscriptionTerminationCase{mwi, reason, stage})
				})
			}
		}
	}
}

type subscriptionTerminationCase struct {
	mwi    bool
	reason string
	stage  string
}

func testSubscriptionTerminationDuringAttempt(t *testing.T, tc subscriptionTerminationCase) {
	t.Helper()
	s := newSubscriptionLifecycleTestService(t, nil)
	initial := beginProtocolSubscription(t, s, tc.mwi)
	completeProtocolSubscription(t, s, tc.mwi, initial)
	prepareAcceptedNotify(t, s, notifyForSubscription(initial.request, "active;expires=120", 1)).afterReply()
	attempt, err := s.beginSubscriptionAttempt(tc.mwi, false)
	if err != nil {
		t.Fatal(err)
	}
	terminate := func() {
		prepareAcceptedNotify(t, s, notifyForSubscription(initial.request,
			"terminated;reason="+tc.reason+";retry-after=120", 2)).afterReply()
	}
	if tc.stage == "before_build" {
		terminate()
	}
	build := s.buildRegistrationSubscription
	if tc.mwi {
		build = s.buildMWISubscription
	}
	request, expires, err := build(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	refresh := subscriptionResult{context: attempt, request: request, requestedExpires: expires}
	if tc.stage == "before_send" {
		if err := s.recordSubscriptionUsageAttempt(refresh, tc.mwi); err != nil {
			t.Fatal(err)
		}
	}
	if tc.stage != "before_build" {
		terminate()
	}
	f := s.subscriptionFieldsLocked(tc.mwi)
	retryAt, lastErr, generation := f.lifecycle.retryAt, *f.lastErr, f.lifecycle.usageGeneration
	if tc.stage == "before_send" {
		refresh.err = s.subscriptionSent(refresh, tc.mwi)
	} else {
		refresh.err = s.recordSubscriptionUsageAttempt(refresh, tc.mwi)
	}
	if !errors.Is(refresh.err, errSubscriptionUsageChanged) {
		t.Fatalf("stale refresh allowed after termination: %v", refresh.err)
	}
	if err := s.recordSubscriptionUsageResult(refresh, tc.mwi); !errors.Is(err, errSubscriptionUsageChanged) {
		t.Fatalf("retired attempt error was lost: %v", err)
	}
	if !*f.closed || !f.lifecycle.retryAt.Equal(retryAt) || *f.lastErr != lastErr || f.lifecycle.usageGeneration != generation {
		t.Fatal("stale attempt overwrote the terminal state or retry deadline")
	}
}

func TestSubscriptionLateResponseCannotChangeReopenedUsage(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		for _, status := range []int{200, 405, 481, 489, 503} {
			t.Run(fmt.Sprintf("mwi=%t/status=%d", mwi, status), func(t *testing.T) {
				s := newSubscriptionLifecycleTestService(t, nil)
				initial := beginProtocolSubscription(t, s, mwi)
				completeProtocolSubscription(t, s, mwi, initial)
				old := beginProtocolSubscription(t, s, mwi)
				prepareAcceptedNotify(t, s, notifyForSubscription(old.request, "terminated;reason=deactivated", 1)).afterReply()
				if start, reason := s.prepareSubscriptionStart(mwi); !start {
					t.Fatal(reason)
				}
				next := beginProtocolSubscription(t, s, mwi)
				f := s.subscriptionFieldsLocked(mwi)
				lifecycle, dialog, lastErr := *f.lifecycle, *f.dialog, *f.lastErr
				old.response = sip.NewResponse(status, "late response")
				if _, retry := s.retrySubscriptionAfter481(old, mwi); retry {
					t.Fatal("stale 481 replaced the new usage")
				}
				if status != 200 {
					old.err = fmt.Errorf("SUBSCRIBE rejected with status %d", status)
				}
				if err := s.recordSubscriptionUsageResult(old, mwi); !errors.Is(err, errSubscriptionUsageChanged) {
					t.Fatalf("stale response accepted into reopened usage: %v", err)
				}
				if !reflect.DeepEqual(*f.lifecycle, lifecycle) || !reflect.DeepEqual(*f.dialog, dialog) || *f.lastErr != lastErr || *f.closed {
					t.Fatal("stale response changed the new usage")
				}
				if s.subscriptionRegistrations.rejected(s.subscriptionBinding, subscriptionEventPackage(mwi)) != 0 {
					t.Fatal("retired response marked the current identity unsupported")
				}
				completeProtocolSubscription(t, s, mwi, next)
			})
		}
	}
}

func TestSubscriptionProbationAllowsOnlyFreshAttemptAfterRetry(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		old := beginProtocolSubscription(t, s, mwi)
		completeProtocolSubscription(t, s, mwi, old)
		prepareAcceptedNotify(t, s, notifyForSubscription(old.request, "terminated;reason=probation;retry-after=120", 1)).afterReply()
		old.response = sip.NewResponse(481, "late response")
		if _, retry := s.retrySubscriptionAfter481(old, mwi); retry {
			t.Fatal("stale 481 bypassed probation")
		}
		if _, err := s.beginSubscriptionAttempt(mwi, false); err == nil {
			t.Fatal("new attempt bypassed Retry-After")
		}
		f := s.subscriptionFieldsLocked(mwi)
		f.lifecycle.retryAt = time.Now().Add(-time.Second)
		if err := s.recordSubscriptionUsageAttempt(old, mwi); !errors.Is(err, errSubscriptionUsageChanged) {
			t.Fatalf("expired Retry-After allowed the retired dialog: %v", err)
		}
		next := beginProtocolSubscription(t, s, mwi)
		if next.request.CallID().Value() == old.request.CallID().Value() || next.context.usageGeneration == old.context.usageGeneration {
			t.Fatal("re-subscription reused the old dialog or generation")
		}
		completeProtocolSubscription(t, s, mwi, next)
	}
}

func TestSubscriptionQueuedTerminationDoesNotDisturbRegistration(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		old := beginProtocolSubscription(t, s, mwi)
		completeProtocolSubscription(t, s, mwi, old)
		queued := buildProtocolSubscription(t, s, mwi)
		if err := s.recordSubscriptionUsageAttempt(queued, mwi); err != nil {
			t.Fatal(err)
		}
		prepareAcceptedNotify(t, s, notifyForSubscription(old.request, "terminated;reason=probation;retry-after=120", 1)).afterReply()
		connection := s.registrationTCP
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, _, queued.err = s.dispatchOutboundRequestWithCallbacks(outboundDispatchOptions{
			Context: ctx, Flow: registrationSubscriptionFlow, Request: queued.request,
			Callbacks: sipTransactionCallbacks{onBeforeSend: func() error { return s.subscriptionSent(queued, mwi) }},
		}, true)
		cancel()
		if !errors.Is(queued.err, errSubscriptionUsageChanged) {
			t.Fatalf("dispatcher did not reject the retired request before writing: %v", queued.err)
		}
		err := s.recordSubscriptionUsageResult(queued, mwi)
		if mwi {
			s.reportMWISubscriptionRuntimeError(err)
		} else {
			s.reportSubscriptionRuntimeError(err)
		}
		if s.RegState() != regRegistered || s.registrationTCP != connection || s.pcscfRecoveryPending.Load() {
			t.Fatal("retired request disturbed the IMS registration")
		}
		select {
		case err := <-s.RegistrationErrors():
			t.Fatalf("retired request requested a runtime rebuild: %v", err)
		default:
		}
	}
}

func TestSubscriptionTerminationDoesNotInvalidateOtherEvent(t *testing.T) {
	for _, terminatedMWI := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		terminated := beginProtocolSubscription(t, s, terminatedMWI)
		completeProtocolSubscription(t, s, terminatedMWI, terminated)
		other := buildProtocolSubscription(t, s, !terminatedMWI)
		prepareAcceptedNotify(t, s, notifyForSubscription(terminated.request, "terminated;reason=rejected", 1)).afterReply()
		if err := s.recordSubscriptionUsageAttempt(other, !terminatedMWI); err != nil {
			t.Fatal(err)
		}
		if err := s.subscriptionSent(other, !terminatedMWI); err != nil {
			t.Fatal(err)
		}
		completeProtocolSubscription(t, s, !terminatedMWI, other)
	}
}

func TestMWISharedRejectionBlocksAlreadyBuiltAttempt(t *testing.T) {
	for _, status := range []int{405, 489} {
		for _, queued := range []bool{false, true} {
			s := newSubscriptionLifecycleTestService(t, nil)
			attempt := buildProtocolSubscription(t, s, true)
			if queued {
				if err := s.recordSubscriptionUsageAttempt(attempt, true); err != nil {
					t.Fatal(err)
				}
			}
			s.subscriptionRegistrations.reject(s.subscriptionBinding, mwiEventPackage, status)
			if queued {
				attempt.err = s.subscriptionSent(attempt, true)
			} else {
				attempt.err = s.recordSubscriptionUsageAttempt(attempt, true)
			}
			if !errors.Is(attempt.err, errSubscriptionUsageChanged) {
				t.Fatalf("cached MWI %d ignored: %v", status, attempt.err)
			}
			if err := s.recordMWISubscriptionResult(attempt); !errors.Is(err, errSubscriptionUsageChanged) {
				t.Fatal(err)
			}
			if !s.mwiSubscriptionLifecycle.retryAt.IsZero() || !s.mwiSubscriptionLifecycle.notifyDeadline.IsZero() {
				t.Fatal("unsupported MWI attempt scheduled more work")
			}
		}
	}
}

func TestSubscription481RetryRetiresUsageAndRecordsBuildFailure(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		s := newSubscriptionLifecycleTestService(t, nil)
		initial := beginProtocolSubscription(t, s, mwi)
		completeProtocolSubscription(t, s, mwi, initial)
		old := beginProtocolSubscription(t, s, mwi)
		old.response = sip.NewResponse(481, "Call/Transaction Does Not Exist")
		retry, ok := s.retrySubscriptionAfter481(old, mwi)
		if !ok || retry.context.usageGeneration == old.context.usageGeneration {
			t.Fatal("481 retry did not start a fresh usage")
		}
		if retry.response != nil || retry.request != nil || retry.err != nil {
			t.Fatal("481 retry retained the previous exchange result")
		}
		if _, again := s.retrySubscriptionAfter481(old, mwi); again {
			t.Fatal("same 481 response retired another usage")
		}
		retry.err = errors.New("registered subscription profile unavailable")
		if err := s.recordSubscriptionUsageResult(retry, mwi); !errors.Is(err, retry.err) {
			t.Fatalf("replacement build error lost: %v", err)
		}
		f := s.subscriptionFieldsLocked(mwi)
		if *f.lastErr != retry.err.Error() || f.lifecycle.retryAt.IsZero() || !f.lifecycle.notifyDeadline.IsZero() {
			t.Fatal("replacement build failure was not diagnosed and scheduled")
		}
	}
}
