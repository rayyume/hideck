package qmi

import (
	"fmt"
	"testing"
)

func TestVoiceCallAlreadyGone(t *testing.T) {
	gone := &QMIError{Service: ServiceVOICE, MessageID: VOICEEndCall, Result: 1, ErrorCode: QMIErrInvalidID}
	if !VoiceCallAlreadyGone(fmt.Errorf("end call failed: %w", gone)) {
		t.Fatal("wrapped invalid-id should mean call already gone")
	}
	if !VoiceCallAlreadyGone(&QMIError{ErrorCode: QMIErrNoEffect}) {
		t.Fatal("no-effect should mean call already gone")
	}
	if VoiceCallAlreadyGone(&QMIError{ErrorCode: QMIErrInternal}) {
		t.Fatal("internal error is not already-gone")
	}
	if VoiceCallAlreadyGone(nil) {
		t.Fatal("nil is not already-gone")
	}
}
