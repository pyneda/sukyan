package fuzz

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPauseGate_NotPaused_WaitReturnsImmediately(t *testing.T) {
	g := NewPauseGate()
	start := time.Now()
	if err := g.Wait(context.Background()); err != nil {
		t.Fatalf("Wait should not error when not paused: %v", err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("Wait blocked %v when not paused (expected immediate)", d)
	}
	if g.IsPaused() {
		t.Fatal("IsPaused should be false for a fresh gate")
	}
}

func TestPauseGate_PauseThenWaitBlocksUntilResume(t *testing.T) {
	g := NewPauseGate()
	if !g.Pause() {
		t.Fatal("Pause on not-paused gate should return true")
	}
	if !g.IsPaused() {
		t.Fatal("IsPaused should be true after Pause")
	}

	var waitErr atomic.Value // error
	var unblocked atomic.Bool
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := g.Wait(context.Background())
		if err != nil {
			waitErr.Store(err)
		}
		unblocked.Store(true)
	}()

	// Verify Wait is actually blocked.
	time.Sleep(50 * time.Millisecond)
	if unblocked.Load() {
		t.Fatal("Wait returned before Resume was called")
	}

	if !g.Resume() {
		t.Fatal("Resume on paused gate should return true")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Wait did not unblock within 1s of Resume")
	}
	if v := waitErr.Load(); v != nil {
		t.Fatalf("Wait returned unexpected error: %v", v)
	}
	if g.IsPaused() {
		t.Fatal("IsPaused should be false after Resume")
	}
}

func TestPauseGate_WaitReturnsCtxErrOnCancel(t *testing.T) {
	g := NewPauseGate()
	g.Pause()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- g.Wait(ctx) }()

	time.Sleep(25 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after ctx cancel")
	}
}

func TestPauseGate_IdempotentPauseResume(t *testing.T) {
	g := NewPauseGate()
	if !g.Pause() {
		t.Fatal("first Pause should report state change")
	}
	if g.Pause() {
		t.Fatal("second Pause should be a no-op (false)")
	}
	if !g.Resume() {
		t.Fatal("first Resume should report state change")
	}
	if g.Resume() {
		t.Fatal("second Resume should be a no-op (false)")
	}
}

func TestPauseGate_ManyWaitersUnblockedByOneResume(t *testing.T) {
	g := NewPauseGate()
	g.Pause()

	const n = 20
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_ = g.Wait(context.Background())
		}()
	}

	time.Sleep(50 * time.Millisecond)
	g.Resume()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("not all waiters unblocked within 1s")
	}
}

func TestPauseGate_PauseAfterResumeReusable(t *testing.T) {
	g := NewPauseGate()
	g.Pause()
	g.Resume()

	if g.IsPaused() {
		t.Fatal("gate should be not-paused after Resume")
	}
	if !g.Pause() {
		t.Fatal("Pause after Resume should succeed")
	}

	errCh := make(chan error, 1)
	go func() { errCh <- g.Wait(context.Background()) }()
	time.Sleep(30 * time.Millisecond)
	g.Resume()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second pause/resume cycle did not unblock waiter")
	}
}
