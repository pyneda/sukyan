package control

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestScanControl_InitialState(t *testing.T) {
	ctrl := New(1)
	if ctrl.State() != StateRunning {
		t.Errorf("Expected initial state to be StateRunning, got %v", ctrl.State())
	}
}

func TestScanControl_NewWithState(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		expected State
	}{
		{"Running", StateRunning, StateRunning},
		{"Paused", StatePaused, StatePaused},
		{"Cancelled", StateCancelled, StateCancelled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := NewWithState(1, tt.state)
			if ctrl.State() != tt.expected {
				t.Errorf("Expected state %v, got %v", tt.expected, ctrl.State())
			}
		})
	}
}

func TestScanControl_PauseResume(t *testing.T) {
	ctrl := New(1)

	// Pause
	ctrl.SetPaused()
	if !ctrl.IsPaused() {
		t.Error("Expected scan to be paused")
	}

	// Resume
	ctrl.SetRunning()
	if !ctrl.IsRunning() {
		t.Error("Expected scan to be running")
	}
}

func TestScanControl_Cancel(t *testing.T) {
	ctrl := New(1)

	ctrl.SetCancelled()
	if !ctrl.IsCancelled() {
		t.Error("Expected scan to be cancelled")
	}

	// Context should be cancelled
	select {
	case <-ctrl.Context().Done():
		// Expected
	default:
		t.Error("Expected context to be cancelled")
	}
}

func TestScanControl_CannotPauseCancelled(t *testing.T) {
	ctrl := New(1)
	ctrl.SetCancelled()
	ctrl.SetPaused()

	if ctrl.IsPaused() {
		t.Error("Should not be able to pause a cancelled scan")
	}
}

func TestScanControl_CannotResumeCancelled(t *testing.T) {
	ctrl := New(1)
	ctrl.SetCancelled()
	ctrl.SetRunning()

	if ctrl.IsRunning() {
		t.Error("Should not be able to resume a cancelled scan")
	}
}

func TestScanControl_Checkpoint_Running(t *testing.T) {
	ctrl := New(1)
	if !ctrl.Checkpoint() {
		t.Error("Checkpoint should return true when running")
	}
}

func TestScanControl_Checkpoint_Cancelled(t *testing.T) {
	ctrl := New(1)
	ctrl.SetCancelled()
	if ctrl.Checkpoint() {
		t.Error("Checkpoint should return false when cancelled")
	}
}

func TestScanControl_Checkpoint_PausedThenResumed(t *testing.T) {
	ctrl := New(1)
	ctrl.SetPaused()

	var result bool
	done := make(chan struct{})

	// Start a goroutine that will block on checkpoint
	go func() {
		result = ctrl.Checkpoint()
		close(done)
	}()

	// Give the goroutine time to block
	time.Sleep(50 * time.Millisecond)

	// Resume
	ctrl.SetRunning()

	// Wait for the checkpoint to complete
	select {
	case <-done:
		if !result {
			t.Error("Checkpoint should return true after resume")
		}
	case <-time.After(time.Second):
		t.Error("Checkpoint did not unblock after resume")
	}
}

func TestScanControl_Checkpoint_PausedThenCancelled(t *testing.T) {
	ctrl := New(1)
	ctrl.SetPaused()

	var result bool
	done := make(chan struct{})

	// Start a goroutine that will block on checkpoint
	go func() {
		result = ctrl.Checkpoint()
		close(done)
	}()

	// Give the goroutine time to block
	time.Sleep(50 * time.Millisecond)

	// Cancel
	ctrl.SetCancelled()

	// Wait for the checkpoint to complete
	select {
	case <-done:
		if result {
			t.Error("Checkpoint should return false after cancel")
		}
	case <-time.After(time.Second):
		t.Error("Checkpoint did not unblock after cancel")
	}
}

func TestScanControl_ConcurrentCheckpoints(t *testing.T) {
	ctrl := New(1)
	ctrl.SetPaused()

	numWorkers := 10
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	results := make([]bool, numWorkers)

	// Start multiple workers that will block on checkpoint
	for i := 0; i < numWorkers; i++ {
		go func(idx int) {
			defer wg.Done()
			results[idx] = ctrl.Checkpoint()
		}(i)
	}

	// Give workers time to block
	time.Sleep(50 * time.Millisecond)

	// Resume
	ctrl.SetRunning()

	// Wait for all workers
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All should have returned true
		for i, r := range results {
			if !r {
				t.Errorf("Worker %d returned false, expected true", i)
			}
		}
	case <-time.After(2 * time.Second):
		t.Error("Not all workers completed in time")
	}
}

func TestScanControl_CheckpointWithContext_ContextCancelled(t *testing.T) {
	ctrl := New(1)
	ctrl.SetPaused()

	ctx, cancel := context.WithCancel(context.Background())

	var result bool
	done := make(chan struct{})

	go func() {
		result = ctrl.CheckpointWithContext(ctx)
		close(done)
	}()

	// Give the goroutine time to block
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	select {
	case <-done:
		if result {
			t.Error("CheckpointWithContext should return false when context is cancelled")
		}
	case <-time.After(time.Second):
		t.Error("CheckpointWithContext did not unblock after context cancel")
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateRunning, "running"},
		{StatePaused, "paused"},
		{StateCancelled, "cancelled"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("State(%d).String() = %s, want %s", tt.state, got, tt.expected)
		}
	}
}
