package voice

import (
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/emergency"
)

const defaultVoiceRTPPort = 12000

// BuildIMSBye builds an in-dialog BYE.
func BuildIMSBye(agent *Agent, call *Call) string {
	if agent == nil || call == nil {
		return ""
	}
	ensureBuilderVoiceDialog(agent, call)
	dialog := call.advanceVoiceCSeq()
	return buildVoiceRequest(dialog, call.CallID(), "BYE", voiceBranch(), "")
}

func buildIMSSessionUpdate(agent *Agent, call *Call) string {
	if agent == nil || call == nil {
		return ""
	}
	dialog := call.advanceVoiceCSeq()
	return attachSessionTimerHeaders(call, buildVoiceRequest(dialog, call.CallID(), "UPDATE", voiceBranch(), ""))
}

func buildIMSReinvite(agent *Agent, call *Call, sdp string) string {
	if agent == nil || call == nil {
		return ""
	}
	dialog := call.advanceVoiceInviteCSeq()
	return attachSessionTimerHeaders(call, buildVoiceRequest(dialog, call.CallID(), "INVITE", voiceBranch(), sdp))
}

func attachSessionTimerHeaders(call *Call, request string) string {
	header := formatSessionTimerHeaders(call)
	if header == "" {
		return request
	}
	return strings.Replace(request, "Content-Length:", header+"Content-Length:", 1)
}

func formatSessionTimerHeaders(call *Call) string {
	expires := formatSessionExpiresHeader(call)
	if expires == "" {
		return ""
	}
	var header strings.Builder
	header.WriteString("Supported: timer\r\n")
	header.WriteString("Session-Expires: ")
	header.WriteString(expires)
	header.WriteString("\r\n")
	if minSE := call.sessionMinSEValue(); minSE > 0 {
		fmt.Fprintf(&header, "Min-SE: %d\r\n", int64(minSE/time.Second))
	}
	return header.String()
}

// BuildIMSACK builds the ACK for the final INVITE response.
func BuildIMSACK(agent *Agent, call *Call) string {
	return buildIMSACKForStatus(agent, call, 200)
}

func buildIMSACKForStatus(agent *Agent, call *Call, statusCode int) string {
	if agent == nil || call == nil {
		return ""
	}
	dialog := ensureBuilderVoiceDialog(agent, call)
	dialog.cseq = dialog.inviteCSeq
	branch := voiceBranch()
	if statusCode >= 300 {
		branch = dialog.inviteBranch
	}
	return buildVoiceRequest(dialog, call.CallID(), "ACK", branch, "")
}

func buildIMSPrack(agent *Agent, call *Call, rseq uint32) string {
	if agent == nil || call == nil || rseq == 0 {
		return ""
	}
	dialog := call.advanceVoiceCSeq()
	request := buildVoiceRequest(dialog, call.CallID(), "PRACK", voiceBranch(), "")
	rack := fmt.Sprintf("RAck: %d %d INVITE\r\n", rseq, dialog.inviteCSeq)
	return strings.Replace(request, "Content-Length: 0\r\n", rack+"Content-Length: 0\r\n", 1)
}

func ensureBuilderVoiceDialog(agent *Agent, call *Call) voiceSIPDialog {
	dialog := call.voiceDialogSnapshot()
	if dialog.remoteURI != "" {
		return dialog
	}
	dialog = fallbackVoiceDialog(agent, call)
	call.setVoiceDialog(&dialog)
	return dialog
}

func buildVoiceRequest(dialog voiceSIPDialog, callID, method, branch, body string) string {
	contentType := ""
	if body != "" {
		contentType = "application/sdp"
	}
	return buildVoiceRequestWithContent(dialog, callID, method, branch, contentType, body)
}

func buildVoiceRequestWithContent(dialog voiceSIPDialog, callID, method, branch, contentType, body string) string {
	target := dialog.remoteTarget
	if target == "" {
		target = dialog.remoteURI
	}
	var request strings.Builder
	fmt.Fprintf(&request, "%s %s SIP/2.0\r\n", method, target)
	writeVoiceDialogHeaders(&request, dialog, callID, method, branch)
	if body != "" && strings.TrimSpace(contentType) != "" {
		fmt.Fprintf(&request, "Content-Type: %s\r\n", contentType)
	}
	fmt.Fprintf(&request, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return request.String()
}

func writeVoiceDialogHeaders(out *strings.Builder, dialog voiceSIPDialog, callID, method, branch string) {
	writeVoiceCoreHeaders(out, dialog, callID, method, branch)
	writeVoiceOptionalHeader(out, "P-Preferred-Identity", "<"+dialog.localURI+">")
	writeVoiceOptionalHeader(out, "Security-Verify", dialog.securityVerify)
	writeVoiceOptionalHeader(out, "P-Access-Network-Info", dialog.pani)
	writeVoiceOptionalHeader(out, "User-Agent", dialog.userAgent)
}

func writeVoiceCoreHeaders(out *strings.Builder, dialog voiceSIPDialog, callID, method, branch string) {
	transport := strings.ToUpper(strings.TrimSpace(dialog.transport))
	if transport == "" {
		transport = "UDP"
	}
	fmt.Fprintf(out, "Via: SIP/2.0/%s %s;rport;branch=%s\r\n", transport, dialog.localAddress, branch)
	for _, route := range dialog.serviceRoute {
		fmt.Fprintf(out, "Route: %s\r\n", route)
	}
	fmt.Fprintf(out, "From: <%s>;tag=%s\r\n", dialog.localURI, dialog.localTag)
	to := "<" + dialog.remoteURI + ">"
	if dialog.remoteTag != "" {
		to += ";tag=" + dialog.remoteTag
	}
	fmt.Fprintf(out, "To: %s\r\nCall-ID: %s\r\nCSeq: %d %s\r\n", to, callID, dialog.cseq, method)
	out.WriteString("Max-Forwards: 70\r\n")
}

func writeVoiceOptionalHeader(out *strings.Builder, name, value string) {
	if strings.TrimSpace(value) != "" && value != "<>" {
		fmt.Fprintf(out, "%s: %s\r\n", name, strings.TrimSpace(value))
	}
}

func fallbackVoiceDialog(agent *Agent, call *Call) voiceSIPDialog {
	snapshot := agent.imsSnapshot()
	domain := snapshot.Realm
	localURI := strings.TrimSpace(snapshot.IMPU)
	if localURI == "" {
		localURI = "sip:unknown@" + domain
	}
	if identity := strings.TrimSpace(snapshot.IMPU); identity != "" {
		localURI = identity
	}
	remoteURI := buildIMSCalledPartyURI(call.Peer(), localURI, domain)
	if emergency.IsEmergencyDestination(call.Peer()) {
		if agent == nil || !agent.emergencyOriginatingEnabled() {
			return voiceSIPDialog{}
		}
		if urn := emergency.ServiceURNFor(call.Peer()); urn != "" {
			remoteURI = urn
		}
	}
	return voiceSIPDialog{
		localURI: localURI, remoteURI: remoteURI, remoteTarget: remoteURI,
		contactURI: localURI, contactHeader: "<" + localURI + ">",
		localAddress: agent.localAddr(), transport: "udp",
		serviceRoute: splitVoiceHeaderValues(effectiveVoiceRoute(snapshot.ServiceRoute, snapshot.Path)), securityVerify: snapshot.SecVerify,
		pani: snapshot.PAccessNetworkInfo, userAgent: snapshot.UserAgent,
		localTag: voiceTag(), inviteBranch: voiceBranch(),
		sessionID: voiceSessionID(), cseq: 1,
		inviteCSeq: 1,
	}
}

type imsConfigView struct {
	Domain string
	IMPI   string
}

func (a *Agent) imsConfig() *imsConfigView {
	if a == nil || a.imsEndpoint() == nil {
		return &imsConfigView{}
	}
	snapshot := a.imsSnapshot()
	return &imsConfigView{Domain: snapshot.Realm, IMPI: strings.TrimPrefix(snapshot.IMPU, "sip:")}
}

// generateBasicSDP restores the original Agent method and byte return type.
func (a *Agent) generateBasicSDP() []byte {
	port := defaultVoiceRTPPort
	var gateway *Gateway
	if a != nil {
		a.mu.RLock()
		gateway = a.gateway
		a.mu.RUnlock()
	}
	if gateway != nil {
		if adapter := gateway.GetClientAdapter(); adapter != nil {
			_, end := adapter.RTPPortRange()
			if end > 1 {
				port = end - 2
			}
		}
	}
	return buildBasicSDP(a.localIP(), port, time.Now().Unix())
}

func generateBasicSDPCurrent(agent *Agent, call *Call) string {
	port := 0
	if call != nil && call.RTPRelay() != nil {
		port = call.RTPRelay().IMSPort()
	}
	if port <= 0 {
		return string(agent.generateBasicSDP())
	}
	return string(buildBasicSDP(agent.localIP(), port, time.Now().Unix()))
}

func buildBasicSDP(ip string, port int, sessionID int64) []byte {
	if ip == "" {
		ip = "0.0.0.0"
	}
	if port <= 0 {
		return nil
	}
	ipFamily := sdpIPFamily(ip)
	return []byte(fmt.Sprintf(
		"v=0\r\n"+
			"o=- %d %d IN %s %s\r\n"+
			"s=VoHive Call\r\n"+
			"c=IN %s %s\r\n"+
			"t=0 0\r\n"+
			"m=audio %d RTP/AVP 104 110 102 108 106 101 0\r\n"+
			"b=AS:80\r\n"+
			"a=rtpmap:104 AMR-WB/16000\r\n"+
			"a=fmtp:104 mode-change-capability=2;max-red=0\r\n"+
			"a=rtpmap:110 AMR-WB/16000\r\n"+
			"a=fmtp:110 octet-align=1;mode-change-capability=2;max-red=0\r\n"+
			"a=rtpmap:102 AMR/8000\r\n"+
			"a=fmtp:102 mode-change-capability=2;max-red=0\r\n"+
			"a=rtpmap:108 AMR/8000\r\n"+
			"a=fmtp:108 octet-align=1;mode-change-capability=2;max-red=0\r\n"+
			"a=rtpmap:106 EVS/16000\r\n"+
			"a=fmtp:106 evs-mode-switch=1;hf-only=0;br=6.6-23.85;bw=wb;ch-aw-recv=-1;max-red=0\r\n"+
			"a=rtpmap:101 telephone-event/8000\r\n"+
			"a=fmtp:101 0-15\r\n"+
			"a=rtpmap:0 PCMU/8000\r\n"+
			"a=curr:qos local none\r\n"+
			"a=curr:qos remote none\r\n"+
			"a=des:qos mandatory local sendrecv\r\n"+
			"a=des:qos optional remote sendrecv\r\n"+
			"a=sendrecv\r\n"+
			"a=ptime:20\r\n"+
			"a=maxptime:20\r\n",
		sessionID, sessionID, ipFamily, ip, ipFamily, ip, port))
}

func (a *Agent) localAddr() string {
	if a == nil {
		return "0.0.0.0:5060"
	}
	if address := strings.TrimSpace(a.imsSnapshot().LocalAddr); address != "" {
		return address
	}
	return "0.0.0.0:5060"
}

func (a *Agent) localIP() string {
	return voiceHost(a.localAddr())
}

func voiceHost(address string) string {
	address = strings.TrimSpace(address)
	if ip := net.ParseIP(strings.Trim(address, "[]")); ip != nil {
		return ip.String()
	}
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(address, "[]")
}

func sdpIPFamily(address string) string {
	ip := net.ParseIP(strings.TrimSpace(address))
	if ip != nil && ip.To4() == nil {
		return "IP6"
	}
	return "IP4"
}

func (a *Agent) mediaPort() int {
	if a == nil {
		return 0
	}
	a.mu.RLock()
	call := a.activeCall
	a.mu.RUnlock()
	if call != nil && call.RTPRelay() != nil {
		return call.RTPRelay().IMSPort()
	}
	return 0
}

func sanitizeVoicePhone(phone string) string {
	var sanitized strings.Builder
	for _, char := range phone {
		if char >= '0' && char <= '9' {
			sanitized.WriteRune(char)
		}
	}
	return sanitized.String()
}

func buildIMSCalledPartyURI(phone, publicIdentity, fallbackDomain string) string {
	digits := sanitizeVoicePhone(phone)
	if digits == "" {
		return ""
	}
	user := digits
	normalized := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(phone), "tel:"))
	if strings.HasPrefix(normalized, "+") {
		user = "+" + digits
	}
	domain := publicIdentityDomain(publicIdentity)
	if domain == "" {
		domain = strings.TrimSpace(fallbackDomain)
	}
	if domain == "" {
		return ""
	}
	return "sip:" + user + "@" + domain + ";user=phone"
}

func publicIdentityDomain(identity string) string {
	identity = strings.Trim(strings.TrimSpace(identity), "<>")
	at := strings.LastIndexByte(identity, '@')
	if at < 0 || at == len(identity)-1 {
		return ""
	}
	domain, _, _ := strings.Cut(identity[at+1:], ";")
	return strings.TrimSpace(domain)
}

func voiceTag() string       { return voiceHex(16) }
func voiceBranch() string    { return "z9hG4bK-" + voiceHex(24) }
func voiceSessionID() string { return voiceHex(32) }

func voiceHex(length int) string {
	const digits = "0123456789abcdef"
	bytes := make([]byte, length)
	_, _ = randVoiceRead(bytes)
	for index := range bytes {
		bytes[index] = digits[int(bytes[index])%len(digits)]
	}
	return string(bytes)
}

func randVoiceRead(bytes []byte) (int, error) { return rand.Read(bytes) }
