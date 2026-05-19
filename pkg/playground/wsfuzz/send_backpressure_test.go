package wsfuzz

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

// TestEngine_SlowServer_SendBackpressure verifies that a server which never
// reads from its socket causes the iteration's Send to time out after
// SendTimeout, and the iteration is marked StatusConnectionError. The
// closeWithTimeout safety net ensures no goroutine leak even if the upstream
// is wedged.
func TestEngine_SlowServer_SendBackpressure(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()
		// Hold the connection open without reading.
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	// 10 MB payload — way bigger than any reasonable kernel/library write buffer,
	// guaranteeing that Send blocks waiting for the never-reading peer.
	bigContent := strings.Repeat("A", 10*1024*1024)

	cfg := WsFuzzerConfig{
		TargetURL: wsURL,
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{{
			Role:      RoleFuzz,
			Opcode:    1,
			Content:   bigContent,
			Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1}},
			WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchAny, TimeoutMs: 1000},
		}},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 1, RequestTimeoutSeconds: 5},
	}

	dial := func(ctx context.Context, c wsreplay.SessionConfig) (SessionHandle, error) {
		c.Persister = noopPersister{}
		c.SendTimeout = 500 * time.Millisecond
		s, err := wsreplay.DialSession(ctx, c)
		if err != nil {
			return nil, err
		}
		return WrapSession(s), nil
	}

	persister := &fakeRunPersister{}
	require.NoError(t, Run(context.Background(), 203, cfg, EngineDeps{
		Persister:   persister,
		Broadcaster: stream.NewBroadcaster(64, 1000),
		Dial:        dial,
	}))
	require.Equal(t, 1, persister.iterationCount())
	it := persister.iterations()[0]
	require.Equal(t, StatusConnectionError, it.Status, "slow server must surface as StatusConnectionError; got %s (%s)", it.Status, it.FailureReason)
}
