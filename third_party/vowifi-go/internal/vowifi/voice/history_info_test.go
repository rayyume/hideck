package voice

import (
	"testing"
)

func TestParseHistoryInfoSingleAndMultiLevel(t *testing.T) {
	entries, err := parseHistoryInfo(
		`<sip:+447700900111@ims.example>;index=1, <sip:+447700900222@ims.example;cause=302>;index=1.1`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d, want 2", len(entries))
	}
	if entries[0].Index != "1" || entries[0].URI != "sip:+447700900111@ims.example" || entries[0].Cause != 0 {
		t.Fatalf("index=1 entry = %+v", entries[0])
	}
	if entries[1].Index != "1.1" || entries[1].URI != "sip:+447700900222@ims.example" || entries[1].Cause != 302 {
		t.Fatalf("forwarded entry = %+v", entries[1])
	}
}

func TestParseHistoryInfoCauseAfterURI(t *testing.T) {
	entries, err := parseHistoryInfo(`<sip:orig@ims.example>;index=1;cause=486`)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Cause != 486 || entries[0].URI != "sip:orig@ims.example" {
		t.Fatalf("entry=%+v", entries)
	}
}

func TestParseHistoryInfoMalformedIsError(t *testing.T) {
	if _, err := parseHistoryInfo("sip:missing-brackets@ims.example;index=1"); err == nil {
		t.Fatal("malformed History-Info succeeded")
	}
}

func TestInboundHistoryInfoExposesOriginalCalled(t *testing.T) {
	agent := startedVoiceAgent(t)
	peer := listenVoiceUDP(t)
	invite := inboundAgentInvite("hist-1", peer, &capturedVoiceResponder{localTag: "h1"})
	invite.HistoryInfo = `<sip:+447700900000@ims.example>;index=1, <sip:user@ims.example;cause=302>;index=1.1`
	if _, err := agent.HandleInboundVoiceRequest(invite); err != nil {
		t.Fatal(err)
	}
	call := agent.callByID(invite.CallID)
	if call == nil {
		t.Fatal("inbound call missing")
	}
	if got := call.OriginalCalledURI(); got != "sip:+447700900000@ims.example" {
		t.Fatalf("OriginalCalledURI=%q", got)
	}
	entries := call.HistoryInfo()
	if len(entries) != 2 || entries[1].Cause != 302 {
		t.Fatalf("HistoryInfo=%+v", entries)
	}
	snap := call.incomingSnapshot()
	if snap.OriginalCalledURI != "sip:+447700900000@ims.example" {
		t.Fatalf("snapshot original=%q", snap.OriginalCalledURI)
	}
}

func TestInboundMalformedHistoryInfoDoesNotFailInvite(t *testing.T) {
	agent := startedVoiceAgent(t)
	peer := listenVoiceUDP(t)
	responder := &capturedVoiceResponder{localTag: "h2"}
	invite := inboundAgentInvite("hist-bad", peer, responder)
	invite.HistoryInfo = "this is not a History-Info header"
	result, err := agent.HandleInboundVoiceRequest(invite)
	if err != nil || result.StatusCode != 0 {
		t.Fatalf("invite result=%+v err=%v", result, err)
	}
	if got := responder.lastResponse().StatusCode; got != 180 {
		t.Fatalf("status=%d, want 180", got)
	}
	call := agent.callByID(invite.CallID)
	if call == nil || len(call.HistoryInfo()) != 0 {
		t.Fatalf("malformed header was stored: %+v", call.HistoryInfo())
	}
}
