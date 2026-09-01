package device

import (
	"errors"
	"testing"
	"time"

	"github.com/yibaiba/hideck/internal/volte"
)

func TestIsTransientVoLTEStartError(t *testing.T) {
	if isTransientVoLTEStartError(nil) {
		t.Fatal("nil")
	}
	if isTransientVoLTEStartError(volte.ErrNoUniqueProfile) || isTransientVoLTEStartError(ErrLebaraUKRFLocked) {
		t.Fatal("permanent errors must not retry")
	}
	if !isTransientVoLTEStartError(errors.New("volte provision apply_ims: read before apply: query mbn list: timeout")) {
		t.Fatal("AT timeout must retry")
	}
	if nativeVoLTERetryDelay(1) != 3*time.Second || nativeVoLTERetryDelay(2) != 6*time.Second {
		t.Fatalf("retry delay %s %s", nativeVoLTERetryDelay(1), nativeVoLTERetryDelay(2))
	}
}
