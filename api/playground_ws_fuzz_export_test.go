package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func wsFuzzExportApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/export.csv", ExportWsFuzzRunCSV)
	app.Get("/api/v1/playground/ws-fuzz/runs/:run_id/export.json", ExportWsFuzzRunJSON)
	return app
}

func TestExportWsFuzzRunCSV_AllRows(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	for i, st := range []string{"completed", "check_failed", "completed"} {
		it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: i, Status: st, DurationMs: 100 + i}
		require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))
	}

	app := wsFuzzExportApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/export.csv", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/csv", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	csv := string(body)
	require.Contains(t, csv, "iteration,status,baseline_match")
	// Header line + 3 data rows = 4 lines (with trailing newline).
	require.Equal(t, 4, strings.Count(csv, "\n"))
}

func TestExportWsFuzzRunCSV_FindingsOnly(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	for i, st := range []string{"completed", "check_failed", "completed"} {
		it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: i, Status: st}
		require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))
	}

	app := wsFuzzExportApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/export.csv?findings_only=true", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	// Header + 1 finding row = 2 lines.
	require.Equal(t, 2, strings.Count(string(body), "\n"))
}

func TestExportWsFuzzRunCSV_404OnMissingRun(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	app := wsFuzzExportApp()
	resp := doJSON(t, app, "GET", "/api/v1/playground/ws-fuzz/runs/9999999/export.csv", nil)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestExportWsFuzzRunJSON_NoFrames(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	for i, st := range []string{"completed", "check_failed"} {
		it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: i, Status: st}
		require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))
	}

	app := wsFuzzExportApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/export.json", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.NotNil(t, out["run"])
	require.Len(t, out["iterations"], 2)
	_, hasFrames := out["frames"]
	require.False(t, hasFrames, "frames must NOT be included without include_frames=true")
}

func TestExportWsFuzzRunJSON_FindingsOnly(t *testing.T) {
	ensureWsFuzzTablesApi(t)
	_, sessionID := createWsFuzzWorkspaceSession(t)
	conn := db.Connection()
	run := &db.PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	for i, st := range []string{"completed", "check_failed", "completed"} {
		it := &db.PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: i, Status: st}
		require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(it))
	}

	app := wsFuzzExportApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/ws-fuzz/runs/%d/export.json?findings_only=true", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&out))
	require.Len(t, out["iterations"], 1)
}
