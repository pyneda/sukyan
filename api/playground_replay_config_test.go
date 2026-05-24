package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
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
