package voice

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/policy"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestParseReplacesRequiresCallIDAndTags(t *testing.T) {
	if _, err := parseReplaces(""); err == nil {
		t.Fatal("empty Replaces accepted")
	}
	if _, err := parseReplaces("only-call-id"); err == nil {
		t.Fatal("missing tags accepted")
	}
	spec, err := parseReplaces(`abc;to-tag=to1;from-tag=from1;early-only`)
	if err != nil {
		t.Fatal(err)
	}
	if spec.CallID != "abc" || spec.ToTag != "to1" || spec.FromTag != "from1" || !spec.EarlyOnly {
		t.Fatalf("%+v", spec)
	}
}

func TestInboundReplacesMalformedIs400(t *testing.T) {
	agent := startedVoiceAgent(t)
	req := inboundAgentInvite("new-call", listenVoiceUDP(t), &capturedVoiceResponder{localTag: "n"})
	req.Replaces = "no-tags"
	result, err := agent.HandleInboundVoiceRequest(req)
	if err != nil || result.StatusCode != 400 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestInboundReplacesUnknownDialogIs481(t *testing.T) {
	agent := startedVoiceAgent(t)
	req := inboundAgentInvite("new-call", listenVoiceUDP(t), &capturedVoiceResponder{localTag: "n"})
	req.Replaces = "missing;to-tag=a;from-tag=b"
	result, err := agent.HandleInboundVoiceRequest(req)
	if err != nil || result.StatusCode != 481 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestInboundReplacesEarlyOnlyOnConfirmedIs486(t *testing.T) {
	agent := startedVoiceAgent(t)
	firstPeer := listenVoiceUDP(t)
	first := inboundAgentInvite("call-active", firstPeer, &capturedVoiceResponder{localTag: "local-tag"})
	if _, err := agent.HandleInboundVoiceRequest(first); err != nil {
		t.Fatal(err)
	}
	client := listenVoiceUDP(t)
	if _, err := agent.AnswerWithSDP(first.CallID, voiceSDP(client.LocalAddr().(*net.UDPAddr).Port)); err != nil {
		t.Fatal(err)
	}
	call := agent.callByID(first.CallID)
	_, fromTag, toTag := call.replacesDialogID()
	req := inboundAgentInvite("replace-call", listenVoiceUDP(t), &capturedVoiceResponder{localTag: "n2"})
	req.Replaces = first.CallID + ";to-tag=" + toTag + ";from-tag=" + fromTag + ";early-only"
	result, err := agent.HandleInboundVoiceRequest(req)
	if err != nil || result.StatusCode != 486 {
		t.Fatalf("early-only result=%+v err=%v tags=%q/%q", result, err, fromTag, toTag)
	}
}

func TestInboundReplacesTerminatesMatchedDialog(t *testing.T) {
	agent := startedVoiceAgent(t)
	firstPeer := listenVoiceUDP(t)
	first := inboundAgentInvite("call-active", firstPeer, &capturedVoiceResponder{localTag: "local-tag"})
	if _, err := agent.HandleInboundVoiceRequest(first); err != nil {
		t.Fatal(err)
	}
	call := agent.callByID(first.CallID)
	if call.CallState() != callstate.StateRinging {
		t.Fatalf("state=%s", call.CallState())
	}
	_, fromTag, toTag := call.replacesDialogID()
	if fromTag == "" || toTag == "" {
		t.Fatalf("missing tags from=%q to=%q", fromTag, toTag)
	}
	req := inboundAgentInvite("replace-call", listenVoiceUDP(t), &capturedVoiceResponder{localTag: "n2"})
	req.Replaces = first.CallID + ";to-tag=" + toTag + ";from-tag=" + fromTag
	result, err := agent.HandleInboundVoiceRequest(req)
	if err != nil || result.StatusCode != 0 {
		t.Fatalf("replace INVITE result=%+v err=%v", result, err)
	}
	if !waitCallTerminated(t, call) {
		t.Fatal("replaced early dialog was not terminated")
	}
	if agent.callByID("replace-call") == nil {
		t.Fatal("replacement call was not reserved")
	}
}

func TestSupportedReplacesTokensAreDedupedAcrossSurfaces(t *testing.T) {
	headers := []string{
		policy.DefaultIMSRegisterTemplate().VoiceSupportedHeader,
		voiceInviteSupported,
		"path, 100rel, replaces, outbound, gruu",
		"path, sec-agree, 100rel, replaces, outbound, gruu",
		"path, 100rel, replaces, gruu",
		"100rel,replaces,from-change,histinfo,tdialog",
	}
	for _, header := range headers {
		if countToken(header, "replaces") != 1 {
			t.Fatalf("replaces count in %q = %d", header, countToken(header, "replaces"))
		}
	}
}

func countToken(header, token string) int {
	n := 0
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			n++
		}
	}
	return n
}

func waitCallTerminated(t *testing.T, call *Call) bool {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if call.IsTerminalState() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return call.IsTerminalState()
}
