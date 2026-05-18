package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"github.com/stretchr/testify/require"
)

// fuzzTestApp returns a fiber app wired only to the fuzz endpoints — keeps
// each test scoped to what it's checking.
func fuzzTestApp() *fiber.App {
	app := fiber.New()
	app.Post("/api/v1/playground/fuzz/preview", FuzzPreview)
	app.Post("/api/v1/playground/fuzz", FuzzRequest)
	app.Get("/api/v1/playground/fuzz/runs/:run_id", GetFuzzRun)
	app.Delete("/api/v1/playground/fuzz/runs/:run_id", CancelFuzzRun)
	app.Get("/api/v1/playground/sessions/:id/fuzz-runs", ListFuzzRunsForSession)
	return app
}

func ensureFuzzRunTableApi(t *testing.T) {
	t.Helper()
	m := db.Connection().DB().Migrator()
	if !m.HasTable(&db.PlaygroundFuzzRun{}) {
		require.NoError(t, m.CreateTable(&db.PlaygroundFuzzRun{}))
	}
}

func createFuzzWorkspaceSession(t *testing.T) (workspaceID, sessionID uint) {
	t.Helper()
	conn := db.Connection()
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{Title: "fuzz-api-" + t.Name(), Code: "fuzzapi_" + sanitizeForCode(t.Name())})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })
	coll := &db.PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &db.PlaygroundSession{Name: "s", Type: db.FuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	return ws.ID, sess.ID
}

func sanitizeForCode(s string) string {
	return strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(s)
}

func doJSON(t *testing.T, app *fiber.App, method, path string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	return resp
}

func TestFuzzPreviewSingleCount(t *testing.T) {
	app := fuzzTestApp()
	resp := doJSON(t, app, "POST", "/api/v1/playground/fuzz/preview", map[string]any{
		"mode":      "single",
		"positions": []map[string]any{{"start": 0, "end": 1, "originalValue": "a"}, {"start": 2, "end": 3, "originalValue": "b"}},
		"shared_payloads": map[string]any{"payloads": []string{"x", "y", "z"}, "type": "list"},
	})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got fuzz.PreviewResult
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, 6, got.RequestCount)
}

func TestFuzzPreviewValidationError(t *testing.T) {
	app := fuzzTestApp()
	resp := doJSON(t, app, "POST", "/api/v1/playground/fuzz/preview", map[string]any{
		"mode":      "single",
		"positions": []map[string]any{{"start": 0, "end": 1, "originalValue": "a"}},
		// no shared_payloads → invalid for single mode
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestFuzzLaunchAndCancel(t *testing.T) {
	ensureFuzzRunTableApi(t)
	wsID, sessID := createFuzzWorkspaceSession(t)
	_ = wsID

	// Slow server so the run is still in flight when we cancel.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	raw := fmt.Sprintf("GET /?q=Q HTTP/1.1\r\nHost: %s\r\n\r\n", strings.TrimPrefix(srv.URL, "http://"))
	qIdx := strings.Index(raw, "Q")
	payloads := make([]string, 50)
	for i := range payloads {
		payloads[i] = fmt.Sprintf("p%d", i)
	}

	app := fuzzTestApp()
	launch := doJSON(t, app, "POST", "/api/v1/playground/fuzz", map[string]any{
		"url":         srv.URL,
		"raw_request": raw,
		"session_id":  sessID,
		"mode":        "single",
		"positions":   []map[string]any{{"start": qIdx, "end": qIdx + 1, "originalValue": "Q"}},
		"shared_payloads": map[string]any{"payloads": payloads, "type": "list"},
		"execution":   map[string]any{"concurrency": 2, "request_timeout_seconds": 30},
	})
	require.Equal(t, http.StatusOK, launch.StatusCode, "launch response")
	var launchResp PlaygroundFuzzResponse
	require.NoError(t, json.NewDecoder(launch.Body).Decode(&launchResp))
	require.NotZero(t, launchResp.RunID)
	require.Equal(t, 50, launchResp.RequestsCount)

	// Give the engine a moment to start, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancelResp := doJSON(t, app, "DELETE", fmt.Sprintf("/api/v1/playground/fuzz/runs/%d", launchResp.RunID), nil)
	require.Equal(t, http.StatusNoContent, cancelResp.StatusCode)

	// Wait for the engine to finalize the run.
	deadline := time.Now().Add(3 * time.Second)
	var run *db.PlaygroundFuzzRun
	for time.Now().Before(deadline) {
		r, err := db.Connection().GetPlaygroundFuzzRun(launchResp.RunID)
		require.NoError(t, err)
		if r.Status.IsTerminal() {
			run = r
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	require.NotNil(t, run, "expected run to reach terminal status")
	require.Equal(t, db.FuzzRunCancelled, run.Status)
}

func TestFuzzLaunchRejectsInvalidConfig(t *testing.T) {
	ensureFuzzRunTableApi(t)
	_, sessID := createFuzzWorkspaceSession(t)
	app := fuzzTestApp()
	resp := doJSON(t, app, "POST", "/api/v1/playground/fuzz", map[string]any{
		"url":         "http://example.com",
		"raw_request": "GET / HTTP/1.1\r\n\r\n",
		"session_id":  sessID,
		"mode":        "single",
		"positions":   []map[string]any{{"start": 0, "end": 1, "originalValue": "a"}},
		// missing shared_payloads — invalid for single
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestListFuzzRunsForSession(t *testing.T) {
	ensureFuzzRunTableApi(t)
	wsID, sessID := createFuzzWorkspaceSession(t)
	conn := db.Connection()
	for i := 0; i < 3; i++ {
		run := &db.PlaygroundFuzzRun{
			PlaygroundSessionID: sessID,
			WorkspaceID:         wsID,
			ConfigSnapshot:      []byte(`{}`),
			Status:              db.FuzzRunSucceeded,
		}
		require.NoError(t, conn.CreatePlaygroundFuzzRun(run))
	}
	app := fuzzTestApp()
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/sessions/%d/fuzz-runs", sessID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, float64(3), got["total"].(float64))
}
