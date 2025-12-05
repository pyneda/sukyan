// Package control provides in-memory state management for scan pause/resume/cancel operations.
// It provides fast checkpoint operations without requiring database access for every check.
package control

import (
	"context"
	"sync"
)

// State represents the current state of a scan
type State int

const (
	StateRunning State = iota
	StatePaused
	StateCancelled
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateRunning:
		return "running"
	case StatePaused:
		return "paused"
	case StateCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// ScanControl manages pause/resume/cancel state for a scan.
// It is safe for concurrent use by multiple goroutines.
type ScanControl struct {
	scanID uint
	state  State
	mu     sync.RWMutex

	// pauseCond is used to block workers when paused
	pauseCond *sync.Cond

	// ctx is cancelled when the scan is cancelled
	ctx    context.Context
	cancel context.CancelFunc
}

// New creates a new ScanControl in running state
func New(scanID uint) *ScanControl {
	ctx, cancel := context.WithCancel(context.Background())
	sc := &ScanControl{
		scanID: scanID,
		state:  StateRunning,
		ctx:    ctx,
		cancel: cancel,
	}
	sc.pauseCond = sync.NewCond(&sc.mu)
	return sc
}

// NewWithState creates ScanControl with initial state (for recovery)
func NewWithState(scanID uint, state State) *ScanControl {
	sc := New(scanID)
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.state = state
	if state == StateCancelled {
		sc.cancel()
	}
	return sc
}

// ScanID returns the scan ID this control is managing
func (sc *ScanControl) ScanID() uint {
	return sc.scanID
}

// Context returns the context that is cancelled when the scan is cancelled.
// Use this context for HTTP requests and other cancellable operations.
func (sc *ScanControl) Context() context.Context {
	return sc.ctx
}

// State returns current state
func (sc *ScanControl) State() State {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.state
}

// SetPaused transitions to paused state
func (sc *ScanControl) SetPaused() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.state == StateCancelled {
		return // Can't pause a cancelled scan
	}

	sc.state = StatePaused
}

// SetRunning transitions to running state (unblocks paused workers)
func (sc *ScanControl) SetRunning() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.state == StateCancelled {
		return // Can't resume a cancelled scan
	}

	waspaused := sc.state == StatePaused
	sc.state = StateRunning

	// Wake up all waiting goroutines
	if waspaused {
		sc.pauseCond.Broadcast()
	}
}

// SetCancelled transitions to cancelled state
func (sc *ScanControl) SetCancelled() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.state == StateCancelled {
		return // Already cancelled
	}

	sc.state = StateCancelled

	// Cancel the context to stop in-flight operations
	sc.cancel()

	// Wake up any waiting goroutines so they can exit
	sc.pauseCond.Broadcast()
}

// Checkpoint blocks if paused, returns false if cancelled.
// Workers call this at strategic points during job execution.
// Returns true if work should continue, false if work should stop.
//
// Example usage:
//
//	for _, payload := range payloads {
//	    if !ctrl.Checkpoint() {
//	        return // Scan was cancelled
//	    }
//	    executePayload(payload)
//	}
func (sc *ScanControl) Checkpoint() bool {
	sc.mu.RLock()
	state := sc.state
	sc.mu.RUnlock()

	// Fast path: if running, continue immediately
	if state == StateRunning {
		return true
	}

	// If cancelled, stop immediately
	if state == StateCancelled {
		return false
	}

	// If paused, block until resumed or cancelled
	sc.mu.Lock()
	for sc.state == StatePaused {
		sc.pauseCond.Wait()
	}
	state = sc.state
	sc.mu.Unlock()

	return state != StateCancelled
}

// CheckpointWithContext is like Checkpoint but also respects a passed context.
// This is useful when the job itself has a timeout or external cancellation.
func (sc *ScanControl) CheckpointWithContext(ctx context.Context) bool {
	// Check the external context first
	select {
	case <-ctx.Done():
		return false
	default:
	}

	sc.mu.RLock()
	state := sc.state
	sc.mu.RUnlock()

	// Fast path: if running, continue immediately
	if state == StateRunning {
		return true
	}

	// If cancelled, stop immediately
	if state == StateCancelled {
		return false
	}

	// If paused, need to wait but also watch the external context
	sc.mu.Lock()
	defer sc.mu.Unlock()

	for sc.state == StatePaused {
		// Use a goroutine to watch for context cancellation
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				sc.pauseCond.Broadcast()
			case <-done:
			}
		}()

		sc.pauseCond.Wait()
		close(done)

		// Check if we woke up due to context cancellation
		select {
		case <-ctx.Done():
			return false
		default:
		}
	}

	return sc.state != StateCancelled
}

// IsCancelled returns true if the scan has been cancelled
func (sc *ScanControl) IsCancelled() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.state == StateCancelled
}

// IsPaused returns true if the scan is paused
func (sc *ScanControl) IsPaused() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.state == StatePaused
}

// IsRunning returns true if the scan is running
func (sc *ScanControl) IsRunning() bool {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.state == StateRunning
}

// WaitIfPaused blocks until the scan is no longer paused.
// Returns true if the scan is running, false if cancelled.
// This is an alias for Checkpoint() for better readability in some contexts.
func (sc *ScanControl) WaitIfPaused() bool {
	return sc.Checkpoint()
}
