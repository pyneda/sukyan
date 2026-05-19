package wsfuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

// fakeRunPersister is a shared test double; lives here so other engine tests
// can reuse it.
type fakeRunPersister struct {
	mu  sync.Mutex
	its []WsIterationResult
}

func (f *fakeRunPersister) UpdateRunStatus(uint, string, string) error { return nil }
func (f *fakeRunPersister) UpdateRunProgress(uint, int, int, int) error { return nil }
func (f *fakeRunPersister) UpdateRunStartedAt(uint, time.Time) error    { return nil }
func (f *fakeRunPersister) UpdateRunFinishedAt(uint, time.Time) error   { return nil }
func (f *fakeRunPersister) UpdateRunBaseline(uint, []byte) error        { return nil }
func (f *fakeRunPersister) SaveIteration(it WsIterationResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.its = append(f.its, it)
	return nil
}
func (f *fakeRunPersister) iterationCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.its)
}
func (f *fakeRunPersister) iterations() []WsIterationResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]WsIterationResult(nil), f.its...)
}

// noopPersister implements wsreplay.Persister with no IO.
type noopPersister struct{}

func (noopPersister) CreateConnection(url string, headers []wsreplay.HeaderSpec, statusCode int, source string, sessID *uint) (uint, error) {
	return 1, nil
}
func (noopPersister) RecordMessage(connID uint, opcode int, content string, direction string) (uint, error) {
	return 1, nil
}
func (noopPersister) CloseConnection(connID uint) error { return nil }

// engineDial is the standard dial helper that wraps a real wsreplay.Session
// with the no-op persister so engine tests don't need a real DB.
func engineDial(ctx context.Context, c wsreplay.SessionConfig) (SessionHandle, error) {
	c.Persister = noopPersister{}
	s, err := wsreplay.DialSession(ctx, c)
	if err != nil {
		return nil, err
	}
	return WrapSession(s), nil
}

func startEchoWS(t *testing.T) (url string, srv *httptest.Server) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, append([]byte("echo: "), msg...)); err != nil {
				return
			}
		}
	}))
	url = "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	return
}

func TestEngine_Run_SinglePayloadEndToEnd(t *testing.T) {
	wsURL, srv := startEchoWS(t)
	defer srv.Close()

	cfg := WsFuzzerConfig{
		TargetURL: wsURL,
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{
			{
				Role:      RoleFuzz,
				Opcode:    1,
				Content:   `{"payload":"X"}`,
				Positions: []fuzz.FuzzerPosition{{Start: 12, End: 13, OriginalValue: "X"}},
				WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchContains, Pattern: "echo:", TimeoutMs: 2000},
			},
		},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: []string{"a", "b", "c"}},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 2, RequestTimeoutSeconds: 5},
	}

	persister := &fakeRunPersister{}
	bcast := stream.NewBroadcaster(64, 1000)
	err := Run(context.Background(), 99, cfg, EngineDeps{
		Persister:   persister,
		Broadcaster: bcast,
		Dial:        engineDial,
	})
	require.NoError(t, err)
	require.Equal(t, 3, persister.iterationCount(), "expected one iteration per payload")
	for _, it := range persister.iterations() {
		require.Equal(t, StatusCompleted, it.Status, "iteration %d should complete; got %s (%s)", it.IterationIndex, it.Status, it.FailureReason)
	}
}
