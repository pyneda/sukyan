package active

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	scanopts "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/stretchr/testify/require"
)

func TestMassAssignmentScanDetectsAddedFields(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "ma-detect", Title: "ma-detect"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		// Echo back the parsed JSON so presence of privileged fields is observable.
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		resp, _ := json.Marshal(payload)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(resp)
	}))
	defer srv.Close()

	cleanupIssues(t, db.ApiMassAssignmentCode)

	basePayload := []byte(`{"name":"tester"}`)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/users", bytes.NewReader(basePayload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	baseResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspace.ID,
			TaskID:      10,
		},
	})
	require.NoError(t, baseResult.Err)
	require.NotNil(t, baseResult.History)

	MassAssignmentScan(baseResult.History, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		TaskID:      10,
		ScanMode:    scanopts.ScanModeSmart,
		HTTPClient:  http.DefaultClient,
	})

	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", db.ApiMassAssignmentCode).Count(&count)
	require.Greater(t, count, int64(0), "expected mass assignment issue")
}

func TestMassAssignmentScanSkipsNonJSON(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{Code: "ma-skip", Title: "ma-skip"})
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cleanupIssues(t, db.ApiMassAssignmentCode)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/submit", bytes.NewReader([]byte("name=test")))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	baseResult := http_utils.ExecuteRequest(req, http_utils.RequestExecutionOptions{
		CreateHistory: true,
		HistoryCreationOptions: http_utils.HistoryCreationOptions{
			Source:      db.SourceScanner,
			WorkspaceID: workspace.ID,
			TaskID:      11,
		},
	})
	require.NoError(t, baseResult.Err)
	require.NotNil(t, baseResult.History)

	MassAssignmentScan(baseResult.History, ActiveModuleOptions{
		WorkspaceID: workspace.ID,
		TaskID:      11,
		ScanMode:    scanopts.ScanModeSmart,
		HTTPClient:  http.DefaultClient,
	})

	var count int64
	db.Connection().DB().Model(&db.Issue{}).Where("code = ?", db.ApiMassAssignmentCode).Count(&count)
	require.Equal(t, int64(0), count, "should skip non-JSON baselines")
}
