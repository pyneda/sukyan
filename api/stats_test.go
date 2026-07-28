package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func statsRollupsTestApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/stats/workspaces", ListWorkspaceRollupsHandler)
	return app
}

// TestListWorkspaceRollupsHandler_Valid exercises the happy path: a valid
// request reaches db.ListWorkspaceRollups and returns the created workspace.
func TestListWorkspaceRollupsHandler_Valid(t *testing.T) {
	conn := db.Connection()
	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Title: "Rollups Handler Test " + t.Name(),
		Code:  "rollups_handler_" + sanitizeForCode(t.Name()),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })

	app := statsRollupsTestApp()
	resp := doJSON(t, app, "GET", "/api/v1/stats/workspaces?page=1&page_size=5&sort_by=critical&sort_order=desc&query="+ws.Code, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body WorkspaceRollupsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, int64(1), body.Count)
	require.Len(t, body.Data, 1)
	require.Equal(t, ws.ID, body.Data[0].WorkspaceID)
	require.Equal(t, ws.Code, body.Data[0].Code)
}

// TestListWorkspaceRollupsHandler_InvalidSortBy checks the sort_by allowlist
// rejects unknown values instead of forwarding them to the DB layer.
func TestListWorkspaceRollupsHandler_InvalidSortBy(t *testing.T) {
	app := statsRollupsTestApp()
	resp := doJSON(t, app, "GET", "/api/v1/stats/workspaces?sort_by=drop_table", nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "Invalid sort field", body.Error)
}

// TestListWorkspaceRollupsHandler_InvalidSortOrder checks sort_order is
// restricted to asc/desc.
func TestListWorkspaceRollupsHandler_InvalidSortOrder(t *testing.T) {
	app := statsRollupsTestApp()
	resp := doJSON(t, app, "GET", "/api/v1/stats/workspaces?sort_order=sideways", nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "Invalid sort order", body.Error)
}

// TestListWorkspaceRollupsHandler_NegativePage guards against the defect
// found in db.ListWorkspaceRollups: Pagination.GetData() normalises Page ==
// 0 to 1 but does not guard a negative Page, so a negative page produces a
// negative slice offset and panics. The handler must reject page < 1 before
// ever reaching db.Connection().ListWorkspaceRollups. If this guard were
// removed, this test would crash the whole test binary (a panic inside a
// Fiber handler without a recover middleware propagates out of app.Test),
// not just fail an assertion.
func TestListWorkspaceRollupsHandler_NegativePage(t *testing.T) {
	app := statsRollupsTestApp()
	resp := doJSON(t, app, "GET", "/api/v1/stats/workspaces?page=-5", nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "Invalid page", body.Error)
}

// TestListWorkspaceRollupsHandler_NegativePageSize mirrors the page guard for
// page_size, per the same validation style.
func TestListWorkspaceRollupsHandler_NegativePageSize(t *testing.T) {
	app := statsRollupsTestApp()
	resp := doJSON(t, app, "GET", "/api/v1/stats/workspaces?page_size=-1", nil)
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body ErrorResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Equal(t, "Invalid page size", body.Error)
}
