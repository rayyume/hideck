package imscore

import (
	"io"
	"net"
	"testing"
	"time"
)

func Test2degreesSMSDeliveryAfterMWI405AndOnDemandReconnect(t *testing.T) {
	s, subscriber, outbound := newInboundSMSTestService(t)
	s.cfg.CarrierPresetID = "2degrees_nz_53024"
	s.mu.Lock()
	s.trackSubscriptionRegistrationLocked(time.Hour)
	s.mu.Unlock()
	rejectSubscriptionForTest(t, s, true)
	push, peer := net.Pipe()
	t.Cleanup(func() { _ = push.Close(); _ = peer.Close() })
	s.trackProtectedConnection(push)
	s.recordPortSOpened(push, time.Now())
	s.portSOnDemandObserved.Store(true)
	s.recordPortSClosed(push, io.EOF, time.Now())
	s.untrackProtectedConnection(push)
	s.trackProtectedConnection(push)
	body := inboundRPData(t, 0x25, "+447700900123", "subscription compatibility")
	dispatchInboundRaw(t, s, inboundSMSRequest(t, imsSMSContentType, body))
	select {
	case <-subscriber.events:
	case <-time.After(time.Second):
		t.Fatal("MWI rejection prevented incoming SMS delivery")
	}
	_ = waitForOutboundSMSControl(t, outbound)
	if storeMWIRejection(s) != 405 {
		t.Fatal("receiving SMS cleared the MWI rejection")
	}
	select {
	case err := <-s.RegistrationErrors():
		t.Fatalf("ordinary EOF or MWI rejection rebuilt IMS: %v", err)
	default:
	}
}

func TestSubscriptionClosedWithoutRejectionCanStartAgain(t *testing.T) {
	s := newSubscriptionLifecycleTestService(t, nil)
	for _, mwi := range []bool{false, true} {
		if start, reason := s.prepareSubscriptionStart(mwi); !start {
			t.Fatal(reason)
		}
		if mwi {
			s.closeMWISubscription()
		} else {
			s.closeRegistrationSubscription()
		}
		if start, reason := s.prepareSubscriptionStart(mwi); !start {
			t.Fatalf("ordinary subscription termination became permanent: %s", reason)
		}
	}
}
