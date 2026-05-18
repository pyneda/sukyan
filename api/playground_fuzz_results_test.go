package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func TestListFuzzRunResults(t *testing.T) {
	ensureFuzzRunTableApi(t)
	wsID, sessID := createFuzzWorkspaceSession(t)
	conn := db.Connection()

	run := &db.PlaygroundFuzzRun{
		PlaygroundSessionID: sessID,
		WorkspaceID:         wsID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              db.FuzzRunSucceeded,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))

	// Create 5 history rows tagged to this run, varying status codes.
	for i, code := range []int{200, 200, 404, 200, 500} {
		h := &db.History{
			URL:                 fmt.Sprintf("http://example.com/%d", i),
			Method:              "GET",
			StatusCode:          code,
			Source:              db.SourceFuzzer,
			WorkspaceID:         &wsID,
			PlaygroundFuzzRunID: &run.ID,
		}
		require.NoError(t, conn.DB().Create(h).Error)
	}

	app := fiber.New()
	app.Get("/api/v1/playground/fuzz/runs/:run_id/results", ListFuzzRunResults)

	// Default list returns all 5.
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/results", run.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, float64(5), body["total"].(float64))
	require.Len(t, body["results"].([]any), 5)

	// Status filter narrows to status=200 (3 of them).
	resp2 := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d/results?status=200", run.ID), nil)
	require.Equal(t, http.StatusOK, resp2.StatusCode)
	var body2 map[string]any
	require.NoError(t, json.NewDecoder(resp2.Body).Decode(&body2))
	require.Equal(t, float64(3), body2["total"].(float64))

	// Non-existent run → 404.
	resp3 := doJSON(t, app, "GET", "/api/v1/playground/fuzz/runs/999999/results", nil)
	require.Equal(t, http.StatusNotFound, resp3.StatusCode)
}
