package runtimecore

import (
	"context"
	"errors"
	"strings"

	"github.com/iniwex5/vowifi-go/internal/vowifi/logging"
)

const overlappingReauthTUNSuffix = "-reauth"

func startOverlappingReauth(
	ctx context.Context,
	req *RuntimeStartRequest,
) (*SessionResult, error) {
	if req == nil {
		return nil, errors.New("runtimecore: nil overlapping reauth request")
	}
	previousOmit := req.omitInitialContact
	previousTUN := req.Dataplane.TUNName
	req.omitInitialContact = true
	if name := strings.TrimSpace(previousTUN); name != "" {
		req.Dataplane.TUNName = name + overlappingReauthTUNSuffix
	}
	defer func() {
		req.omitInitialContact = previousOmit
		req.Dataplane.TUNName = previousTUN
	}()
	logging.Info("starting overlapping IKE reauth on a new SA",
		"device", req.DeviceID, "trace_id", req.TraceID)
	started, err := (Runtime{}).startOnce(ctx, req)
	if err != nil {
		return nil, err
	}
	if started.Session == nil || !started.Session.Snapshot.Established {
		if started.Session != nil {
			defaultStopSession(context.Background(), started.Session)
		}
		return nil, errors.New("runtimecore: overlapping reauth did not establish a Child SA")
	}
	return started.Session, nil
}
