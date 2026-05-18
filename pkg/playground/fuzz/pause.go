package fuzz

import (
	"context"
	"sync"
)

// PauseGate is a one-shot reusable gate that workers consult before scheduling
// new work. When not paused, Wait returns immediately. When paused, Wait blocks
// until either Resume is called or ctx is cancelled.
//
// Lifecycle: Pause sets the gate's internal channel to a fresh open channel
// that workers select on. Resume closes that channel (unblocking everyone) and
// drops the reference. Multiple Pauses while already paused are no-ops;
// Resumes while not paused are no-ops.
type PauseGate struct {
	mu     sync.Mutex
	paused chan struct{}
}

// NewPauseGate returns a gate in the not-paused state.
func NewPauseGate() *PauseGate {
	return &PauseGate{}
}

// Pause flips the gate into the paused state. Returns true if the state
// changed (was not paused), false if it was already paused.
func (g *PauseGate) Pause() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.paused != nil {
		return false
	}
	g.paused = make(chan struct{})
	return true
}

// Resume flips the gate back into the not-paused state and unblocks any
// workers waiting on it. Returns true if the state changed (was paused),
// false if it was not paused.
func (g *PauseGate) Resume() bool {
	g.mu.Lock()
	ch := g.paused
	g.paused = nil
	g.mu.Unlock()
	if ch == nil {
		return false
	}
	close(ch)
	return true
}

// IsPaused reports the current state. Mostly useful for tests; callers should
// prefer Wait, which atomically observes-and-blocks.
func (g *PauseGate) IsPaused() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.paused != nil
}

// Wait blocks while the gate is paused. Returns ctx.Err() if ctx is cancelled
// while waiting; nil otherwise. Returns nil immediately if not paused.
//
// Snapshot semantics: Wait reads the paused channel once and then selects.
// If Resume runs between the snapshot and the select, the channel is already
// closed and the select returns immediately — no lost wakeups.
func (g *PauseGate) Wait(ctx context.Context) error {
	g.mu.Lock()
	ch := g.paused
	g.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
