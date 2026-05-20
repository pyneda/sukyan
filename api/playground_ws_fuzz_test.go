package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
