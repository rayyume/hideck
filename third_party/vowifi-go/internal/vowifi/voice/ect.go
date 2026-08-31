package voice

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/voice/callstate"
)

var (
	// ErrECTRequiresReplaces is returned when consultative transfer cannot
	// build a Replaces parameter, or the remote party cannot be transferred
	// that way.
	ErrECTRequiresReplaces = errors.New("voice: consultative transfer requires Replaces")
	// ErrECTRequiresTwoCalls is returned when either dialog is missing.
	ErrECTRequiresTwoCalls = errors.New("voice: consultative transfer requires two connected calls")
)

// TransferConsultative sends an outbound REFER with Replaces on the
// transferee dialog, waits for NOTIFY sipfrag, then releases both calls.
func (a *Agent) TransferConsultative(ctx context.Context, transfereeCallID, targetCallID string) error {
	if a == nil {
		return errors.New("voice: nil agent")
	}
	transferee := a.callByID(strings.TrimSpace(transfereeCallID))
	target := a.callByID(strings.TrimSpace(targetCallID))
	if transferee == nil || target == nil || transferee == target {
		return ErrECTRequiresTwoCalls
	}
	if transferee.CallState() != callstate.StateConnected || target.CallState() != callstate.StateConnected {
		return ErrECTRequiresTwoCalls
	}
	referTo, err := formatConsultativeReferTo(target)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	wait := transferee.armReferSipfrag()
	response, err := a.sendCallDialogRequest(ctx, transferee, buildIMSRefer(a, transferee, referTo))
	if err != nil {
		return err
	}
	if response.StatusCode != 0 && response.StatusCode != 202 && (response.StatusCode < 200 || response.StatusCode >= 300) {
		if response.StatusCode == 403 || response.StatusCode == 420 || response.StatusCode == 501 {
			return fmt.Errorf("%w: REFER rejected %d", ErrECTRequiresReplaces, response.StatusCode)
		}
		return fmt.Errorf("voice: consultative REFER rejected: %d %s", response.StatusCode, response.Reason)
	}
	sipfrag, err := waitReferSipfrag(ctx, wait)
	if err != nil {
		return err
	}
	status := parseSipfragStatus(sipfrag)
	if status < 200 || status >= 300 {
		return fmt.Errorf("voice: consultative transfer failed: %s", strings.TrimSpace(sipfrag))
	}
	hangupCtx, cancel := context.WithTimeout(context.Background(), voiceHangupTimeout)
	defer cancel()
	return errors.Join(
		a.HangupContext(hangupCtx, transferee.CallID()),
		a.HangupContext(hangupCtx, target.CallID()),
	)
}

func formatConsultativeReferTo(target *Call) (string, error) {
	if target == nil {
		return "", ErrECTRequiresReplaces
	}
	callID, fromTag, toTag := target.replacesDialogID()
	if strings.TrimSpace(callID) == "" || strings.TrimSpace(fromTag) == "" || strings.TrimSpace(toTag) == "" {
		return "", ErrECTRequiresReplaces
	}
	dialog := target.voiceDialogSnapshot()
	uri := strings.TrimSpace(dialog.remoteURI)
	if uri == "" {
		uri = strings.TrimSpace(target.Peer())
	}
	if uri == "" {
		return "", ErrECTRequiresReplaces
	}
	if !strings.Contains(uri, ":") {
		uri = "sip:" + uri
	}
	replaces := escapeReferToToken(callID) + "%3Bto-tag%3D" + escapeReferToToken(toTag) +
		"%3Bfrom-tag%3D" + escapeReferToToken(fromTag)
	return "<" + uri + ";method=INVITE?Replaces=" + replaces + ">", nil
}

func escapeReferToToken(value string) string {
	var out strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '.' || c == '_' || c == '~' {
			out.WriteByte(c)
			continue
		}
		fmt.Fprintf(&out, "%%%02X", c)
	}
	return out.String()
}

func waitReferSipfrag(ctx context.Context, wait <-chan string) (string, error) {
	if wait == nil {
		return "", errors.New("voice: consultative transfer has no NOTIFY waiter")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case sipfrag, ok := <-wait:
		if !ok {
			return "", errors.New("voice: consultative transfer NOTIFY closed")
		}
		return sipfrag, nil
	case <-ctx.Done():
		return "", fmt.Errorf("voice: waiting for REFER NOTIFY: %w", ctx.Err())
	}
}

func parseSipfragStatus(body string) int {
	line, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if !strings.HasPrefix(line, "SIP/2.0 ") {
		return 0
	}
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	n, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0
	}
	return n
}
