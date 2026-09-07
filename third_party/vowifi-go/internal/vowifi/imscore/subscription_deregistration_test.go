package imscore

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func TestMWIRejectionClearedOnlyBySuccessfulDeregistration(t *testing.T) {
	for _, operation := range []string{"contact", "all", "cleanup"} {
		for _, status := range []int{200, 503} {
			t.Run(operation+"/"+strconv.Itoa(status), func(t *testing.T) {
				s := newSubscriptionLifecycleTestService(t, nil)
				s.mu.Lock()
				s.regSession.callID = "subscription-deregistration"
				s.regSession.authHeader = `Digest username="user"`
				s.mu.Unlock()
				rejectSubscriptionForTest(t, s, true)
				s.transport.SetSendFn(func(request string) error {
					s.transport.DeliverResponse(registerResponseForRequest(request, status, nil))
					return nil
				})
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				remove := s.Unregister
				if operation == "all" {
					remove = s.UnregisterAll
				} else if operation == "cleanup" {
					remove = s.ClearRegistrationBindings
				}
				err := remove(ctx)
				if (err == nil) != (status == 200) {
					t.Fatalf("status=%d err=%v", status, err)
				}
				want := 405
				if status == 200 {
					want = 0
				}
				if got := storeMWIRejection(s); got != want {
					t.Fatalf("status=%d MWI rejection=%d want=%d", status, got, want)
				}
			})
		}
	}
}
