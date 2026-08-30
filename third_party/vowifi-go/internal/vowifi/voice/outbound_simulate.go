package voice

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const (
	simulateRegistrationTimeout = 10 * time.Second
	simulateRegistrationPoll    = 500 * time.Millisecond
)

// SimulateCall runs the recovered timed-call workflow over real IMS and media.
func (a *Agent) SimulateCall(
	ctx context.Context,
	request SimulateCallRequest,
) (*SimulateCallResult, error) {
	if a == nil || a.imsEndpoint() == nil {
		return nil, errors.New("voice: IMS provider is unavailable")
	}
	request.Callee = strings.TrimSpace(request.Callee)
	if request.Callee == "" {
		return nil, errors.New("voice: callee is empty")
	}
	if request.HoldSeconds < 0 {
		return nil, errors.New("voice: hold seconds must not be negative")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.waitForSimulateRegistration(ctx); err != nil {
		return &SimulateCallResult{Reason: err.Error()}, err
	}
	if a.IsBusy() {
		err := errors.New("voice: another call is active")
		return &SimulateCallResult{Reason: err.Error()}, err
	}
	startedAt := time.Now()
	inviteCtx, cancelInvite := context.WithTimeout(
		ctx, voiceInviteTimeout+outboundCancelSettle,
	)
	call, err := a.startSimulateClientInvite(inviteCtx, request)
	cancelInvite()
	if err != nil {
		return simulateFailure(startedAt, err), err
	}
	if request.OnConnected != nil {
		request.OnConnected()
	}
	if request.HoldSeconds >= 8 {
		go a.exerciseSimulatedHoldResume(call)
	}
	return a.holdSimulatedCall(ctx, call, request.HoldSeconds, startedAt)
}

func (a *Agent) exerciseSimulatedHoldResume(call *Call) {
	if a == nil || call == nil {
		return
	}
	callID := call.CallID()
	time.Sleep(2 * time.Second)
	if err := a.HoldCall(context.Background(), callID); err != nil {
		logging.Info("IMS simulated hold failed", "device", a.deviceID, "err", err)
		return
	}
	logging.Info("IMS simulated hold sent", "device", a.deviceID)
	time.Sleep(3 * time.Second)
	if err := a.ResumeCall(context.Background(), callID); err != nil {
		logging.Info("IMS simulated resume failed", "device", a.deviceID, "err", err)
		return
	}
	logging.Info("IMS simulated resume sent", "device", a.deviceID)
}

func (a *Agent) waitForSimulateRegistration(ctx context.Context) error {
	endpoint := a.imsEndpoint()
	if endpoint == nil {
		return errors.New("voice: IMS provider is unavailable")
	}
	if endpoint.IsRegistered() {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, simulateRegistrationTimeout)
	defer cancel()
	ticker := time.NewTicker(simulateRegistrationPoll)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("voice: IMS registration wait failed: %w", waitCtx.Err())
		case <-ticker.C:
			if endpoint.IsRegistered() {
				return nil
			}
		}
	}
}

func (a *Agent) holdSimulatedCall(
	ctx context.Context,
	call *Call,
	holdSeconds int,
	startedAt time.Time,
) (*SimulateCallResult, error) {
	timer := time.NewTimer(time.Duration(holdSeconds) * time.Second)
	defer timer.Stop()
	select {
	case <-call.Done:
		result := &SimulateCallResult{
			Success:    false,
			DurationMs: time.Since(startedAt).Milliseconds(),
			Reason:     "中途被动终止",
		}
		_ = applySimulatedCaptureResult(result, call)
		return result, nil
	case <-ctx.Done():
		return a.waitSimulateCancelSettle(call, startedAt, ctx.Err())
	case mediaErr := <-call.MediaErrors():
		if mediaErr == nil {
			mediaErr = errors.New("voice: simulated media stopped")
		}
		return a.waitSimulateCancelSettle(call, startedAt, mediaErr)
	case <-timer.C:
		if err := a.closeSimulatedCall(call); err != nil {
			return simulateFailure(startedAt, err), err
		}
		result := &SimulateCallResult{
			Success:    true,
			DurationMs: time.Since(startedAt).Milliseconds(),
			Reason:     "定时正常挂断",
		}
		if err := applySimulatedCaptureResult(result, call); err != nil {
			return simulateFailure(startedAt, err), err
		}
		return result, nil
	}
}

func applySimulatedCaptureResult(result *SimulateCallResult, call *Call) error {
	if result == nil || call == nil {
		return nil
	}
	pcapPath, audioPath, codec, err := call.captureResult()
	result.PCAPPath = pcapPath
	result.AudioPath = audioPath
	result.AudioCodec = codec
	return err
}

func (a *Agent) startSimulateClientInvite(ctx context.Context, request SimulateCallRequest) (*Call, error) {
	return a.dialContextWithCapture(ctx, request.Callee, "", request.CaptureBasePath)
}

func (a *Agent) waitSimulateCancelSettle(
	call *Call,
	startedAt time.Time,
	cause error,
) (*SimulateCallResult, error) {
	return a.failAndCloseSimulatedCall(call, startedAt, cause)
}

func (a *Agent) failAndCloseSimulatedCall(
	call *Call,
	startedAt time.Time,
	cause error,
) (*SimulateCallResult, error) {
	err := errors.Join(cause, a.closeSimulatedCall(call))
	return simulateFailure(startedAt, err), err
}

func (a *Agent) closeSimulatedCall(call *Call) error {
	if call == nil || call.IsTerminalState() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), voiceHangupTimeout)
	defer cancel()
	return a.HangupContext(ctx, call.CallID())
}

func simulateFailure(startedAt time.Time, err error) *SimulateCallResult {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	return &SimulateCallResult{
		Success: false, DurationMs: time.Since(startedAt).Milliseconds(), Reason: reason,
	}
}
