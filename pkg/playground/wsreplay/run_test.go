package wsreplay

import (
	"context"
	"testing"
	"time"
)

func TestRunSucceeds_AllStepsMatch(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := NewBroadcaster(256, 1000)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: RunInstance(1),
		Persister: persist, Events: b,
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	script := []ScriptEntry{
		{ID: "a", Content: "hello", Opcode: 1, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "hello", TimeoutMs: 1000}},
		{ID: "b", Content: "world", Opcode: 1, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchAny, TimeoutMs: 1000}},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	if res.Status != "succeeded" {
		t.Fatalf("expected succeeded got %s (%s)", res.Status, res.FailureReason)
	}
}

func TestRunFailsOnAbortTimeout(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := NewBroadcaster(256, 1000)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: RunInstance(2),
		Persister: persist, Events: b,
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	script := []ScriptEntry{
		{ID: "a", Content: "ping", Opcode: 1, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "WILL_NEVER_MATCH", TimeoutMs: 200}},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	if res.Status != "failed" {
		t.Fatalf("expected failed got %s", res.Status)
	}
}

func TestRunContinuesOnContinueTimeout(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := NewBroadcaster(256, 1000)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: RunInstance(3),
		Persister: persist, Events: b,
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	script := []ScriptEntry{
		{ID: "a", Content: "ping", Opcode: 1, OnTimeout: PolicyContinue, OnNoMatch: PolicyContinue,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "WILL_NEVER_MATCH", TimeoutMs: 100}},
		{ID: "b", Content: "second", Opcode: 1, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "second", TimeoutMs: 1000}},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	if res.Status != "succeeded" {
		t.Fatalf("expected succeeded got %s (%s)", res.Status, res.FailureReason)
	}
}

func TestRunCancelInterrupts(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := NewBroadcaster(256, 1000)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: RunInstance(4),
		Persister: persist, Events: b,
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	script := []ScriptEntry{
		{ID: "a", Content: "x", Opcode: 1, DelayMs: 5000, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	res := WalkScript(ctx, sess, script, SessionOptions{}, b)
	if res.Status != "cancelled" {
		t.Fatalf("expected cancelled got %s", res.Status)
	}
}

func TestRunEmitsExpectedEventSequence(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := NewBroadcaster(256, 1000)
	// Subscribe BEFORE dial so we capture the session state_changed events too.
	subCh, _ := b.Subscribe(0)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: RunInstance(42),
		Persister: persist, Events: b,
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()

	script := []ScriptEntry{
		{ID: "a", Content: "hello", Opcode: 1, OnTimeout: PolicyAbort, OnNoMatch: PolicyAbort,
			WaitFor: &WaitForSpec{MatchType: MatchContains, Pattern: "hello", TimeoutMs: 1000}},
	}
	res := WalkScript(context.Background(), sess, script, SessionOptions{}, b)
	if res.Status != "succeeded" {
		t.Fatalf("expected succeeded got %s (%s)", res.Status, res.FailureReason)
	}

	// Drain a window's worth of events and verify the run-emitted ones occur in order.
	wantOrder := []string{"run_step_started", "wait_started", "wait_matched", "run_step_completed"}
	seen := []string{}
	deadline := time.After(2 * time.Second)
	for len(seen) < len(wantOrder) {
		select {
		case ev := <-subCh:
			for _, want := range wantOrder {
				if ev.Type == want {
					seen = append(seen, ev.Type)
					break
				}
			}
		case <-deadline:
			t.Fatalf("timed out waiting for events, saw: %v", seen)
		}
	}
	for i, want := range wantOrder {
		if seen[i] != want {
			t.Fatalf("event order mismatch at %d: want %s, got %s (full: %v)", i, want, seen[i], seen)
		}
	}
}
