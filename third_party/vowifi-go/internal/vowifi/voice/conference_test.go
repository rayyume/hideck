package voice

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestMergeConferenceWithoutFactoryURI(t *testing.T) {
	agent := startedVoiceAgent(t)
	_, err := agent.MergeConference(context.Background())
	if !errors.Is(err, ErrConferenceFactoryUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestParseConferenceInfoUsers(t *testing.T) {
	body := []byte(`<?xml version="1.0"?>
<conference-info xmlns="urn:ietf:params:xml:ns:conference-info" entity="sip:conf-xyz@ims.example.com" state="full">
  <users>
    <user entity="sip:alice@ims.example.com" state="full">
      <endpoint entity="sip:alice@ims.example.com"><status>connected</status></endpoint>
    </user>
    <user entity="sip:bob@ims.example.com" state="full">
      <endpoint entity="sip:bob@ims.example.com"><status>connected</status></endpoint>
    </user>
  </users>
</conference-info>`)
	info, err := parseConferenceInfo(body)
	if err != nil {
		t.Fatal(err)
	}
	if info.Entity != "sip:conf-xyz@ims.example.com" || len(info.Users) != 2 {
		t.Fatalf("info=%+v", info)
	}
	if info.Users[0].Status != "connected" || info.Users[1].Entity != "sip:bob@ims.example.com" {
		t.Fatalf("users=%+v", info.Users)
	}
}

func TestMergeConferenceInvitesFactoryAndRefersBoth(t *testing.T) {
	var mu sync.Mutex
	var captured []string
	registrar := startScriptedVoiceRegistrar(t, func(request string) (int, string, string) {
		mu.Lock()
		captured = append(captured, request)
		mu.Unlock()
		if strings.HasPrefix(request, "INVITE ") {
			extra := "To: <sip:peer@ims.example.com>;tag=remote\r\nContent-Type: application/sdp\r\n"
			if strings.Contains(request, "conf-factory") {
				extra = "To: <sip:conf-factory@ims.example.com>;tag=focus\r\n" +
					"Contact: <sip:conf-xyz@ims.example.com>;isfocus\r\n" +
					"Content-Type: application/sdp\r\n"
			} else {
				extra += "Contact: <sip:peer@ims.example.com>\r\n"
			}
			return 200, extra, testIMSAnswerSDP
		}
		if strings.HasPrefix(request, "REFER ") {
			return 202, "To: <sip:peer@ims.example.com>;tag=remote\r\n", ""
		}
		if strings.HasPrefix(request, "SUBSCRIBE ") {
			return 200, "To: <sip:conf-xyz@ims.example.com>;tag=focus\r\n", ""
		}
		return 200, "", ""
	})
	agent := newVoiceTestAgent(t, registrar)
	if err := agent.StartCurrent(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = agent.Stop() })
	agent.SetConferenceFactoryURI("sip:conf-factory@ims.example.com")
	first, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	secondSDP := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=client\r\nc=IN IP4 127.0.0.1\r\nt=0 0\r\nm=audio 32010 RTP/AVP 0\r\n"
	second, err := agent.HandleClientInvite("+8613800000001", secondSDP)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conf, err := agent.MergeConference(ctx)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if conf == nil || !conf.conference || conf.CallState() != callstate.StateConnected {
		t.Fatalf("conference call state=%v conference=%t", conf.CallState(), conf != nil && conf.conference)
	}
	mu.Lock()
	dump := append([]string(nil), captured...)
	mu.Unlock()
	var sawFactory, sawSubscribe bool
	refers := 0
	for _, request := range dump {
		switch {
		case strings.HasPrefix(request, "INVITE ") && strings.Contains(request, "conf-factory"):
			sawFactory = true
		case strings.HasPrefix(request, "REFER "):
			refers++
			if !strings.Contains(request, "Refer-To: <sip:conf-xyz@ims.example.com>") &&
				!strings.Contains(request, "Refer-To: <sip:conf-factory@ims.example.com>") {
				t.Fatalf("REFER missing conference URI: %s", request)
			}
		case strings.HasPrefix(request, "SUBSCRIBE ") && strings.Contains(request, "Event: conference"):
			sawSubscribe = true
		}
	}
	if !sawFactory {
		t.Fatal("factory INVITE was not sent")
	}
	if refers < 2 {
		t.Fatalf("REFER count=%d, want at least 2", refers)
	}
	if !sawSubscribe {
		t.Fatal("conference SUBSCRIBE was not sent")
	}
	if first.CallState() != callstate.StateConnected || second.CallState() != callstate.StateConnected {
		t.Fatalf("participants dropped first=%s second=%s", first.CallState(), second.CallState())
	}
}

func TestInboundConferenceNotifyStoresInfo(t *testing.T) {
	agent := startedVoiceAgent(t)
	call, err := agent.HandleClientInvite("+8613800000000", testClientSDP)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`<conference-info entity="sip:conf@ims.example.com"><users><user entity="sip:alice@ims.example.com"><endpoint><status>connected</status></endpoint></user></users></conference-info>`)
	result, err := agent.HandleInboundVoiceRequest(imscore.InboundVoiceRequest{
		Method: "NOTIFY", CallID: call.CallID(), Event: "conference", Body: body,
		Responder: &capturedVoiceResponder{localTag: "n"},
	})
	if err != nil || result.StatusCode != 0 {
		t.Fatalf("NOTIFY result=%+v err=%v", result, err)
	}
	info := call.ConferenceInfoSnapshot()
	if info.Entity != "sip:conf@ims.example.com" || len(info.Users) != 1 {
		t.Fatalf("stored info=%+v", info)
	}
}
