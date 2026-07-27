package active

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/stretchr/testify/require"
)

// isBypassProbeRequest reports whether a request carries one of the header
// names ForbiddenBypassScan injects for its header-based bypass combinations.
// It's used by tests to tell an actual bypass probe apart from the plain
// control replay, which carries none of these.
func isBypassProbeRequest(r *http.Request) bool {
	for _, group := range [][]HeaderTest{ipBasedHeaders, urlBasedHeaders, portBasedHeaders, pathBasedHeaders} {
		for _, ht := range group {
			if r.Header.Get(ht.HeaderName) != "" {
				return true
			}
		}
	}
	return false
}

func captureBaselineHistory(t *testing.T, workspaceID uint, method, targetURL string) *db.History {
	t.Helper()
	req, err := http.NewRequest(method, targetURL, nil)
	require.NoError(t, err)

	result := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspaceID,
		},
	})
	require.NoError(t, result.Err)
	require.NotNil(t, result.History)
	return result.History
}

func TestForbiddenBypassScanStaleBaselineNoIssue(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "bypass-stale", Title: "bypass-stale"})
	require.NoError(t, err)

	var open sync.Mutex
	serverOpen := false
	var controlLikeRequests int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		open.Lock()
		isOpen := serverOpen
		open.Unlock()

		if !isOpen {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.URL.Path == "/account" && !isBypassProbeRequest(r) {
			open.Lock()
			controlLikeRequests++
			open.Unlock()
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cleanupIssues(t, db.ForbiddenBypassCode)

	baseline := captureBaselineHistory(t, workspace.ID, http.MethodGet, srv.URL+"/account")
	require.Equal(t, http.StatusUnauthorized, baseline.StatusCode)

	// Simulate the endpoint becoming reachable after the crawl captured the
	// 401 baseline (e.g. the account got created moments later).
	open.Lock()
	serverOpen = true
	open.Unlock()

	ForbiddenBypassScan(baseline, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		Concurrency: 5,
		HTTPClient:  http.DefaultClient,
	})

	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", db.ForbiddenBypassCode).Count(&count)
	require.Equal(t, int64(0), count, "stale baseline must not be reported as a bypass")

	open.Lock()
	defer open.Unlock()
	require.Equal(t, 1, controlLikeRequests, "expected exactly one control request re-validating the unmodified baseline")
}

func TestForbiddenBypassScanGenuineBypassCreatesOneIssue(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "bypass-genuine", Title: "bypass-genuine"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") == "127.0.0.1" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cleanupIssues(t, db.ForbiddenBypassCode)

	baseline := captureBaselineHistory(t, workspace.ID, http.MethodGet, srv.URL+"/admin")
	require.Equal(t, http.StatusForbidden, baseline.StatusCode)

	ForbiddenBypassScan(baseline, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		Concurrency: 5,
		HTTPClient:  http.DefaultClient,
	})

	var issues []db.Issue
	db.Connection().DB().Where("code = ?", db.ForbiddenBypassCode).Find(&issues)
	require.Len(t, issues, 1, "expected exactly one consolidated issue")
	require.Equal(t, 90, issues[0].Confidence)
}

func TestForbiddenBypassScanMultipleTechniquesStillOneIssue(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "bypass-multi", Title: "bypass-multi"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Forwarded-For") == "127.0.0.1" || r.Header.Get("X-Real-IP") == "127.0.0.1" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cleanupIssues(t, db.ForbiddenBypassCode)

	baseline := captureBaselineHistory(t, workspace.ID, http.MethodGet, srv.URL+"/admin")
	require.Equal(t, http.StatusForbidden, baseline.StatusCode)

	ForbiddenBypassScan(baseline, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		Concurrency: 5,
		HTTPClient:  http.DefaultClient,
	})

	var issues []db.Issue
	db.Connection().DB().Where("code = ?", db.ForbiddenBypassCode).Find(&issues)
	require.Len(t, issues, 1, "expected exactly one consolidated issue even with multiple working techniques")

	var requestCount int64
	db.Connection().DB().Table("issue_requests").Where("issue_id = ?", issues[0].ID).Count(&requestCount)
	require.GreaterOrEqual(t, requestCount, int64(2), "expected the supporting histories from both techniques to be linked")
}

// Scans are cancelled routinely when a job hits its MaxDuration. A bypass the
// control already confirmed must survive that: reporting has to happen on every
// exit path, not only the one where every combination ran.
func TestForbiddenBypassScanReportsFindingsWhenCancelled(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "bypass-cancel", Title: "bypass-cancel"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var mu sync.Mutex
	sawBypass, sawControl, cancelled := false, false, false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bypassed := r.Header.Get("X-Forwarded-For") == "127.0.0.1"

		mu.Lock()
		if bypassed {
			sawBypass = true
		} else if !isBypassProbeRequest(r) {
			sawControl = true
		}
		ready := sawBypass && sawControl && !cancelled
		if ready {
			cancelled = true
		}
		mu.Unlock()

		if ready {
			// Let the confirmed finding land in the shared state, then cancel
			// so the scan takes one of its early-return paths.
			go func() {
				time.Sleep(200 * time.Millisecond)
				cancel()
			}()
		}

		if bypassed {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	cleanupIssues(t, db.ForbiddenBypassCode)

	baseline := captureBaselineHistory(t, workspace.ID, http.MethodGet, srv.URL+"/admin")
	require.Equal(t, http.StatusForbidden, baseline.StatusCode)

	ForbiddenBypassScan(baseline, ActiveModuleOptions{
		Ctx:         ctx,
		WorkspaceID: workspace.ID,
		Concurrency: 5,
		HTTPClient:  http.DefaultClient,
	})

	var issues []db.Issue
	db.Connection().DB().Where("code = ?", db.ForbiddenBypassCode).Find(&issues)
	require.Len(t, issues, 1, "a control-confirmed bypass must still be reported when the scan is cancelled")
}
