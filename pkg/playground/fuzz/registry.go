package fuzz

import (
	"context"
	"sync"

	"github.com/pyneda/sukyan/pkg/playground/stream"
)

// Registry tracks the runtime state for in-flight fuzz runs: the cancel func
// for each run's engine ctx, and the broadcaster the engine publishes to.
// Both are looked up by runID by the cancellation and streaming endpoints.
//
// Lifecycle: api.FuzzRequest creates a run, calls registry.Register, then
// launches Run in a goroutine. When Run returns (success / cancel / error),
// the goroutine calls registry.Unregister.
type Registry struct {
	mu     sync.Mutex
	cancel map[uint]context.CancelFunc
	bcast  map[uint]*stream.Broadcaster
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
	}
}

// Register records the cancel func and broadcaster for a run. Replaces any
// previous registration (cancel is called on the old one if present).
func (r *Registry) Register(runID uint, cancel context.CancelFunc, bcast *stream.Broadcaster) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if old, ok := r.cancel[runID]; ok && old != nil {
		old()
	}
	r.cancel[runID] = cancel
	r.bcast[runID] = bcast
}

// Unregister removes a run's entries. Idempotent. Does NOT close the
// broadcaster; the caller (engine cleanup) handles that.
func (r *Registry) Unregister(runID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.cancel, runID)
	delete(r.bcast, runID)
}

// Cancel signals cancellation for the given run. Returns true if a cancel
// func was registered and called; false if the run is unknown (already
// finished, never existed, or already cancelled).
func (r *Registry) Cancel(runID uint) bool {
	r.mu.Lock()
	cancel := r.cancel[runID]
	r.mu.Unlock()
	if cancel == nil {
		return false
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
