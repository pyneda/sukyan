package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/pyneda/sukyan/pkg/playground/wsfuzz"
	"github.com/stretchr/testify/require"
)

// ensureWsFuzzTablesApi mirrors the db package's ensureWsFuzzTables helper for
// API-layer tests. Creates the two ws_fuzz tables if missing.
func ensureWsFuzzTablesApi(t *testing.T) {
	t.Helper()
	m := db.Connection().DB().Migrator()
	if !m.HasTable(&db.PlaygroundWsFuzzRun{}) {
		require.NoError(t, m.CreateTable(&db.PlaygroundWsFuzzRun{}))
	}
	if !m.HasTable(&db.PlaygroundWsFuzzIteration{}) {
		require.NoError(t, m.CreateTable(&db.PlaygroundWsFuzzIteration{}))
	}
}

// createWsFuzzWorkspaceSession creates a workspace + collection + parent
// playground_session (type ws_fuzz) + playground_ws_session, and returns the
// parent session ID. Mirrors createFuzzWorkspaceSession from api/fuzz_test.go.
func createWsFuzzWorkspaceSession(t *testing.T) (workspaceID, sessionID uint) {
	t.Helper()
	conn := db.Connection()
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{Title: "ws-fuzz-api-" + t.Name(), Code: "wsfuzzapi_" + sanitizeForCode(t.Name())})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })
	coll := &db.PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &db.PlaygroundSession{Name: "s", Type: db.WsFuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	wsSess := &db.PlaygroundWsSession{PlaygroundSessionID: sess.ID}
	require.NoError(t, conn.CreatePlaygroundWsSession(wsSess))
	return ws.ID, sess.ID
}

func wsFuzzConfigApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/playground/sessions/:id/ws-fuzzer-config", GetWsFuzzerConfig)
	app.Put("/api/v1/playground/sessions/:id/ws-fuzzer-config", PutWsFuzzerConfig)
	return app
}

func TestGetWsFuzzerConfig_204WhenEmpty(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	app := wsFuzzConfigApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/sessions/%d/ws-fuzzer-config", sessionID), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestPutThenGetWsFuzzerConfig(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	app := wsFuzzConfigApp()

	cfg := wsfuzz.WsFuzzerConfig{
		TargetURL: "ws://example.com/ws",
		Mode:      fuzz.ModeSingle,
		Script:    []wsfuzz.WsFuzzStep{{ID: "s1", Role: wsfuzz.RoleSetup, Opcode: 1, Content: "{}"}},
	}

	put := doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/playground/sessions/%d/ws-fuzzer-config", sessionID), cfg)
	require.Equal(t, http.StatusNoContent, put.StatusCode)

	get := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/sessions/%d/ws-fuzzer-config", sessionID), nil)
	require.Equal(t, http.StatusOK, get.StatusCode)

	var got wsfuzz.WsFuzzerConfig
	require.NoError(t, json.NewDecoder(get.Body).Decode(&got))
	require.Equal(t, "ws://example.com/ws", got.TargetURL)
	require.Equal(t, fuzz.ModeSingle, got.Mode)
	require.Len(t, got.Script, 1)
	require.Equal(t, "s1", got.Script[0].ID)
}

func TestPutWsFuzzerConfig_404WhenNoSession(t *testing.T) {
	app := wsFuzzConfigApp()
	cfg := wsfuzz.WsFuzzerConfig{TargetURL: "ws://x/", Mode: fuzz.ModeSingle}
	put := doJSON(t, app, "PUT", "/api/v1/playground/sessions/9999999/ws-fuzzer-config", cfg)
	require.Equal(t, http.StatusNotFound, put.StatusCode)
}

func wsFuzzPreviewApp() *fiber.App {
	app := fiber.New()
	app.Post("/api/v1/playground/ws-fuzz/preview", PreviewWsFuzz)
	return app
}

func TestPreviewWsFuzz_ReturnsIterationCount(t *testing.T) {
	cfg := wsfuzz.WsFuzzerConfig{
		TargetURL: "ws://example.com/ws",
		Mode:      fuzz.ModeSingle,
		Script: []wsfuzz.WsFuzzStep{{
			Role: wsfuzz.RoleFuzz, Opcode: 1, Content: "x",
			Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1, OriginalValue: "x"}},
		}},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"a", "b", "c"}},
	}
	app := wsFuzzPreviewApp()
	resp := doJSON(t, app, "POST", "/api/v1/playground/ws-fuzz/preview", cfg)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out previewWsFuzzResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, 3, out.IterationCount)
	require.Equal(t, 1, out.PositionsCount)
}

func TestPreviewWsFuzz_ReportsErrors(t *testing.T) {
	app := wsFuzzPreviewApp()
	resp := doJSON(t, app, "POST", "/api/v1/playground/ws-fuzz/preview", wsfuzz.WsFuzzerConfig{Mode: fuzz.ModeSingle})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out previewWsFuzzResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotEmpty(t, out.Errors)
}

func TestPreviewWsFuzz_BadJSON(t *testing.T) {
	app := wsFuzzPreviewApp()
	// Send malformed JSON via direct httptest (doJSON marshals everything we hand it).
	req := httptest.NewRequest("POST", "/api/v1/playground/ws-fuzz/preview", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func wsFuzzScheduleApp() *fiber.App {
	app := fiber.New()
	app.Post("/api/v1/playground/ws-fuzz/sessions/:id/runs", ScheduleWsFuzzRun)
	return app
}

func TestScheduleWsFuzzRun_RejectsInvalidConfig(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	app := wsFuzzScheduleApp()
	// Missing target URL + no positions → Validate returns errors.
	cfg := wsfuzz.WsFuzzerConfig{Mode: fuzz.ModeSingle}
	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/sessions/%d/runs", sessionID), cfg)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotEmpty(t, out["errors"])
}

func TestScheduleWsFuzzRun_CreatesRunAndReturnsID(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	app := wsFuzzScheduleApp()

	// Use a non-routable URL so the engine's dial fails fast — we only want to
	// verify the row was created and a run_id returned. The background goroutine
	// will write status=failed eventually; the assertion looks at the
	// scheduling response, not the engine outcome.
	cfg := wsfuzz.WsFuzzerConfig{
		TargetURL:         "ws://127.0.0.1:1/", // port 1 is reserved; dial fails
		Mode:              fuzz.ModeSingle,
		ConnectionTimeout: 200, // 200ms so dial fails quickly
		Script: []wsfuzz.WsFuzzStep{{
			Role:      wsfuzz.RoleFuzz,
			Opcode:    1,
			Content:   "x",
			Positions: []fuzz.FuzzerPosition{{Start: 0, End: 1, OriginalValue: "x"}},
		}},
		SharedPayloads: &fuzz.FuzzerPayloadsGroup{Payloads: []string{"a", "b"}},
	}

	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/sessions/%d/runs", sessionID), cfg)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out scheduleWsFuzzResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotZero(t, out.RunID)
	require.Equal(t, 2, out.IterationCount)

	// Verify the run row exists in the DB.
	got, err := db.Connection().GetPlaygroundWsFuzzRun(out.RunID)
	require.NoError(t, err)
	require.Equal(t, uint(sessionID), got.SessionID)
}

func wsFuzzRunControlApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id", GetWsFuzzRun)
	app.Delete("/api/v1/playground/ws-fuzz/runs/:run_id", CancelWsFuzzRun)
	app.Post("/api/v1/playground/ws-fuzz/runs/:run_id/pause", PauseWsFuzzRun)
	app.Post("/api/v1/playground/ws-fuzz/runs/:run_id/resume", ResumeWsFuzzRun)
	return app
}

func TestGetWsFuzzRun_404OnMissing(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "GET", "/api/v1/playground/ws-fuzz/runs/9999999", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetWsFuzzRun_ReturnsRow(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded", IterationCount: 42}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got db.PlaygroundWsFuzzRun
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, run.ID, got.ID)
	require.Equal(t, "succeeded", got.Status)
	require.Equal(t, 42, got.IterationCount)
}

func TestCancelWsFuzzRun_AlreadyFinished(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "DELETE", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, "already_finished", out["status"])
}

func TestPauseWsFuzzRun_TerminalReturns409(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/pause", run.ID), nil)
	require.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestPauseWsFuzzRun_AlreadyPaused(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "paused"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/pause", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, "already_paused", out["status"])
}

func TestPauseWsFuzzRun_RunningFlipsToPaused(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "running"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/pause", run.ID), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	got, err := conn.GetPlaygroundWsFuzzRun(run.ID)
	require.NoError(t, err)
	require.Equal(t, "paused", got.Status)
}

func TestResumeWsFuzzRun_NotPaused(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "running"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/resume", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, "not_paused", out["status"])
}

func TestResumeWsFuzzRun_PausedFlipsToRunning(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "paused"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	app := wsFuzzRunControlApp()
	resp := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/resume", run.ID), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	got, err := conn.GetPlaygroundWsFuzzRun(run.ID)
	require.NoError(t, err)
	require.Equal(t, "running", got.Status)
}

func wsFuzzListApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/playground/ws-fuzz/sessions/:id/runs", ListWsFuzzRunsForSession)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/iterations", ListWsFuzzIterations)
	return app
}

func TestListWsFuzzRunsForSession(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	for i := 0; i < 3; i++ {
		r := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded", IterationCount: 10}
		require.NoError(t, conn.CreatePlaygroundWsFuzzRun(r))
	}

	app := wsFuzzListApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/sessions/%d/runs?page=1&page_size=10", sessionID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Runs  []db.PlaygroundWsFuzzRun `json:"runs"`
		Total int64                    `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, int64(3), out.Total)
	require.Len(t, out.Runs, 3)
}

func TestListWsFuzzIterations_WithStatusFilter(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	for i, st := range []string{"completed", "completed", "check_failed", "connection_error"} {
		it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: i, Status: st}
		require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))
	}

	app := wsFuzzListApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations?status=check_failed", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Iterations []db.PlaygroundWsFuzzIteration `json:"iterations"`
		Total      int64                          `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, int64(1), out.Total)
	require.Equal(t, "check_failed", out.Iterations[0].Status)
}

func TestListWsFuzzIterations_NoFilters(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	for i := 0; i < 4; i++ {
		it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: i, Status: "completed"}
		require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))
	}

	app := wsFuzzListApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Total int64 `json:"total"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Equal(t, int64(4), out.Total)
}

func wsFuzzDetailApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/iterations/:index", GetWsFuzzIteration)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/iterations/:index/frames", GetWsFuzzIterationFrames)
	return app
}

func TestGetWsFuzzIteration_ReturnsRow(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: 7, Status: "completed", DurationMs: 200}
	require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))

	app := wsFuzzDetailApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations/7", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got db.PlaygroundWsFuzzIteration
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, 7, got.IterationIndex)
	require.Equal(t, 200, got.DurationMs)
}

func TestGetWsFuzzIteration_404OnMissing(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	app := wsFuzzDetailApp()
	resp := doJSON(t, app, "GET", "/api/v1/playground/ws-fuzz/runs/9999999/iterations/0", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetWsFuzzIterationFrames_EmptyWhenNoConnection(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: 0, Status: "completed"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))

	app := wsFuzzDetailApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations/0/frames", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Frames []db.WebSocketMessage `json:"frames"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Empty(t, out.Frames)
}

func TestGetWsFuzzIterationFrames_ReturnsMessages(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	workspaceID, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()

	// Create a WebSocketConnection + 3 messages.
	wsConn := &db.WebSocketConnection{URL: "ws://x/", Source: db.SourceWsFuzz, WorkspaceID: &workspaceID}
	require.NoError(t, conn.CreateWebSocketConnection(wsConn))
	for i := 0; i < 3; i++ {
		m := &db.WebSocketMessage{
			ConnectionID: wsConn.ID,
			Opcode:       1,
			PayloadData:  fmt.Sprintf("msg-%d", i),
			Direction:    db.MessageSent,
		}
		require.NoError(t, conn.CreateWebSocketMessage(m))
	}

	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	it := &db.PlaygroundWsFuzzIteration{
		RunID:                 run.ID,
		IterationIndex:        0,
		Status:                "completed",
		WebSocketConnectionID: &wsConn.ID,
	}
	require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))

	app := wsFuzzDetailApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/iterations/0/frames", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out struct {
		Frames []db.WebSocketMessage `json:"frames"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out.Frames, 3)
}
