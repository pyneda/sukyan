package wsreplay

import (
	"context"
	"testing"
	"time"
)

func TestManagerOpenAndClose(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	m := NewManager(persist)
	sess, err := m.OpenInteractive(context.Background(), 42, SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
		Persister: persist, Events: m.BroadcasterFor(42),
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.GetInteractive(42); got != sess {
		t.Fatal("manager should return same session")
	}
	m.CloseInteractive(42)
	if got := m.GetInteractive(42); got != nil {
		t.Fatal("expected nil after close")
	}
}

func TestManagerBroadcasterIsShared(t *testing.T) {
	persist := newFakePersister()
	m := NewManager(persist)
	b1 := m.BroadcasterFor(7)
	b2 := m.BroadcasterFor(7)
	if b1 != b2 {
		t.Fatal("expected the same broadcaster for the same session")
	}
}

func TestManagerOpenInteractiveIsRaceSafe(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	m := NewManager(persist)
	const sessionID = uint(99)

	// 8 concurrent OpenInteractive calls. Only one Session should win the
	// registry; all others must either return the same session or close themselves.
	const goroutines = 8
	results := make(chan *Session, goroutines)
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			sess, err := m.OpenInteractive(context.Background(), sessionID, SessionConfig{
				TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
				Persister: persist, Events: m.BroadcasterFor(sessionID),
				ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
			})
			if err != nil {
				errs <- err
				results <- nil
				return
			}
			errs <- nil
			results <- sess
		}()
	}

	var winners []*Session
	for i := 0; i < goroutines; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
		s := <-results
		winners = append(winners, s)
	}

	registered := m.GetInteractive(sessionID)
	if registered == nil {
		t.Fatal("expected one session registered")
	}
	// Every returned session must be the registered winner; otherwise the loser
	// leaked instead of being closed.
	for i, s := range winners {
		if s != registered {
			t.Fatalf("goroutine %d returned a non-winner session %p (winner is %p)", i, s, registered)
		}
	}
	m.CloseInteractive(sessionID)
}

func TestManagerCloseInteractiveDoesNotAffectRuns(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	m := NewManager(persist)
	const sessionID = uint(101)

	// Open interactive.
	_, err := m.OpenInteractive(context.Background(), sessionID, SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
		Persister: persist, Events: m.BroadcasterFor(sessionID),
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Spin up a "run" session manually (Task 15 will do this for real).
	runSess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: RunInstance(7),
		Persister: persist, Events: m.BroadcasterFor(sessionID),
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer runSess.Close()
	m.RegisterRun(sessionID, 7, runSess)

	// Close interactive.
	m.CloseInteractive(sessionID)

	// Run should still be registered.
	if got := m.GetRun(sessionID, 7); got != runSess {
		t.Fatalf("run session lost after CloseInteractive (got %p, want %p)", got, runSess)
	}
	m.UnregisterRun(sessionID, 7)
	if got := m.GetRun(sessionID, 7); got != nil {
		t.Fatal("expected nil run after Unregister")
	}
}

func TestManagerAutoClosesIdleInteractive(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	m := NewManager(persist)
	m.SetIdleTimeout(200 * time.Millisecond)
	cfg := SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
		Persister: persist, Events: m.BroadcasterFor(99),
		ConnectTimeout: 2 * time.Second, SendTimeout: time.Second,
	}
	sess, err := m.OpenInteractive(context.Background(), 99, cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = sess
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.GetInteractive(99) == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("expected interactive session to auto-close")
}

func TestManagerCloseBroadcaster(t *testing.T) {
	persist := newFakePersister()
	m := NewManager(persist)
	const sessionID = uint(202)

	b := m.BroadcasterFor(sessionID)
	subCh, _ := b.Subscribe(0)
	m.CloseBroadcaster(sessionID)

	select {
	case _, ok := <-subCh:
		if ok {
			t.Fatal("expected channel closed after CloseBroadcaster")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber channel did not close in time")
	}

	// Subsequent BroadcasterFor returns a fresh broadcaster (different pointer).
	b2 := m.BroadcasterFor(sessionID)
	if b2 == b {
		t.Fatal("expected a fresh broadcaster after CloseBroadcaster")
	}
}
