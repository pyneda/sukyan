package fuzz

import (
	"context"
	"sync"

	"github.com/pyneda/sukyan/pkg/playground/stream"
)

// Registry tracks the runtime state for in-flight fuzz runs: the cancel func
// for each run's engine ctx, the broadcaster the engine publishes to, and a
// pause gate workers consult before scheduling new requests. All are looked
// up by runID by the cancellation, streaming, and pause/resume endpoints.
//
// Lifecycle: api.FuzzRequest creates a run, calls registry.Register, then
// launches Run in a goroutine. When Run returns (success / cancel / error),
// the goroutine calls registry.Unregister.
type Registry struct {
	mu     sync.Mutex
	cancel map[uint]context.CancelFunc
	bcast  map[uint]*stream.Broadcaster
	gates  map[uint]*PauseGate
}

var defaultRegistry = NewRegistry()

// Default returns the process-wide Registry. Single-process model for now;
// horizontal scaling is out of scope.
func Default() *Registry { return defaultRegistry }

// NewRegistry returns an empty registry. Useful in tests.
func NewRegistry() *Registry {
	return &Registry{
		cancel: make(map[uint]context.CancelFunc),
		bcast:  make(map[uint]*stream.Broadcaster),
		gates:  make(map[uint]*PauseGate),
	}
}

// Register records the cancel func, broadcaster, and a fresh pause gate for
// a run. Replaces any previous registration (the old cancel is called and the
// old gate is resumed so any waiters there don't deadlock).
func (r *Registry) Register(runID uint, cancel context.CancelFunc, bcast *stream.Broadcaster) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.cancel[runID]; ok && old != nil {
		old()
	}
	if oldGate, ok := r.gates[runID]; ok && oldGate != nil {
		oldGate.Resume()
	}
	r.cancel[runID] = cancel
	r.bcast[runID] = bcast
	r.gates[runID] = NewPauseGate()
}

// Unregister removes a run's entries. Idempotent. Does NOT close the
// broadcaster; the caller (engine cleanup) handles that. Resumes the gate
// first so any worker still blocked there unwinds cleanly.
func (r *Registry) Unregister(runID uint) {
	r.mu.Lock()
	gate := r.gates[runID]
	delete(r.cancel, runID)
	delete(r.bcast, runID)
	delete(r.gates, runID)
	r.mu.Unlock()
	if gate != nil {
		gate.Resume()
	}
}

// Cancel signals cancellation for the given run. Returns true if a cancel
// func was registered and called; false if the run is unknown (already
// finished, never existed, or already cancelled). Also resumes the gate so
// any worker waiting there unblocks and observes ctx cancellation.
func (r *Registry) Cancel(runID uint) bool {
	r.mu.Lock()
	cancel := r.cancel[runID]
	gate := r.gates[runID]
	r.mu.Unlock()
	if cancel == nil {
		return false
	}
	if gate != nil {
		gate.Resume()
	}
	cancel()
	return true
}

// Broadcaster returns the broadcaster for the given run, or nil if the run
// is no longer registered. Lazy creation is intentionally NOT done here —
// the streaming endpoint will fall back to a freshly-created bounded-replay
// broadcaster so late subscribers to finished runs still get history.
func (r *Registry) Broadcaster(runID uint) *stream.Broadcaster {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bcast[runID]
}

// Gate returns the pause gate for the given run, or nil if the run is not
// registered. The engine reads this once at run start so it can Wait on the
// same gate the API endpoints flip.
func (r *Registry) Gate(runID uint) *PauseGate {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.gates[runID]
}

// Pause flips the run's gate into the paused state. Returns true if the gate
// transitioned (was running), false if the run is unknown or was already
// paused. The API layer is responsible for the DB status transition.
func (r *Registry) Pause(runID uint) bool {
	r.mu.Lock()
	gate := r.gates[runID]
	r.mu.Unlock()
	if gate == nil {
		return false
	}
	return gate.Pause()
}

// Resume flips the run's gate back into the not-paused state. Returns true
// if the gate transitioned (was paused), false if the run is unknown or was
// not paused.
func (r *Registry) Resume(runID uint) bool {
	r.mu.Lock()
	gate := r.gates[runID]
	r.mu.Unlock()
	if gate == nil {
		return false
	}
	return gate.Resume()
}

// Has reports whether a run is currently registered (i.e. has a live engine
// context). Used by the API layer to distinguish "unknown run" from "known
// but not paused".
func (r *Registry) Has(runID uint) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.gates[runID]
	return ok
}
