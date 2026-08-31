package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

func (a *Agent) queuePreconditionStatusUpdate(call *Call, remoteSDP string) {
	if a == nil || call == nil || strings.TrimSpace(remoteSDP) == "" {
		return
	}
	a.runCallTask(call, "precondition_update", func() {
		ctx, cancel := context.WithTimeout(context.Background(), voiceHangupTimeout)
		defer cancel()
		if err := a.sendPreconditionStatusUpdate(ctx, call, remoteSDP); err != nil {
			logging.WarnRate("ims-precondition-update:"+call.CallID(), 10*time.Second,
				"IMS precondition UPDATE failed; INVITE can still complete",
				"device", a.deviceID, "connected", call.IsConnected(), "err", err)
		}
	})
}

func (a *Agent) sendPreconditionStatusUpdate(
	ctx context.Context,
	call *Call,
	remoteSDP string,
) error {
	if a == nil || call == nil || strings.TrimSpace(remoteSDP) == "" {
		return nil
	}
	if !call.claimPreconditionStatusUpdate() {
		return nil
	}
	statusSDP, err := buildPreconditionStatusSDP(call.imsLocalSDPValue(), remoteSDP)
	if err != nil {
		return err
	}
	response, err := a.sendCallDialogRequest(ctx, call, buildIMSPreconditionUpdate(a, call, statusSDP))
	if err != nil {
		return fmt.Errorf("voice: precondition UPDATE failed: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("voice: precondition UPDATE rejected: %d %s", response.StatusCode, response.Reason)
	}
	if !isVoiceSDPContentType(voiceResponseHeader(response.Headers, "Content-Type")) || len(response.Body) == 0 {
		return errors.New("voice: precondition UPDATE response has no application/sdp answer")
	}
	if err := a.updateRemoteMedia(call, response); err != nil {
		return fmt.Errorf("voice: precondition UPDATE answer: %w", err)
	}
	clientSDP, _ := call.localSDPs()
	call.setLocalSDP(clientSDP, statusSDP)
	a.applyCallPreconditions(call, string(response.Body))
	logging.Info("IMS precondition UPDATE succeeded", "device", a.deviceID, "status", response.StatusCode)
	return nil
}
