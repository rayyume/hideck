package volte

import (
	"strings"
	"testing"
)

func TestPCMUOfferSDPIsAttachable(t *testing.T) {
	sdp := pcmuOfferSDP(40000)
	if !strings.Contains(sdp, "c=IN IP4 127.0.0.1") || !strings.Contains(sdp, "PCMU/8000") {
		t.Fatalf("sdp %s", sdp)
	}
	port, err := rtpPortFromSDP(sdp)
	if err != nil || port != 40000 {
		t.Fatalf("port %d err=%v", port, err)
	}
}

func TestStartCallMediaAdvertisesLocalPCMU(t *testing.T) {
	browser := "v=0\r\nm=audio 41000 RTP/AVP 0\r\nc=IN IP4 127.0.0.1\r\n"
	media, err := startCallMedia(browser, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer media.Close()
	port, err := rtpPortFromSDP(media.sdp)
	if err != nil || port <= 0 {
		t.Fatalf("advertised port %d err=%v", port, err)
	}
}
