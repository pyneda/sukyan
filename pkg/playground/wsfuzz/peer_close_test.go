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

// TestEngine_PeerClose1008 verifies that a server-initiated close frame (with
// a specific close code) is distinguished from a network error and surfaced as
// StatusPeerClosed on the iteration result.
func TestEngine_PeerClose1008(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _ := upgrader.Upgrade(w, r, nil)
		defer conn.Close()
		_, _, _ = conn.ReadMessage()
		conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(1008, "policy violation"),
			time.Now().Add(time.Second),
		)
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"

	cfg := WsFuzzerConfig{
		TargetURL: wsURL, Mode: fuzz.ModeSingle,
		Script: []WsFuzzStep{{
			Role: RoleFuzz, Opcode: 1,
			Content:   "fuzz",
			Positions: []fuzz.FuzzerPosition{{Start: 0, End: 4, OriginalValue: "fuzz"}},
			WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchAny, TimeoutMs: 2000},
		}},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: []string{"x"}},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 1, RequestTimeoutSeconds: 5},
	}
	persister := &fakeRunPersister{}
	bcast := stream.NewBroadcaster(64, 1000)
	require.NoError(t, Run(context.Background(), 200, cfg, EngineDeps{
		Persister: persister, Broadcaster: bcast, Dial: engineDial,
	}))
	require.Equal(t, 1, persister.iterationCount())
	it := persister.iterations()[0]
	require.Equal(t, StatusPeerClosed, it.Status, "peer close must surface as StatusPeerClosed; got %s (%s)", it.Status, it.FailureReason)
}
