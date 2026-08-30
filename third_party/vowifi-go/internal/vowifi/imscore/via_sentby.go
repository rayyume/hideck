package imscore

import "strings"

// sipTransportIsUDP reports an explicit SIP/UDP hop. Empty and TCP/TLS
// stay on the connection-oriented family so a missing token cannot flip a
// live TCP Via onto the UDP port-s rule.
func sipTransportIsUDP(transport string) bool {
	return strings.EqualFold(strings.TrimSpace(transport), "udp")
}

// protectedViaSentByPort picks the Via sent-by port for an IMS AKA
// security association. RFC 3329 only negotiates port-c / port-s; it
// does not set Via.
//
// UDP IPsec — TS 24.229 5.1.1.2.2 (c) and 5.1.1.4.2 (c): a protected
// REGISTER Via sent-by includes the protected server port. 5.1.2A.1.1
// (IMS AKA) (b) says the same for non-REGISTER. Annex K.2.1.2.2.2 /
// K.2.1.4.1 repeat port-s for UDP. TS 33.203 7.1 UDP: the UE still
// transmits from port-c and receives on port-s; 24.229 5.2.2.2 NOTE 3
// says the P-CSCF replies to a different UE port than the request
// source. Annex M SM7 (UDP-enc-tun) also puts port-s in Via.
//
// TCP / TLS — 5.1.1.2.2 (c) is written "for UDP" only. K.2.1.2.2.2 and
// K.2.1.4.1 say a protected TCP Via carries the public IP or FQDN, not
// port-s. 5.2.2.2 NOTE 3, TS 33.203 7.1 TCP, and RFC 3261 §18 send the
// response on the same connection (UE port-c → P-CSCF port-s). There
// is no TCP/TLS clause that requires rewriting a working sent-by to
// port-s; fallback stays the actual TCP source or configured local
// port.
func protectedViaSentByPort(transport string, portS, fallback int) int {
	if sipTransportIsUDP(transport) && portS > 0 {
		return portS
	}
	return fallback
}
