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

// TestEngine_ServerPings_AutoPong verifies that gorilla's default PING handler
// auto-PONGs and that PING control frames do NOT appear as data frames on the
// engine's NextFrame channel (they're not delivered to the script's wait_for).
func TestEngine_ServerPings_AutoPong(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()
		conn.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		conn.WriteMessage(websocket.TextMessage, append([]byte("echo: "), msg...))
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	cfg := WsFuzzerConfig{
		TargetURL: wsURL, Mode: fuzz.ModeSingle,
		Script: []WsFuzzStep{{
			Role:      RoleFuzz,
			Opcode:    1,
			Content:   "x",
			Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1, OriginalValue: "x"}},
			WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchContains, Pattern: "echo:", TimeoutMs: 2000},
		}},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: []string{"a"}},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 1, RequestTimeoutSeconds: 5},
	}
	persister := &fakeRunPersister{}
	require.NoError(t, Run(context.Background(), 201, cfg, EngineDeps{
		Persister:   persister,
		Broadcaster: stream.NewBroadcaster(64, 1000),
		Dial:        engineDial,
	}))
	require.Equal(t, StatusCompleted, persister.iterations()[0].Status, "PING control frame must not interfere with wait_for")
}
