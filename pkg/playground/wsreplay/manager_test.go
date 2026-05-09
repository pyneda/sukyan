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
