package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildScanInfo must carry workspace identity. Without it the global overview
// cannot label which workspace a cross-workspace scan row belongs to.
func TestBuildScanInfo_CarriesWorkspaceIdentity(t *testing.T) {
	conn := db.Connection()
	marker := lib.GenerateRandomLowercaseString(12)

	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Code:        "scaninfo-" + marker,
		Title:       "scaninfo " + marker,
		Description: "scan info workspace identity test",
	})
	require.NoError(t, err)
	t.Cleanup(func() { conn.DB().Delete(&db.Workspace{}, ws.ID) })

	scan := &db.Scan{
		Title:       "scaninfo scan " + marker,
		WorkspaceID: ws.ID,
		Status:      db.ScanStatusPending,
	}

	info := buildScanInfo(scan)

	assert.Equal(t, ws.ID, info.WorkspaceID)
}

// The deployment payload must be buildable without a scan manager, so the SPA
// still renders a degraded health strip when the manager is not running.
func TestBuildDashboardStats_TolerationsWithoutManager(t *testing.T) {
	stats := buildDashboardStats(nil)

	assert.False(t, stats.ManagerRunning)
	assert.False(t, stats.OrchestratorRunning)
	assert.NotNil(t, stats.ActiveScans)
	assert.NotNil(t, stats.PausedScans)
}

// TestGetDeploymentStatsHandler_ReturnsWorkspaceLabelledScans exercises the new
// JWT-facing /api/v1/stats/deployment route end to end (route -> handler ->
// buildDashboardStats -> resolveScanWorkspaceTitles), substituting for the
// manual curl verification described in the task brief since no live API
// server is available in this environment.
func TestGetDeploymentStatsHandler_ReturnsWorkspaceLabelledScans(t *testing.T) {
	conn := db.Connection()
	marker := lib.GenerateRandomLowercaseString(12)

	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Code:  "deploystats-" + marker,
		Title: "deploystats " + marker,
	})
	require.NoError(t, err)
	t.Cleanup(func() { conn.DB().Delete(&db.Workspace{}, ws.ID) })

	scan, err := conn.CreateScan(&db.Scan{
		WorkspaceID: ws.ID,
		Status:      db.ScanStatusScanning,
		Title:       "deploystats scan " + marker,
	})
	require.NoError(t, err)
	t.Cleanup(func() { conn.DB().Delete(&db.Scan{}, scan.ID) })

	app := fiber.New()
	app.Get("/api/v1/stats/deployment", GetDeploymentStatsHandler)

	resp := doJSON(t, app, "GET", "/api/v1/stats/deployment", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body DashboardStats
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	require.NotNil(t, body.ActiveScans)
	require.NotNil(t, body.PausedScans)
	require.NotNil(t, body.GlobalQueueStats)

	var found *ScanInfo
	for i := range body.ActiveScans {
		if body.ActiveScans[i].ID == scan.ID {
			found = &body.ActiveScans[i]
		}
	}
	require.NotNil(t, found, "expected the newly created scanning scan to appear in active_scans")
	assert.Equal(t, ws.ID, found.WorkspaceID)
	assert.Equal(t, ws.Title, found.WorkspaceTitle)
}
