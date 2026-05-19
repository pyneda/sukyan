package wsreplay

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/stretchr/testify/require"
)

// startEchoServer starts an in-process WS echo server. Used by session and run tests.
func startEchoServer(t *testing.T) *httptest.Server {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				return
			}
			if err := c.WriteMessage(mt, msg); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func wsURL(httpURL string) string { return "ws" + strings.TrimPrefix(httpURL, "http") }

// fakePersister is a no-IO Persister used by engine tests.
// readLoop and writeLoop call RecordMessage concurrently, so the fixture
// is mutex-guarded; the race detector caught this on the original draft.
type fakePersister struct {
	mu                sync.Mutex
	connectionCreated uint
	lastSource        string
	messages          []PersistedMessage
}

func newFakePersister() *fakePersister { return &fakePersister{} }

func (f *fakePersister) CreateConnection(url string, headers []HeaderSpec, statusCode int, source string, playgroundSessionID *uint) (uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectionCreated++
	f.lastSource = source
	return f.connectionCreated, nil
}

func (f *fakePersister) RecordMessage(connID uint, opcode int, content string, direction string) (uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uint(len(f.messages) + 1)
	f.messages = append(f.messages, PersistedMessage{ID: id, ConnectionID: connID, Opcode: opcode, Content: content, Direction: direction})
	return id, nil
}

func (f *fakePersister) CloseConnection(connID uint) error { return nil }

func TestSessionConnectAndEcho(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := stream.NewBroadcaster(64, 1000)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL:      wsURL(echo.URL),
		Headers:        nil,
		Instance:       InteractiveInstance(),
		Persister:      persist,
		Events:         b,
		ConnectTimeout: 5 * time.Second,
		SendTimeout:    1 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.Send(1, "hello"); err != nil {
		t.Fatal(err)
	}
	frame, err := sess.NextFrame(2 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if frame.Content != "hello" || frame.Direction != "received" {
		t.Fatalf("unexpected frame: %+v", frame)
	}
	if persist.connectionCreated == 0 {
		t.Fatal("expected persister to record a connection")
	}
}

func TestSessionCloseTransitionsAndJoins(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := stream.NewBroadcaster(64, 1000)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
		Persister: persist, Events: b,
		ConnectTimeout: time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess.Close()
	if got := sess.State(); got != StateDisconnected {
		t.Fatalf("expected StateDisconnected after Close, got %s", got)
	}
	// Idempotent.
	sess.Close()
	// NextFrame after Close returns the closed error promptly.
	start := time.Now()
	if _, err := sess.NextFrame(2 * time.Second); err == nil {
		t.Fatal("expected error from NextFrame after close")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("NextFrame took too long after close: %s", elapsed)
	}
}

func TestSessionSendAfterCloseFails(t *testing.T) {
	echo := startEchoServer(t)
	persist := newFakePersister()
	b := stream.NewBroadcaster(64, 1000)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
		Persister: persist, Events: b,
		ConnectTimeout: time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess.Close()
	if err := sess.Send(1, "hi"); err == nil {
		t.Fatal("expected Send to fail after Close")
	}
}

func TestSessionDialPersisterFails(t *testing.T) {
	echo := startEchoServer(t)
	persist := &failingPersister{err: errors.New("db down")}
	b := stream.NewBroadcaster(64, 1000)
	_, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
		Persister: persist, Events: b,
		ConnectTimeout: time.Second, SendTimeout: time.Second,
	})
	if err == nil {
		t.Fatal("expected DialSession to fail when persister fails")
	}
}

// failingPersister always errors on CreateConnection.
type failingPersister struct{ err error }

func (f *failingPersister) CreateConnection(string, []HeaderSpec, int, string, *uint) (uint, error) {
	return 0, f.err
}
func (f *failingPersister) RecordMessage(uint, int, string, string) (uint, error) { return 0, nil }
func (f *failingPersister) CloseConnection(uint) error                            { return nil }

func TestSessionPersistErrorEmitsEvent(t *testing.T) {
	echo := startEchoServer(t)
	// Persister that errors on RecordMessage.
	persist := &flakyPersister{recordErr: errors.New("disk full")}
	b := stream.NewBroadcaster(64, 1000)
	subCh, _ := b.Subscribe(0)
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL), Instance: InteractiveInstance(),
		Persister: persist, Events: b,
		ConnectTimeout: time.Second, SendTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if err := sess.Send(1, "x"); err != nil {
		t.Fatal(err)
	}
	// We expect to see at least one persist_error event within a short window.
	deadline := time.After(2 * time.Second)
	saw := false
	for !saw {
		select {
		case raw := <-subCh:
			ev := raw.(*Event)
			if ev.Type == "persist_error" {
				saw = true
			}
		case <-deadline:
			t.Fatal("did not receive persist_error event")
		}
	}
}

// flakyPersister returns a fixed error on RecordMessage. CreateConnection succeeds.
type flakyPersister struct {
	mu        sync.Mutex
	recordErr error
	connID    uint
}

func (f *flakyPersister) CreateConnection(string, []HeaderSpec, int, string, *uint) (uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connID++
	return f.connID, nil
}
func (f *flakyPersister) RecordMessage(uint, int, string, string) (uint, error) {
	return 0, f.recordErr
}
func (f *flakyPersister) CloseConnection(uint) error { return nil }

func TestDialSession_SourceFieldHonored(t *testing.T) {
	echo := startEchoServer(t)
	p := newFakePersister()
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL),
		Instance:  InteractiveInstance(),
		Persister: p,
		Source:    "ws_fuzz",
	})
	require.NoError(t, err)
	defer sess.Close()
	require.Equal(t, "ws_fuzz", p.lastSource, "DialSession must pass cfg.Source to the persister")
}

func TestDialSession_SourceDefaultsToPlayground(t *testing.T) {
	echo := startEchoServer(t)
	p := newFakePersister()
	sess, err := DialSession(context.Background(), SessionConfig{
		TargetURL: wsURL(echo.URL),
		Instance:  InteractiveInstance(),
		Persister: p,
		// Source omitted intentionally
	})
	require.NoError(t, err)
	defer sess.Close()
	require.Equal(t, "playground", p.lastSource, "DialSession must default Source to playground when unset")
}
