package fuzz

import (
	"context"
	"testing"
	"time"
)

func TestRegistry_RegisterCreatesGate(t *testing.T) {
	reg := NewRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Register(42, cancel, nil)
	if !reg.Has(42) {
		t.Fatal("Has should be true after Register")
	}
	if g := reg.Gate(42); g == nil {
		t.Fatal("Gate should be non-nil after Register")
	}
}

func TestRegistry_UnregisterClearsState(t *testing.T) {
	reg := NewRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Register(42, cancel, nil)
	reg.Unregister(42)
	if reg.Has(42) {
		t.Fatal("Has should be false after Unregister")
	}
	if g := reg.Gate(42); g != nil {
		t.Fatal("Gate should be nil after Unregister")
	}
}

func TestRegistry_PauseResumeRoundtrip(t *testing.T) {
	reg := NewRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Register(7, cancel, nil)

	if !reg.Pause(7) {
		t.Fatal("first Pause should return true")
	}
	if reg.Pause(7) {
		t.Fatal("second Pause should return false (already paused)")
	}
	if !reg.Gate(7).IsPaused() {
		t.Fatal("gate should report paused")
	}
	if !reg.Resume(7) {
		t.Fatal("Resume from paused should return true")
	}
	if reg.Resume(7) {
		t.Fatal("Resume when not paused should return false")
	}
}

func TestRegistry_PauseResumeUnknownRun(t *testing.T) {
	reg := NewRegistry()
	if reg.Pause(999) {
		t.Fatal("Pause on unknown run should return false")
	}
	if reg.Resume(999) {
		t.Fatal("Resume on unknown run should return false")
	}
	if reg.Has(999) {
		t.Fatal("Has on unknown run should return false")
	}
}

func TestRegistry_UnregisterUnblocksWaiters(t *testing.T) {
	reg := NewRegistry()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg.Register(1, cancel, nil)
	reg.Pause(1)

	gate := reg.Gate(1)
	done := make(chan struct{})
	go func() {
		_, _ = gate.Wait(context.Background())
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	reg.Unregister(1)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Unregister should unblock waiters via gate Resume")
	}
}

func TestRegistry_CancelResumesGate(t *testing.T) {
	reg := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	reg.Register(5, cancel, nil)
	reg.Pause(5)

	gate := reg.Gate(5)
	done := make(chan struct{})
	go func() {
		_, _ = gate.Wait(context.Background())
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	if !reg.Cancel(5) {
		t.Fatal("Cancel should return true")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Cancel should resume gate so workers unblock")
	}
	if ctx.Err() == nil {
		t.Fatal("Cancel should have cancelled the registered context")
	}
}
