package runtimecore

import "github.com/iniwex5/vowifi-go/internal/vowifi/imscore"

func (network *installerIMSNetwork) IMSNetworkDiagnostics() map[string]any {
	if provider, ok := network.IMSNetwork.(imscore.IMSNetworkDiagnosticsProvider); ok {
		return provider.IMSNetworkDiagnostics()
	}
	return map[string]any{"available": false}
}
