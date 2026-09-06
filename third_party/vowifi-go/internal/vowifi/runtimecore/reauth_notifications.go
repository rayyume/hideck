package runtimecore

import (
	"context"
	"sync"

	"github.com/iniwex5/vowifi-go/internal/vowifi/imscore"
	"github.com/iniwex5/vowifi-go/internal/vowifi/profile"
)

// Candidate callbacks must not replace the live runtime until startup succeeds.
type candidateNotifications struct {
	mu        sync.Mutex
	pending   []func()
	committed bool
	discarded bool
	draining  bool
}

func (g *candidateNotifications) emit(fn func()) {
	g.mu.Lock()
	if g.discarded {
		g.mu.Unlock()
		return
	}
	g.pending = append(g.pending, fn)
	start := g.committed && !g.draining
	if start {
		g.draining = true
	}
	g.mu.Unlock()
	if start {
		g.drain()
	}
}

func (g *candidateNotifications) commit() {
	g.mu.Lock()
	g.committed = true
	g.draining = true
	g.mu.Unlock()
	g.drain()
}

func (g *candidateNotifications) drain() {
	for {
		g.mu.Lock()
		if len(g.pending) == 0 {
			g.draining = false
			g.mu.Unlock()
			return
		}
		fn := g.pending[0]
		g.pending[0] = nil
		g.pending = g.pending[1:]
		g.mu.Unlock()
		fn()
	}
}

func (g *candidateNotifications) discard() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.committed {
		g.discarded = true
		g.pending = nil
	}
}

type candidateEventSink struct {
	gate *candidateNotifications
	sink RuntimeEventSink[*SessionResult]
}

func (s candidateEventSink) OnRuntimeEvent(ctx context.Context, event RuntimeEvent[*SessionResult]) {
	s.gate.emit(func() { s.sink.OnRuntimeEvent(ctx, event) })
}

func gateCandidateNotifications(req *RuntimeStartRequest) *candidateNotifications {
	g := &candidateNotifications{}
	if req.Observer != nil {
		req.Observer = candidateEventSink{g, req.Observer}
	}
	h := req.Hooks
	if h.Events != nil {
		req.Hooks.Events = candidateEventSink{g, h.Events}
	}
	if h.OnPrepared != nil {
		req.Hooks.OnPrepared = func(ctx context.Context, p profile.PreparedSession) { g.emit(func() { h.OnPrepared(ctx, p) }) }
	}
	if h.OnConnecting != nil {
		req.Hooks.OnConnecting = func(ctx context.Context) { g.emit(func() { h.OnConnecting(ctx) }) }
	}
	if h.OnEstablished != nil {
		req.Hooks.OnEstablished = func(ctx context.Context, r RuntimeStartResult) { g.emit(func() { h.OnEstablished(ctx, r) }) }
	}
	if h.OnIMSRegistered != nil {
		req.Hooks.OnIMSRegistered = func(ctx context.Context) { g.emit(func() { h.OnIMSRegistered(ctx) }) }
	}
	if h.OnSMSReady != nil {
		req.Hooks.OnSMSReady = func(ctx context.Context) { g.emit(func() { h.OnSMSReady(ctx) }) }
	}
	if h.OnSMSReadinessChanged != nil {
		req.Hooks.OnSMSReadinessChanged = func(ctx context.Context, r imscore.SMSReadiness) { g.emit(func() { h.OnSMSReadinessChanged(ctx, r) }) }
	}
	if h.OnError != nil {
		req.Hooks.OnError = func(ctx context.Context, err error) { g.emit(func() { h.OnError(ctx, err) }) }
	}
	return g
}
