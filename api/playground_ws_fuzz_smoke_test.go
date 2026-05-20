package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/websocket"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsreplay"
	"github.com/stretchr/testify/require"
)

// wsFuzzSmokeApp wires up the full ws-fuzz route surface for the smoke test.
// Mirrors the production wiring in api/server.go but without the JWT middleware
// (the handler functions are invoked directly — JWT is exercised by the
// existing JWT middleware tests, not here).
func wsFuzzSmokeApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/playground/sessions/:id/ws-fuzzer-config", GetWsFuzzerConfig)
	app.Put("/api/v1/playground/sessions/:id/ws-fuzzer-config", PutWsFuzzerConfig)
	app.Post("/api/v1/playground/ws-fuzz/preview", PreviewWsFuzz)
	app.Post("/api/v1/playground/ws-fuzz/sessions/:id/runs", ScheduleWsFuzzRun)
	app.Get("/api/v1/playground/ws-fuzz/sessions/:id/runs", ListWsFuzzRunsForSession)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id", GetWsFuzzRun)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/iterations", ListWsFuzzIterations)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/iterations/:index", GetWsFuzzIteration)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/iterations/:index/frames", GetWsFuzzIterationFrames)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/export.csv", ExportWsFuzzRunCSV)
	app.Get("/api/v1/playground/ws-fuzz/matcher-fields", GetWsFuzzMatcherFields)
	return app
}

func startSmokeEchoWS(t *testing.T) (string, *httptest.Server) {
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

// TestSmoke_FullWsFuzzUserJourney walks the entire user journey:
//  1. PUT a config
//  2. GET it back
//  3. POST preview
//  4. POST schedule a run
//  5. Wait for the run to terminate
//  6. GET the run row
//  7. LIST runs for the session (must include the new one)
//  8. LIST iterations
//  9. GET one iteration detail
//  10. GET iteration frames
//  11. GET CSV export
//  12. GET matcher-fields metadata
func TestSmoke_FullWsFuzzUserJourney(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()

	wsURL, srv := startSmokeEchoWS(t)
	defer srv.Close()

	app := wsFuzzSmokeApp()
	cfg := wsfuzz.WsFuzzerConfig{
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
		SharedPayloads:   &fuzz.FuzzerPayloadsGroup{Payloads: []string{"a", "b", "c"}},
		ExecutionOptions: fuzz.FuzzerExecutionOptions{Concurrency: 1, RequestTimeoutSeconds: 5},
	}

	// 1. PUT config.
	resp := doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/playground/sessions/%d/ws-fuzzer-config", sessionID), cfg)
	require.Equal(t, http.StatusNoContent, resp.StatusCode, "PUT config")

	// 2. GET config.
	resp = doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/sessions/%d/ws-fuzzer-config", sessionID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET config")
	var roundtrip wsfuzz.WsFuzzerConfig
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&roundtrip))
	require.Equal(t, wsURL, roundtrip.TargetURL)

	// 3. Preview.
	resp = doJSON(t, app, "POST", "/api/v1/playground/ws-fuzz/preview", cfg)
	require.Equal(t, http.StatusOK, resp.StatusCode, "preview")
	var preview previewWsFuzzResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&preview))
	require.Equal(t, 3, preview.IterationCount)
	require.Empty(t, preview.Errors)

	// 4. Schedule a run.
	resp = doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/sessions/%d/runs", sessionID), cfg)
	require.Equal(t, http.StatusOK, resp.StatusCode, "schedule")
	var sched scheduleWsFuzzResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&sched))
	require.NotZero(t, sched.RunID)
	require.Equal(t, 3, sched.IterationCount)

	// 5. Wait for terminal.
	require.Eventually(t, func() bool {
		r, err := conn.GetPlaygroundWsFuzzRun(sched.RunID)
		if err != nil {
			return false
		}
		return r.Status == "succeeded" || r.Status == "failed"
	}, 15*time.Second, 100*time.Millisecond, "run must reach terminal status")

	// 6. GET run row.
	resp = doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d", sched.RunID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get run")
	var runRow db.PlaygroundWsFuzzRun
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&runRow))
	require.Equal(t, "succeeded", runRow.Status)

	// 7. LIST runs for session (must include the new one).
	resp = doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/sessions/%d/runs", sessionID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "list runs")
	var runs struct {
		Runs  []db.PlaygroundWsFuzzRun `json:"runs"`
		Total int64                    `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&runs))
	require.GreaterOrEqual(t, runs.Total, int64(1))
	foundRun := false
	for _, r := range runs.Runs {
		if r.ID == sched.RunID {
			foundRun = true
		}
	}
	require.True(t, foundRun, "scheduled run must be in the session's run list")

	// 8. LIST iterations.
	resp = doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations", sched.RunID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "list iterations")
	var iters struct {
		Iterations []db.PlaygroundWsFuzzIteration `json:"iterations"`
		Total      int64                          `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&iters))
	require.Equal(t, int64(3), iters.Total)

	// 9. GET one iteration detail.
	resp = doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations/0", sched.RunID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get iteration")
	var it db.PlaygroundWsFuzzIteration
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&it))
	require.Equal(t, "completed", it.Status)

	// 10. GET iteration frames.
	resp = doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations/0/frames", sched.RunID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get iteration frames")
	var framesBody struct {
		Frames []db.WebSocketMessage `json:"frames"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&framesBody))
	// Should include both the sent payload and the received echo frame.
	require.GreaterOrEqual(t, len(framesBody.Frames), 2)

	// 11. CSV export.
	resp = doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/export.csv", sched.RunID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "export csv")
	require.Equal(t, "text/csv", resp.Header.Get("Content-Type"))
	body, _ := io.ReadAll(resp.Body)
	require.Contains(t, string(body), "iteration,status,baseline_match")

	// 12. Matcher-fields metadata.
	resp = doJSON(t, app, "GET", "/api/v1/playground/ws-fuzz/matcher-fields", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode, "matcher-fields")
	var meta struct {
		Fields []map[string]any `json:"fields"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&meta))
	require.NotEmpty(t, meta.Fields)
}
