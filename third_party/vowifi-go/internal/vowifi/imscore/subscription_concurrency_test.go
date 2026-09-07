package imscore

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestSubscriptionRetiredAttemptCannotChangeNewTimers(t *testing.T) {
	s := newSubscriptionLifecycleTestService(t, nil)
	old := subscriptionResult{context: s.subscriptionContextLocked(), requestedExpires: time.Hour}
	s.mu.Lock()
	s.subscriptionGeneration++
	s.mu.Unlock()
	for _, record := range []func(subscriptionResult) error{s.recordSubscriptionAttempt, s.recordMWISubscriptionAttempt} {
		if err := record(old); !errors.Is(err, errSubscriptionContextChanged) {
			t.Fatalf("retired attempt accepted: %v", err)
		}
	}
	if !s.subscriptionRefreshAt.IsZero() || !s.mwiSubscriptionRefreshAt.IsZero() {
		t.Fatal("retired attempt changed new subscription timers")
	}
}

func TestSubscriptionInFlightDuringNewContactIsRestarted(t *testing.T) {
	for _, mwi := range []bool{false, true} {
		t.Run(map[bool]string{false: "reg", true: "mwi"}[mwi], func(t *testing.T) {
			s := newProtectedKeepaliveTestService(t)
			client, server := net.Pipe()
			s.activateProtectedRegistrationTCP(client)
			t.Cleanup(func() { _ = server.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = server.SetDeadline(time.Now().Add(2 * time.Second))
			seen, release := make(chan string, 2), make(chan struct{})
			wireDone := make(chan error, 1)
			go func() { wireDone <- serveRetiredSubscription(ctx, server, seen, release) }()
			send, report := s.sendSubscribeReg, s.reportSubscriptionRuntimeError
			if mwi {
				send, report = s.sendSubscribeMWI, s.reportMWISubscriptionRuntimeError
			}
			done := make(chan error, 1)
			go func() { done <- send(ctx) }()
			select {
			case <-seen:
			case <-ctx.Done():
				t.Fatal("initial subscription did not reach the wire")
			}
			s.mu.Lock()
			s.subscriptionGeneration++
			s.regSession.contactUser = "replacement-contact"
			s.trackSubscriptionRegistrationLocked(time.Hour)
			s.mu.Unlock()
			if start, reason := s.prepareSubscriptionStart(mwi); !start {
				t.Fatal(reason)
			}
			if err := send(ctx); err != nil {
				t.Fatal(err)
			}
			close(release)
			err := <-done
			if !errors.Is(err, errSubscriptionContextChanged) {
				t.Fatalf("old response accepted: %v", err)
			}
			report(err)
			select {
			case request := <-seen:
				if sipAddressTag(rawSIPHeaderValue(request, "To")) != "" {
					t.Fatal("new subscription reused retired dialog")
				}
			case <-ctx.Done():
				t.Fatal("replacement subscription was lost behind old in-flight request")
			}
			if err := <-wireDone; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func serveRetiredSubscription(ctx context.Context, conn net.Conn, seen chan<- string, release <-chan struct{}) error {
	reader := bufio.NewReader(conn)
	for index := range 2 {
		request, err := readSIPStreamMessage(reader)
		if err != nil {
			return err
		}
		seen <- request
		if index == 0 {
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if _, err := io.WriteString(conn, subscriptionWireResponse(request, 200, "Expires: 120\r\n")); err != nil {
			return err
		}
	}
	return nil
}
