package runtimecore

import (
	"context"
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/profile"
)

type notifierObserver struct {
	events []RuntimeEvent[*SessionResult]
	order  *[]string
}

func (o *notifierObserver) OnRuntimeEvent(
	_ context.Context,
	event RuntimeEvent[*SessionResult],
) {
	o.events = append(o.events, event)
	if o.order != nil {
		*o.order = append(*o.order, event.Kind)
	}
}

func TestIMSRegisteredNotifierWaitsForSessionService(t *testing.T) {
	observer := &notifierObserver{}
	hookCalls := 0
	notifier := &imsRegisteredNotifier{
		ctx: context.Background(), events: observer,
		hooks:  RuntimeHostHooks{OnIMSRegistered: func(context.Context) { hookCalls++ }},
		device: "wwan1", traceID: "trace-1",
	}

	notifier.OnIMSRegistered()
	if len(observer.events) != 0 || hookCalls != 0 {
		t.Fatalf("registration published before session: events=%d hooks=%d", len(observer.events), hookCalls)
	}

	service := &imscore.Service{}
	session := &SessionResult{DeviceID: "wwan1", IMSService: service}
	notifier.SetSession(session)
	if len(observer.events) != 1 || hookCalls != 1 {
		t.Fatalf("registration publication: events=%d hooks=%d", len(observer.events), hookCalls)
	}
	event := observer.events[0]
	if event.Kind != "ims_registered" || event.Handle != session || event.Service != service {
		t.Fatalf("registration event missing live session service: %+v", event)
	}
}

func TestIMSRegisteredNotifierPublishesImmediatelyWithSession(t *testing.T) {
	observer := &notifierObserver{}
	service := &imscore.Service{}
	session := &SessionResult{DeviceID: "wwan1", IMSService: service}
	notifier := &imsRegisteredNotifier{
		ctx: context.Background(), events: observer, device: "wwan1", traceID: "trace-2",
	}
	notifier.SetSession(session)
	notifier.OnIMSRegistered()

	if len(observer.events) != 1 || observer.events[0].Service != service {
		t.Fatalf("registration events = %+v", observer.events)
	}
}

func TestSessionConfigFlushesSMSAfterPendingRegistration(t *testing.T) {
	order := make([]string, 0, 2)
	observer := &notifierObserver{order: &order}
	ctx := context.Background()
	request := &RuntimeStartRequest{
		Observer: observer,
		Hooks: RuntimeHostHooks{OnSMSReady: func(context.Context) {
			order = append(order, "sms_ready")
		}},
	}
	notifier := newIMSRegisteredNotifier(ctx, request, profile.IMSIdentityResult{})
	config := sessionConfigFromRequest(ctx, request, profile.PreparedSession{}, notifier)

	config.OnIMSRegistered()
	config.OnSMSReady()
	if len(order) != 0 {
		t.Fatalf("events published before session: %v", order)
	}
	notifier.SetSession(&SessionResult{IMSService: &imscore.Service{}})
	if got, want := strings.Join(order, ","), "ims_registered,sms_ready"; got != want {
		t.Fatalf("event order = %q, want %q", got, want)
	}
}

func TestSessionConfigForwardsEverySMSReadinessChange(t *testing.T) {
	ctx := context.Background()
	readinessEvents := make([]imscore.SMSReadiness, 0, 2)
	request := &RuntimeStartRequest{Hooks: RuntimeHostHooks{
		OnSMSReadinessChanged: func(_ context.Context, readiness imscore.SMSReadiness) {
			readinessEvents = append(readinessEvents, readiness)
		},
	}}
	notifier := newIMSRegisteredNotifier(ctx, request, profile.IMSIdentityResult{})
	config := sessionConfigFromRequest(ctx, request, profile.PreparedSession{}, notifier)
	config.OnSMSReadinessChanged(imscore.SMSReadiness{Ready: true, Reason: "ready"})
	config.OnSMSReadinessChanged(imscore.SMSReadiness{Ready: false, Reason: "port-s closed"})

	if len(readinessEvents) != 2 || !readinessEvents[0].Ready || readinessEvents[1].Ready {
		t.Fatalf("SMS readiness events = %+v", readinessEvents)
	}
	if readinessEvents[1].Reason != "port-s closed" {
		t.Fatalf("SMS unavailable reason = %q", readinessEvents[1].Reason)
	}
}

func TestIMSRegisteredNotifierHoldsSMSUntilRegistrationEvent(t *testing.T) {
	order := make([]string, 0, 2)
	observer := &notifierObserver{order: &order}
	notifier := &imsRegisteredNotifier{
		ctx: context.Background(), events: observer,
		hooks: RuntimeHostHooks{OnSMSReady: func(context.Context) {
			order = append(order, "sms_ready")
		}},
	}
	notifier.SetSession(&SessionResult{IMSService: &imscore.Service{}})

	notifier.OnSMSReady()
	if len(order) != 0 {
		t.Fatalf("SMS ready published before registration: %v", order)
	}
	notifier.OnIMSRegistered()
	if got, want := strings.Join(order, ","), "ims_registered,sms_ready"; got != want {
		t.Fatalf("event order = %q, want %q", got, want)
	}
}
