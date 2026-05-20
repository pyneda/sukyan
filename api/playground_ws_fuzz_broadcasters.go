package api

import (
	"sync"

	"github.com/pyneda/sukyan/pkg/playground/stream"
)

// wsFuzzBroadcasters keys per-run broadcasters by run ID. Distinct from the
// HTTP fuzz broadcaster registry so a WS run 42 and an HTTP run 42 don't share
// state (both registries are keyed by runID).
type wsFuzzBroadcasters struct {
	mu   sync.Mutex
	live map[uint]*stream.Broadcaster
}

// newWsFuzzBroadcasters returns an empty registry. Callers should normally use
// wsFuzzBroadcastersDefault below; this constructor exists for tests.
func newWsFuzzBroadcasters() *wsFuzzBroadcasters {
	return &wsFuzzBroadcasters{live: map[uint]*stream.Broadcaster{}}
}

// Acquire returns the broadcaster for runID, creating one on first call.
// Buffer sizes (64 events per subscriber, 1000-event history) match the
// wsreplay manager's defaults — see pkg/playground/stream/broadcaster.go.
func (r *wsFuzzBroadcasters) Acquire(runID uint) *stream.Broadcaster {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.live[runID]; ok {
		return b
	}
	b := stream.NewBroadcaster(64, 1000)
	r.live[runID] = b
	return b
}

// Lookup returns the broadcaster for runID if one is registered, else nil.
func (r *wsFuzzBroadcasters) Lookup(runID uint) *stream.Broadcaster {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live[runID]
}

// Release removes the entry. Subsequent Acquire(runID) returns a fresh
// broadcaster. The caller is responsible for closing the broadcaster before
// releasing it.
func (r *wsFuzzBroadcasters) Release(runID uint) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.live, runID)
}

// wsFuzzBroadcastersDefault is the process-wide registry used by the handlers.
var wsFuzzBroadcastersDefault = newWsFuzzBroadcasters()
