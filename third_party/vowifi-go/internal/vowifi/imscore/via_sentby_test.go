package imscore

import "testing"

func TestProtectedViaSentByPortByTransportFamily(t *testing.T) {
	tests := []struct {
		name      string
		transport string
		portS     int
		fallback  int
		want      int
	}{
		{name: "udp uses port-s", transport: "udp", portS: 48554, fallback: 5060, want: 48554},
		{name: "UDP token is case-insensitive", transport: "UDP", portS: 48554, fallback: 5060, want: 48554},
		{name: "udp without port-s keeps fallback", transport: "udp", portS: 0, fallback: 5060, want: 5060},
		{name: "tcp keeps fallback not port-s", transport: "tcp", portS: 48554, fallback: 5060, want: 5060},
		{name: "tls keeps fallback not port-s", transport: "tls", portS: 48554, fallback: 50309, want: 50309},
		{name: "empty transport does not take UDP rule", transport: "", portS: 48554, fallback: 5060, want: 5060},
	}
	for _, test := range tests {
		if got := protectedViaSentByPort(test.transport, test.portS, test.fallback); got != test.want {
			t.Fatalf("%s: sent-by = %d, want %d", test.name, got, test.want)
		}
	}
}

func TestSipTransportIsUDPRequiresExplicitUDP(t *testing.T) {
	if !sipTransportIsUDP("udp") || !sipTransportIsUDP("UDP") {
		t.Fatal("udp was not recognized")
	}
	for _, transport := range []string{"", "tcp", "tls", "auto"} {
		if sipTransportIsUDP(transport) {
			t.Fatalf("%q was treated as UDP", transport)
		}
	}
}
