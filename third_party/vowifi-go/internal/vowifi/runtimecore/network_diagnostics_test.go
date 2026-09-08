package runtimecore

import (
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
)

type diagnosticIMSNetwork struct{ imscore.IMSNetwork }

func (diagnosticIMSNetwork) IMSNetworkDiagnostics() map[string]any {
	return map[string]any{"available": true, "observed_syn": uint64(3)}
}

func TestInstallerNetworkForwardsDiagnostics(t *testing.T) {
	n := newInstallerIMSNetwork(diagnosticIMSNetwork{}, nil)
	d := n.IMSNetworkDiagnostics()
	if d["available"] != true || d["observed_syn"] != uint64(3) {
		t.Fatalf("installer wrapper lost diagnostics: %+v", d)
	}
	if d := newInstallerIMSNetwork(nil, nil).IMSNetworkDiagnostics(); d["available"] != false {
		t.Fatal("unsupported network reported diagnostics available")
	}
}
