package device

import (
	"testing"

	"github.com/yibaiba/hideck/internal/config"
)

func TestApplyNetworkPreferenceSuppressesHostPathWhenDataDisabled(t *testing.T) {
	orig := suppressHostInternetPath
	t.Cleanup(func() { suppressHostInternetPath = orig })
	called := false
	suppressHostInternetPath = func(w *Worker) { called = true }

	p := NewPool(&config.Config{})
	t.Cleanup(func() { p.cancel() })
	disconnected := 0
	w := &Worker{
		ID: "wwan1",
		Config: config.DeviceConfig{
			ID:             "wwan1",
			NetworkEnabled: false,
			PhoneMode:      PhoneModeVoLTE,
		},
		netOverride: &fakeController{
			connected:      true,
			disconnectHook: func() { disconnected++ },
		},
	}
	if err := p.applyNetworkPreference(w); err != nil {
		t.Fatal(err)
	}
	if disconnected != 1 {
		t.Fatalf("disconnect=%d want 1", disconnected)
	}
	if !called {
		t.Fatal("VoLTE without 网络 must suppress the host internet path")
	}
}

func TestApplyNetworkPreferenceSuppressesHostPathEvenIfQMIDisconnected(t *testing.T) {
	orig := suppressHostInternetPath
	t.Cleanup(func() { suppressHostInternetPath = orig })
	called := false
	suppressHostInternetPath = func(w *Worker) { called = true }

	p := NewPool(&config.Config{})
	t.Cleanup(func() { p.cancel() })
	w := &Worker{
		ID: "wwan1",
		Config: config.DeviceConfig{
			NetworkEnabled: false,
			PhoneMode:      PhoneModeVoLTE,
		},
		netOverride: &fakeController{connected: false},
	}
	if err := p.applyNetworkPreference(w); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("even with QMI data already down, host usbnet must stay flushed")
	}
}
