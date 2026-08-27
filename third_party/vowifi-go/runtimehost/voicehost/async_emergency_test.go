package voicehost

import (
	"context"
	"errors"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/emergency"
)

func TestBeginCallRejectsEmergencyDestination(t *testing.T) {
	gateway := &Gateway{}
	_, err := gateway.BeginCall(context.Background(), BeginCallRequest{
		DeviceID: "wwan0", Callee: "999",
	})
	if !errors.Is(err, emergency.ErrOriginatingDisabled) {
		t.Fatalf("BeginCall error = %v", err)
	}
}
