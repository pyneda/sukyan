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

// startSlowEchoWS is a variant of startEchoWS that delays each response by `d`,
// giving the test time to pause mid-run.
func startSlowEchoWS(t *testing.T, d time.Duration) (string, *httptest.Server) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		require.NoError(t, err)
		defer conn.Close()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			time.Sleep(d)
			if err := conn.WriteMessage(websocket.TextMessage, append([]byte("echo: "), msg...)); err != nil {
				return
			}
		}
	}))
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/"
	return url, srv
}

func TestEngine_PauseResume(t *testing.T) {
	wsURL, srv := startSlowEchoWS(t, 300*time.Millisecond)
	defer srv.Close()

	// 20 iterations × 300ms = 6s sequential; concurrency 2 → ~3s worst case.
	payloads := make([]string, 20)
	for i := range payloads {
		payloads[i] = "p"
	}

	cfg := WsFuzzerConfig{
		TargetURL: wsURL,
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{{
			Role: RoleFuzz, Opcode: 1,
			Content:   `{"p":"X"}`,
			Positions: []fuzz.FuzzerPosition{{Start: 6, End: 7, OriginalValue: "X"}},
			WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchContains, Pattern: "echo:", TimeoutMs: 2000},
		}},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: payloads},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 2, RequestTimeoutSeconds: 5},
	}

	persister := &fakeRunPersister{}
	bcast := stream.NewBroadcaster(64, 1000)
	runID := uint(100)

	done := make(chan struct{})
	go func() {
		_ = Run(context.Background(), runID, cfg, EngineDeps{Persister: persister, Broadcaster: bcast, Dial: engineDial})
		close(done)
	}()

	// Wait until at least 2 iterations are visible (we know the engine is making progress).
	require.Eventually(t, func() bool {
		return persister.iterationCount() >= 2
	}, 5*time.Second, 50*time.Millisecond, "engine should produce iterations before pause")

	// Pause — gate flips; the assignment loop blocks before scheduling new work.
	require.True(t, Registry().Pause(runID), "Pause must flip the gate")

	// Give in-flight iterations time to drain.
	time.Sleep(800 * time.Millisecond)
	pausedCount := persister.iterationCount()

	// While paused, the count must NOT advance.
	time.Sleep(500 * time.Millisecond)
	require.Equal(t, pausedCount, persister.iterationCount(), "no new iterations should land while paused")

	// Resume — assignment loop unblocks.
	require.True(t, Registry().Resume(runID), "Resume must flip the gate")

	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("run did not complete after resume")
	}
	require.Equal(t, 20, persister.iterationCount(), "all 20 iterations should complete after resume")
}

func TestEngine_Cancel(t *testing.T) {
	wsURL, srv := startSlowEchoWS(t, 200*time.Millisecond)
	defer srv.Close()

	// 100 iterations × 200ms with concurrency 4 = ~5s sequential floor. We cancel
	// after ~2 iterations are visible, so the count must end up well below 100.
	payloads := make([]string, 100)
	for i := range payloads {
		payloads[i] = "p"
	}

	cfg := WsFuzzerConfig{
		TargetURL: wsURL,
		Mode:      fuzz.ModeSingle,
		Script: []WsFuzzStep{{
			Role:      RoleFuzz,
			Opcode:    1,
			Content:   "x",
			Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1, OriginalValue: "x"}},
			WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchAny, TimeoutMs: 2000},
		}},
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: payloads},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 4, RequestTimeoutSeconds: 5},
	}

	persister := &fakeRunPersister{}
	bcast := stream.NewBroadcaster(64, 1000)
	runID := uint(101)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, runID, cfg, EngineDeps{Persister: persister, Broadcaster: bcast, Dial: engineDial})
	}()

	// Let some iterations complete first so we know the engine is making progress.
	require.Eventually(t, func() bool {
		return persister.iterationCount() >= 2
	}, 5*time.Second, 50*time.Millisecond, "engine should produce iterations before cancel")

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after parent context cancel")
	}

	require.Less(t, persister.iterationCount(), 100, "cancel must stop scheduling well before all 100 iterations land")
}
