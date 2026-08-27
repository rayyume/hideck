package voice

import (
	"errors"
	"strings"
	"testing"

	"github.com/iniwex5/vowifi-go/internal/vowifi/emergency"
	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

func TestEmergencyINVITEUsesServiceURNAndPriority(t *testing.T) {
	agent := &Agent{}
	agent.SetAllowEmergencyCalls(true)
	call := NewCall(agent, callstate.DirectionOutbound, "sos-call", "999")
	call.setVoiceDialog(&voiceSIPDialog{
		localURI: "sip:+447840844894@o2.co.uk", remoteURI: emergency.ServiceURN,
		remoteTarget:  emergency.ServiceURN,
		contactHeader: `<sip:binding@10.0.0.2:5060>;sos;audio`,
		localAddress:  "10.0.0.2:5060", transport: "tcp",
		serviceRoute: []string{"<sip:pcscf.example;lr>"},
		pani:         "IEEE-802.11;country=GB", userAgent: "test-agent",
		localTag: "local-tag", inviteBranch: "z9hG4bK-sos",
		cseq: 4, inviteCSeq: 4,
	})
	invite := buildIMSInviteWithSDP(agent, call, "v=0\r\n")
	checks := []string{
		"INVITE urn:service:sos SIP/2.0",
		"To: <urn:service:sos>",
		"Priority: emergency",
		"P-Preferred-Identity: <sip:+447840844894@o2.co.uk>",
		"P-Access-Network-Info: IEEE-802.11;country=GB",
		`P-Preferred-Service: urn:urn-7:3gpp-service.ims.icsi.mmtel`,
	}
	for _, value := range checks {
		if !strings.Contains(invite, value) {
			t.Fatalf("emergency INVITE missing %q: %s", value, invite)
		}
	}
}

func TestPrepareVoiceDialogRejectsEmergencyByDefault(t *testing.T) {
	agent := newTestAgent(t)
	call := NewCall(agent, callstate.DirectionOutbound, "blocked-sos", "999")
	err := agent.prepareVoiceDialog(call, "999")
	if !errors.Is(err, emergency.ErrOriginatingDisabled) {
		t.Fatalf("prepare error = %v", err)
	}
}

func TestPrepareVoiceDialogMapsEmergencyNumberWhenEnabled(t *testing.T) {
	agent := newTestAgent(t)
	agent.SetAllowEmergencyCalls(true)
	call := NewCall(agent, callstate.DirectionOutbound, "allowed-sos", "112")
	if err := agent.prepareVoiceDialog(call, "112"); err != nil {
		t.Fatal(err)
	}
	dialog := call.voiceDialogSnapshot()
	if dialog.remoteURI != emergency.ServiceURN {
		t.Fatalf("remote URI = %q", dialog.remoteURI)
	}
}

func TestNormalINVITEDoesNotAddPriorityEmergency(t *testing.T) {
	agent := &Agent{}
	call := NewCall(agent, callstate.DirectionOutbound, "normal-call", "+447942985429")
	call.setVoiceDialog(&voiceSIPDialog{
		localURI:     "sip:+447840844894@o2.co.uk",
		remoteURI:    "sip:+447942985429@o2.co.uk;user=phone",
		localAddress: "10.0.0.2:5060", transport: "tcp",
		localTag: "local-tag", inviteBranch: "z9hG4bK-normal",
		cseq: 4, inviteCSeq: 4,
	})
	invite := buildIMSInviteWithSDP(agent, call, "v=0\r\n")
	if strings.Contains(invite, "Priority: emergency") {
		t.Fatalf("normal INVITE included emergency priority: %s", invite)
	}
	if strings.Contains(invite, "urn:service:sos") {
		t.Fatalf("normal INVITE used SOS URN: %s", invite)
	}
}
