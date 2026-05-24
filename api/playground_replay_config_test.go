package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/manual"
	"github.com/stretchr/testify/require"
)

func replayConfigTestApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/playground/sessions/:id/replay-config", GetReplayConfig)
	app.Put("/api/v1/playground/sessions/:id/replay-config", PutReplayConfig)
	app.Post("/api/v1/playground/sessions/:id/replay-config/flush", FlushReplayConfig)
	return app
}

func createReplayWorkspaceSession(t *testing.T) (workspaceID, sessionID uint) {
	t.Helper()
	conn := db.Connection()
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{Title: "replay-api-" + t.Name(), Code: "replayapi_" + sanitizeForCode(t.Name())})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })
	coll := &db.PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &db.PlaygroundSession{Name: "s", Type: db.ManualType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	return ws.ID, sess.ID
}

func TestGetReplayConfig_NoContent(t *testing.T) {
	app := replayConfigTestApp()
	_, sessID := createReplayWorkspaceSession(t)
	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/sessions/%d/replay-config", sessID), nil)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestPutThenGetReplayConfig_RoundTrip(t *testing.T) {
	app := replayConfigTestApp()
	_, sessID := createReplayWorkspaceSession(t)
	cfg := ReplayConfig{
		URL:        "https://example.com/test",
		RawRequest: "GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n",
		Options: manual.RequestOptions{
			FollowRedirects: true,
			MaxRedirects:    5,
		},
		ReplayMode: "raw",
	}
	put := doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/playground/sessions/%d/replay-config", sessID), cfg)
	require.Equal(t, http.StatusNoContent, put.StatusCode)

	got := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/sessions/%d/replay-config", sessID), nil)
	require.Equal(t, http.StatusOK, got.StatusCode)
	var result ReplayConfig
	require.NoError(t, json.NewDecoder(got.Body).Decode(&result))
	require.Equal(t, cfg.URL, result.URL)
	require.Equal(t, cfg.RawRequest, result.RawRequest)
	require.Equal(t, cfg.ReplayMode, result.ReplayMode)
	require.Equal(t, cfg.Options.FollowRedirects, result.Options.FollowRedirects)
	require.Equal(t, cfg.Options.MaxRedirects, result.Options.MaxRedirects)
}

func TestFlushReplayConfig_RoundTrip(t *testing.T) {
	app := replayConfigTestApp()
	_, sessID := createReplayWorkspaceSession(t)
	cfg := ReplayConfig{
		URL:        "https://example.com/flush",
		RawRequest: "POST /flush HTTP/1.1\r\nHost: example.com\r\n\r\n",
		ReplayMode: "raw",
	}
	flush := doJSON(t, app, "POST", fmt.Sprintf("/api/v1/playground/sessions/%d/replay-config/flush", sessID), cfg)
	require.Equal(t, http.StatusNoContent, flush.StatusCode)

	got := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/playground/sessions/%d/replay-config", sessID), nil)
	require.Equal(t, http.StatusOK, got.StatusCode)
	var result ReplayConfig
	require.NoError(t, json.NewDecoder(got.Body).Decode(&result))
	require.Equal(t, cfg.URL, result.URL)
	require.Equal(t, cfg.ReplayMode, result.ReplayMode)
}

func TestPutReplayConfig_InvalidJSON(t *testing.T) {
	app := replayConfigTestApp()
	_, sessID := createReplayWorkspaceSession(t)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/playground/sessions/%d/replay-config", sessID), bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestPutReplayConfig_NotFound(t *testing.T) {
	app := replayConfigTestApp()
	cfg := ReplayConfig{URL: "https://example.com/", ReplayMode: "raw"}
	resp := doJSON(t, app, "PUT", "/api/v1/playground/sessions/99999/replay-config", cfg)
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}
