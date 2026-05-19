package wsfuzz

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/stream"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

// TestEngine_TLSInsecureSkipVerify dials a self-signed wss:// server. Without
// InsecureSkipVerify the dial must fail; with it the dial must succeed.
func TestEngine_TLSInsecureSkipVerify(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		conn.WriteMessage(websocket.TextMessage, append([]byte("ok: "), msg...))
	}))
	defer srv.Close()
	wssURL := "wss" + strings.TrimPrefix(srv.URL, "https") + "/"

	cfg := WsFuzzerConfig{
		TargetURL: wssURL,
		Mode:      fuzz.ModeSingle,
		TLSConfig: TLSConfig{InsecureSkipVerify: true},
		Script: []WsFuzzStep{{
			Role:      RoleFuzz,
			Opcode:    1,
			Content:   "x",
			Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1, OriginalValue: "x"}},
			WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchContains, Pattern: "ok:", TimeoutMs: 2000},
		}},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: []string{"a"}},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 1, RequestTimeoutSeconds: 5},
	}

	// Custom dial that forwards the TLSConfig through.
	dial := func(ctx context.Context, c wsreplay.SessionConfig) (SessionHandle, error) {
		c.Persister = noopPersister{}
		c.TLSConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		s, err := wsreplay.DialSession(ctx, c)
		if err != nil {
			return nil, err
		}
		return WrapSession(s), nil
	}

	persister := &fakeRunPersister{}
	require.NoError(t, Run(context.Background(), 202, cfg, EngineDeps{
		Persister:   persister,
		Broadcaster: stream.NewBroadcaster(64, 1000),
		Dial:        dial,
	}))
	require.Equal(t, StatusCompleted, persister.iterations()[0].Status)
}
