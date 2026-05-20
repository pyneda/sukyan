package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

// startConcurrentTestEchoWS spins up an in-process WS echo server for the
// concurrent-runs test. Local to the api package so we don't import test
// helpers across packages.
func startConcurrentTestEchoWS(t *testing.T) (string, *httptest.Server) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
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
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/", srv
}

func TestConcurrentWsFuzzRunsOnSameSession(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()

	wsURL, srv := startConcurrentTestEchoWS(t)
	defer srv.Close()

	app := wsFuzzScheduleApp()

	mkCfg := func(payloads []string) wsfuzz.WsFuzzerConfig {
		return wsfuzz.WsFuzzerConfig{
			TargetURL: wsURL,
			Mode:      fuzz.ModeSingle,
			Script: []wsfuzz.WsFuzzStep{{
				ID:        "s1",
				Role:      wsfuzz.RoleFuzz,
				Opcode:    1,
				Content:   "x",
				Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1, OriginalValue: "x"}},
				WaitFor:   &wsreplay.WaitForSpec{MatchType: wsreplay.MatchContains, Pattern: "echo:", TimeoutMs: 2000},
			}},
			SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: payloads},
			ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 1, RequestTimeoutSeconds: 5},
		}
	}

	// Schedule two runs on the SAME session, concurrently.
	var runIDs [2]uint
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			cfg := mkCfg([]string{"a", "b", "c"})
			resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/sessions/%d/runs", sessionID), cfg)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			var out scheduleWsFuzzResponse
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
			runIDs[i] = out.RunID
		}()
	}
	wg.Wait()

	// Both runs got distinct IDs.
	require.NotZero(t, runIDs[0])
	require.NotZero(t, runIDs[1])
	require.NotEqual(t, runIDs[0], runIDs[1], "concurrent schedules must yield distinct run IDs")

	// Wait for both runs to reach terminal status.
	require.Eventually(t, func() bool {
		for _, id := range runIDs {
			r, err := conn.GetPlaygroundWsFuzzRun(id)
			if err != nil {
				return false
			}
			switch r.Status {
			case "succeeded", "failed", "cancelled":
				continue
			default:
				return false
			}
		}
		return true
	}, 30*time.Second, 200*time.Millisecond, "both runs must reach terminal state")

	// Each run should have produced 3 iteration rows (one per payload).
	for _, id := range runIDs {
		iters, total, err := conn.ListPlaygroundWsFuzzIterations(db.PlaygroundWsFuzzIterationFilter{RunID: id, Page: 1, PageSize: 10})
		require.NoError(t, err)
		require.Equal(t, int64(3), total, "run %d should have 3 iterations", id)
		require.Len(t, iters, 3)
		for _, it := range iters {
			require.Equal(t, "completed", it.Status, "iteration must complete cleanly; failure_reason=%q", it.FailureReason)
		}
	}
}
