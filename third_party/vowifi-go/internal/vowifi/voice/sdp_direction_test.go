package voice

import "testing"

func TestSDPMediaDirection(t *testing.T) {
	tests := []struct {
		name string
		sdp  string
		want string
	}{
		{name: "default", sdp: "v=0\r\nm=audio 9 RTP/AVP 0\r\n", want: sdpDirectionSendRecv},
		{name: "media sendonly", sdp: "v=0\r\nm=audio 9 RTP/AVP 0\r\na=sendonly\r\n", want: sdpDirectionSendOnly},
		{name: "session recvonly", sdp: "v=0\r\na=recvonly\r\nm=audio 9 RTP/AVP 0\r\n", want: sdpDirectionRecvOnly},
		{name: "media overrides session", sdp: "v=0\r\na=sendonly\r\nm=audio 9 RTP/AVP 0\r\na=sendrecv\r\n", want: sdpDirectionSendRecv},
	}
	for _, test := range tests {
		if got := sdpMediaDirection(test.sdp); got != test.want {
			t.Fatalf("%s: %s, want %s", test.name, got, test.want)
		}
	}
}

func TestRewriteSDPDirectionReplacesAudioAttribute(t *testing.T) {
	raw := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\nm=audio 9 RTP/AVP 0\r\na=sendrecv\r\na=ptime:20\r\n"
	got := rewriteSDPDirection(raw, sdpDirectionSendOnly)
	if sdpMediaDirection(got) != sdpDirectionSendOnly {
		t.Fatalf("rewritten direction = %s", sdpMediaDirection(got))
	}
	if !containsLine(got, "a=sendonly") || containsLine(got, "a=sendrecv") {
		t.Fatalf("rewritten SDP = %q", got)
	}
}

func TestBumpSDPOriginVersion(t *testing.T) {
	raw := "v=0\r\no=- 8 8 IN IP4 127.0.0.1\r\ns=call\r\n"
	got := bumpSDPOriginVersion(raw)
	if !containsLine(got, "o=- 8 9 IN IP4 127.0.0.1") {
		t.Fatalf("origin = %q", got)
	}
}

func TestNegotiateAnswerDirection(t *testing.T) {
	tests := []struct {
		offer string
		hold  bool
		want  string
	}{
		{offer: sdpDirectionSendRecv, want: sdpDirectionSendRecv},
		{offer: sdpDirectionSendOnly, want: sdpDirectionRecvOnly},
		{offer: sdpDirectionRecvOnly, want: sdpDirectionSendOnly},
		{offer: sdpDirectionInactive, want: sdpDirectionInactive},
		{offer: sdpDirectionSendRecv, hold: true, want: sdpDirectionSendOnly},
		{offer: sdpDirectionSendOnly, hold: true, want: sdpDirectionInactive},
	}
	for _, test := range tests {
		if got := negotiateAnswerDirection(test.offer, test.hold); got != test.want {
			t.Fatalf("offer %s hold %t = %s, want %s", test.offer, test.hold, got, test.want)
		}
	}
}

func containsLine(raw, line string) bool {
	for _, source := range splitSDPTextLines(raw) {
		if source == line {
			return true
		}
	}
	return false
}
