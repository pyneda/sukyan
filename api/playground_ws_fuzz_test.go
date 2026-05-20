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
