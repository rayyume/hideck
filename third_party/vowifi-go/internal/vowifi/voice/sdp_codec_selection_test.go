package voice

import (
	"strings"
	"testing"
)

func TestBuildPreconditionStatusSDPUsesSelectedCodecsAndQoS(t *testing.T) {
	local := string(buildBasicSDP("192.0.2.10", 12000, 10))
	remote := "v=0\r\no=- 20 20 IN IP4 192.0.2.20\r\ns=ims\r\nc=IN IP4 192.0.2.20\r\n" +
		"t=0 0\r\nm=audio 22000 RTP/AVP 104 96\r\n" +
		"a=rtpmap:104 AMR-WB/16000\r\na=fmtp:104 mode-set=0,2;max-red=0\r\n" +
		"a=rtpmap:96 telephone-event/16000\r\na=fmtp:96 0-15\r\n" +
		"a=curr:qos local sendrecv\r\na=curr:qos remote sendrecv\r\n" +
		"a=des:qos optional local sendrecv\r\na=des:qos mandatory remote sendrecv\r\n"
	got, err := buildPreconditionStatusSDP(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"m=audio 12000 RTP/AVP 104 96", "a=fmtp:104 mode-set=0,2;max-red=0",
		"a=curr:qos local sendrecv", "a=curr:qos remote sendrecv",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("status UPDATE SDP missing %q: %q", want, got)
		}
	}
	for _, removed := range []string{"rtpmap:110", "rtpmap:102", "rtpmap:106", "rtpmap:101"} {
		if strings.Contains(got, removed) {
			t.Fatalf("status UPDATE SDP kept unselected payload %q: %q", removed, got)
		}
	}
}

func TestBuildPreconditionStatusSDPRejectsNoCommonCodec(t *testing.T) {
	local := "v=0\r\nm=audio 12000 RTP/AVP 104\r\na=rtpmap:104 AMR-WB/16000\r\n"
	remote := "v=0\r\nm=audio 22000 RTP/AVP 8\r\n"
	if _, err := buildPreconditionStatusSDP(local, remote); err == nil {
		t.Fatal("status UPDATE accepted an answer without an offered codec")
	}
}

func TestBuildPreconditionStatusSDPPreservesVideoPayloadAttributes(t *testing.T) {
	local := "v=0\r\nm=audio 12000 RTP/AVP 104 110\r\na=rtpmap:104 AMR-WB/16000\r\n" +
		"a=rtpmap:110 AMR-WB/16000\r\nm=video 12002 RTP/AVP 110\r\na=rtpmap:110 H264/90000\r\n"
	remote := "v=0\r\nm=audio 22000 RTP/AVP 104\r\na=rtpmap:104 AMR-WB/16000\r\n" +
		"m=video 22002 RTP/AVP 110\r\na=rtpmap:110 H264/90000\r\n"
	got, err := buildPreconditionStatusSDP(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "m=video 12002 RTP/AVP 110") ||
		!strings.Contains(got, "a=rtpmap:110 H264/90000") {
		t.Fatalf("audio codec selection changed the video section: %q", got)
	}
}
