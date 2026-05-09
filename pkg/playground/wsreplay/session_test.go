package wsreplay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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
	messages          []PersistedMessage
}

func newFakePersister() *fakePersister { return &fakePersister{} }

func (f *fakePersister) CreateConnection(url string, headers []HeaderSpec, statusCode int, source string, playgroundSessionID *uint) (uint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectionCreated++
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
	b := NewBroadcaster(64, 1000)
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
