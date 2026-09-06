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
	candidate := *req
	gate := gateCandidateNotifications(&candidate)
	defer gate.discard()
	candidate.omitInitialContact = true
	if name := strings.TrimSpace(req.Dataplane.TUNName); name != "" {
		candidate.Dataplane.TUNName = name + overlappingReauthTUNSuffix
	}
	logging.Info("starting overlapping IKE reauth on a new SA",
		"device", req.DeviceID, "trace_id", req.TraceID)
	started, err := (Runtime{}).startOnce(ctx, &candidate)
	if err != nil {
		return nil, err
	}
	if started.Session == nil || !started.Session.Snapshot.Established {
		if started.Session != nil {
			defaultStopSession(context.Background(), started.Session)
		}
		return nil, errors.New("runtimecore: overlapping reauth did not establish a Child SA")
	}
	gate.commit()
	return started.Session, nil
}
