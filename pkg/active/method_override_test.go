package active

import (
	"net/http"
	"net/http/httptest"
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
			TaskID:      1,
		},
	})
	require.NoError(t, baseResult.Err)
	require.NotNil(t, baseResult.History)

	MethodOverrideScan(baseResult.History, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		TaskID:      1,
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
			TaskID:      2,
		},
	})
	require.NoError(t, baseResult.Err)
	require.NotNil(t, baseResult.History)

	MethodOverrideScan(baseResult.History, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		TaskID:      2,
		ScanMode:    scanopts.ScanModeSmart,
		HTTPClient:  http.DefaultClient,
	})

	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", db.HttpMethodOverrideCode).Count(&count)
	require.Equal(t, int64(0), count, "should not report for non-GET baseline")
}
