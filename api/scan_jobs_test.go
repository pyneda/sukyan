package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The frontend deliberately sends no sort_by for this endpoint and relies on
// GetScanJobsHandler defaulting filter.SortBy to "relevance". The db-level
// ordering tests exercise ListScanJobs directly with an explicit sort_by, so
// they cannot catch a regression to the "id" default; only a test through the
// handler itself can.
func TestGetScanJobsHandler_DefaultsToRelevanceOrdering(t *testing.T) {
	conn := db.Connection()
	marker := lib.GenerateRandomLowercaseString(12)

	ws, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Code:  "scanjobs-" + marker,
		Title: "scanjobs " + marker,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.DeleteWorkspace(ws.ID) })

	scan, err := conn.CreateScan(&db.Scan{
		WorkspaceID: ws.ID,
		Status:      db.ScanStatusScanning,
		Title:       "scanjobs scan " + marker,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.DB().Where("scan_id = ?", scan.ID).Delete(&db.ScanJob{})
		conn.DB().Delete(&db.Scan{}, scan.ID)
	})

	// The running job is created first (lower id) and the pending job second
	// (higher id). GetScanJobsHandler's filter defaults SortOrder to "desc",
	// same as every other list handler in this file, so a regression from
	// "relevance" back to "id" sorts by id descending — which would put this
	// higher-id pending job first. Only the relevance default ranks the
	// running job first regardless of id.
	running, err := conn.CreateScanJob(&db.ScanJob{
		ScanID:  scan.ID,
		Status:  db.ScanJobStatusRunning,
		JobType: db.ScanJobTypeActiveScan,
		URL:     "https://example.com/running",
	})
	require.NoError(t, err)

	pending, err := conn.CreateScanJob(&db.ScanJob{
		ScanID:  scan.ID,
		Status:  db.ScanJobStatusPending,
		JobType: db.ScanJobTypeActiveScan,
		URL:     "https://example.com/pending",
	})
	require.NoError(t, err)
	require.Greater(t, pending.ID, running.ID, "the pending job must be created after the running one so an id-descending result would put it first")

	app := fiber.New()
	app.Get("/api/v1/scans/:id/jobs", GetScanJobsHandler)

	resp := doJSON(t, app, "GET", fmt.Sprintf("/api/v1/scans/%d/jobs", scan.ID), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body ScanJobsListResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	require.Len(t, body.Jobs, 2)
	assert.Equal(t, running.ID, body.Jobs[0].ID, "the running job must sort first under the default relevance order")
	assert.Equal(t, db.ScanJobStatusRunning, body.Jobs[0].Status)
}
