package active

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	scanopts "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/stretchr/testify/require"
)

func TestMethodOverrideScanDetectsHeaderOverride(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "mo-detect", Title: "mo-detect"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-HTTP-Method-Override") == "DELETE" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cleanupIssues(t, db.HttpMethodOverrideCode)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/item", nil)
	require.NoError(t, err)

	baseResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspace.ID,
			TaskID:      0,
		},
	})
	require.NoError(t, baseResult.Err)
	require.NotNil(t, baseResult.History)

	MethodOverrideScan(baseResult.History, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		TaskID:      0,
		ScanMode:    scanopts.ScanModeSmart,
		HTTPClient:  http.DefaultClient,
	})

	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", db.HttpMethodOverrideCode).Count(&count)
	require.Greater(t, count, int64(0), "expected method override issue")
}

func TestMethodOverrideScanSkipsNonGET(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "mo-skip", Title: "mo-skip"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cleanupIssues(t, db.HttpMethodOverrideCode)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/submit", nil)
	require.NoError(t, err)

	baseResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspace.ID,
			TaskID:      0,
		},
	})
	require.NoError(t, baseResult.Err)
	require.NotNil(t, baseResult.History)

	MethodOverrideScan(baseResult.History, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		TaskID:      0,
		ScanMode:    scanopts.ScanModeSmart,
		HTTPClient:  http.DefaultClient,
	})

	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", db.HttpMethodOverrideCode).Count(&count)
	require.Equal(t, int64(0), count, "should not report for non-GET baseline")
}

func TestMethodOverrideScanStaleBaselineNoIssue(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "mo-stale", Title: "mo-stale"})
	require.NoError(t, err)

	overrideHeaders := []string{"X-HTTP-Method-Override", "X-Method-Override", "X-HTTP-Method"}

	var mu sync.Mutex
	open := false
	controlLikeRequests := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		isOpen := open
		mu.Unlock()

		if !isOpen {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		hasOverrideHeader := false
		for _, h := range overrideHeaders {
			if r.Header.Get(h) != "" {
				hasOverrideHeader = true
				break
			}
		}
		if r.URL.RawQuery == "" && !hasOverrideHeader {
			mu.Lock()
			controlLikeRequests++
			mu.Unlock()
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cleanupIssues(t, db.HttpMethodOverrideCode)

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/item", nil)
	require.NoError(t, err)

	baseResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspace.ID,
		},
	})
	require.NoError(t, baseResult.Err)
	require.Equal(t, http.StatusUnauthorized, baseResult.History.StatusCode)

	// Simulate the endpoint becoming reachable after the crawl captured the
	// 401 baseline.
	mu.Lock()
	open = true
	mu.Unlock()

	MethodOverrideScan(baseResult.History, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		ScanMode:    scanopts.ScanModeSmart,
		HTTPClient:  http.DefaultClient,
	})

	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", db.HttpMethodOverrideCode).Count(&count)
	require.Equal(t, int64(0), count, "stale baseline must not be reported as a method override bypass")

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 1, controlLikeRequests, "expected exactly one control request re-validating the unmodified baseline")
}
