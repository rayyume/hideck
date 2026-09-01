package volte

import (
	"errors"
	"testing"
)

func TestATTimeoutForMBNList(t *testing.T) {
	if got := atTimeout(MBNListQueryCommand()); got != mbnListTimeout {
		t.Fatalf("list timeout %s", got)
	}
	if got := atTimeout(MBNActivateCommand()); got != mbnListTimeout {
		t.Fatalf("activate timeout %s", got)
	}
	if got := atTimeout(IMSQueryCommand()); got != defaultATTimeout {
		t.Fatalf("ims timeout %s", got)
	}
}

func TestIsATTimeout(t *testing.T) {
	if !isATTimeout(errors.New("timeout")) || !isATTimeout(errors.New("query mbn list: timeout")) {
		t.Fatal("want timeout detection")
	}
	if isATTimeout(errors.New("AT ERROR")) || isATTimeout(nil) {
		t.Fatal("non-timeout must not retry")
	}
}
