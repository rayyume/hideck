package runtimecore

import (
	"context"
	"errors"
	"testing"

	"github.com/iniwex5/vowifi-go/engine/swu"
	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
)

func TestReauthCandidateFailureDoesNotPublishRuntimeState(t *testing.T) {
	recorder := &eventRecorder{}
	req := baseRuntimeRequest(recorder)
	readinessCalls := 0
	req.Hooks.OnSMSReadinessChanged = func(context.Context, imscore.SMSReadiness) { readinessCalls++ }
	req.Hooks.OnError = func(context.Context, error) { t.Error("candidate error reached active runtime hook") }
	var candidateReadiness func(imscore.SMSReadiness)
	wantErr := errors.New("candidate IMS failed after Child SA established")
	req.SessionStarter = func(_ context.Context, cfg SessionConfig) (*SessionResult, error) {
		cfg.OnTunnelReady(&SessionResult{Snapshot: swu.SessionSnapshot{Established: true}})
		cfg.OnSMSReadinessChanged(imscore.SMSReadiness{})
		candidateReadiness = cfg.OnSMSReadinessChanged
		return nil, wantErr
	}
	if _, err := startOverlappingReauth(context.Background(), &req, nil); !errors.Is(err, wantErr) {
		t.Fatalf("reauth error = %v", err)
	}
	candidateReadiness(imscore.SMSReadiness{})
	if got := recorder.kinds(); len(got) != 0 || readinessCalls != 0 {
		t.Fatalf("failed candidate published events %v, readiness callbacks %d", got, readinessCalls)
	}
}

func TestReauthSuccessRetiresPreviousSessionCallbacks(t *testing.T) {
	req := baseRuntimeRequest(&eventRecorder{})
	var configs []SessionConfig
	ready := false
	req.Hooks.OnSMSReadinessChanged = func(_ context.Context, r imscore.SMSReadiness) { ready = r.Ready }
	req.SessionStarter = func(_ context.Context, cfg SessionConfig) (*SessionResult, error) {
		configs = append(configs, cfg)
		cfg.OnSMSReadinessChanged(imscore.SMSReadiness{Registered: true, Ready: true})
		return &SessionResult{Snapshot: swu.SessionSnapshot{Established: true}}, nil
	}
	old, err := (Runtime{}).startOnce(context.Background(), &req)
	if err != nil {
		t.Fatal(err)
	}
	_, err = startOverlappingReauth(context.Background(), &req, old.Session)
	if err != nil {
		t.Fatal(err)
	}
	configs[0].OnSMSReadinessChanged(imscore.SMSReadiness{})
	if !ready {
		t.Fatal("old session shutdown cleared successor readiness")
	}
	configs[1].OnSMSReadinessChanged(imscore.SMSReadiness{})
	if ready {
		t.Fatal("successor readiness callback was discarded")
	}
}

func TestReauthFailureKeepsPreviousSessionCallbacks(t *testing.T) {
	req := baseRuntimeRequest(&eventRecorder{})
	var oldCallback func(imscore.SMSReadiness)
	ready := false
	req.Hooks.OnSMSReadinessChanged = func(_ context.Context, r imscore.SMSReadiness) { ready = r.Ready }
	req.SessionStarter = func(_ context.Context, cfg SessionConfig) (*SessionResult, error) {
		oldCallback = cfg.OnSMSReadinessChanged
		return &SessionResult{Snapshot: swu.SessionSnapshot{Established: true}}, nil
	}
	old, err := (Runtime{}).startOnce(context.Background(), &req)
	if err != nil {
		t.Fatal(err)
	}
	req.SessionStarter = func(context.Context, SessionConfig) (*SessionResult, error) {
		return nil, errors.New("candidate failed")
	}
	if _, err := startOverlappingReauth(context.Background(), &req, old.Session); err == nil {
		t.Fatal("expected candidate failure")
	}
	oldCallback(imscore.SMSReadiness{Registered: true, Ready: true})
	if !ready {
		t.Fatal("failed candidate retired the live session callbacks")
	}
}

func TestReauthCandidatePublishesOnlyAfterSuccessAndKeepsLiveCallbacks(t *testing.T) {
	recorder := &eventRecorder{}
	req := baseRuntimeRequest(recorder)
	readinessCalls := 0
	req.Hooks.OnSMSReadinessChanged = func(context.Context, imscore.SMSReadiness) { readinessCalls++ }
	var ready func(imscore.SMSReadiness)
	req.SessionStarter = func(_ context.Context, cfg SessionConfig) (*SessionResult, error) {
		result := &SessionResult{Snapshot: swu.SessionSnapshot{Established: true}}
		cfg.OnTunnelReady(result)
		ready = cfg.OnSMSReadinessChanged
		ready(imscore.SMSReadiness{Registered: true, Ready: true})
		if len(recorder.kinds()) != 0 || readinessCalls != 0 {
			t.Error("candidate published before success")
		}
		return result, nil
	}
	if _, err := startOverlappingReauth(context.Background(), &req, nil); err != nil {
		t.Fatal(err)
	}
	if len(recorder.kinds()) == 0 || readinessCalls != 1 {
		t.Fatal("successful candidate was not published")
	}
	ready(imscore.SMSReadiness{})
	if readinessCalls != 2 {
		t.Fatal("successor callbacks stopped forwarding")
	}
}
