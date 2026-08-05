package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func deploymentOverviewTestApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/stats/deployment/pulse", GetDeploymentPulseHandler)
	app.Get("/api/v1/stats/deployment/findings", GetDeploymentFindingsHandler)
	return app
}

// TestGetDeploymentPulseHandler_Shape checks the series the chart depends on:
// a fixed bucket count, contiguous 15-minute slices, and a window that ends in
// the future so the newest bucket is the one still filling.
func TestGetDeploymentPulseHandler_Shape(t *testing.T) {
	app := deploymentOverviewTestApp()
	resp := doJSON(t, app, "GET", "/api/v1/stats/deployment/pulse", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body db.DeploymentPulse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	require.Equal(t, pulseBucketSeconds, body.BucketSeconds)
	require.Len(t, body.Buckets, pulseBucketCount)
	require.Equal(t, body.Start, body.Buckets[0].Start)
	require.True(t, body.End.After(time.Now()), "window must end after now so the current bucket is included")
	require.Equal(t,
		time.Duration(pulseBucketSeconds*pulseBucketCount)*time.Second,
		body.End.Sub(body.Start),
	)

	step := time.Duration(pulseBucketSeconds) * time.Second
	for i := 1; i < len(body.Buckets); i++ {
		require.Equal(t, step, body.Buckets[i].Start.Sub(body.Buckets[i-1].Start),
			"buckets must be contiguous at index %d", i)
	}
}

// TestGetDeploymentFindingsHandler_CrossesWorkspaces is the point of the
// endpoint: /api/v1/issues requires a workspace, this one must not.
func TestGetDeploymentFindingsHandler_CrossesWorkspaces(t *testing.T) {
	conn := db.Connection()
	before := time.Now()

	makeWorkspace := func(suffix string) uint {
		ws, err := conn.GetOrCreateWorkspace(&db.Workspace{
			Title: "Deployment Findings " + t.Name() + " " + suffix,
			Code:  "deploy_findings_" + sanitizeForCode(t.Name()) + "_" + suffix,
		})
		require.NoError(t, err)
		id := ws.ID
		t.Cleanup(func() {
			conn.DB().Where("workspace_id = ?", id).Delete(&db.Issue{})
			_ = conn.DeleteWorkspace(id)
		})
		return id
	}

	first := makeWorkspace("a")
	second := makeWorkspace("b")

	for _, spec := range []struct {
		workspace uint
		severity  string
		url       string
	}{
		{first, "Critical", "http://deploy-findings.test/a/critical"},
		{first, "High", "http://deploy-findings.test/a/high"},
		{second, "Critical", "http://deploy-findings.test/b/critical"},
	} {
		workspaceID := spec.workspace
		_, err := conn.CreateIssue(db.Issue{
			Code:        "sql_injection",
			Title:       "deployment findings " + spec.url,
			Details:     "detail " + spec.url,
			URL:         spec.url,
			StatusCode:  200,
			HTTPMethod:  "GET",
			Confidence:  90,
			Severity:    db.NewSeverity(spec.severity),
			WorkspaceID: &workspaceID,
		})
		require.NoError(t, err)
	}

	app := deploymentOverviewTestApp()
	resp := doJSON(t, app, "GET",
		"/api/v1/stats/deployment/findings?limit=50&since="+before.UTC().Format(time.RFC3339), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body db.DeploymentFindings
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	require.Equal(t, int64(3), body.Total)
	require.Equal(t, int64(2), body.Issues.Critical)
	require.Equal(t, int64(1), body.Issues.High)

	seen := map[uint]bool{}
	for _, row := range body.Data {
		seen[row.WorkspaceID] = true
		require.NotEmpty(t, row.WorkspaceTitle, "feed rows must carry the workspace they belong to")
	}
	require.True(t, seen[first] && seen[second], "feed must span every workspace")

	// Newest first — the list is read top-down as "what just happened".
	for i := 1; i < len(body.Data); i++ {
		require.False(t, body.Data[i].CreatedAt.After(body.Data[i-1].CreatedAt))
	}
}

// TestGetDeploymentFindingsHandler_RejectsBadParams keeps an unparseable
// timestamp or an unbounded limit from reaching the DB layer.
func TestGetDeploymentFindingsHandler_RejectsBadParams(t *testing.T) {
	app := deploymentOverviewTestApp()

	for _, tc := range []struct{ name, query string }{
		{"since is not a timestamp", "?since=yesterday"},
		{"limit is zero", "?limit=0"},
		{"limit is negative", "?limit=-1"},
		{"limit is above the cap", "?limit=5000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := doJSON(t, app, "GET", "/api/v1/stats/deployment/findings"+tc.query, nil)
			require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}
