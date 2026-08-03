package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTransferWorkspace(t *testing.T) *db.Workspace {
	t.Helper()
	conn := db.Connection()

	workspace, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Code:  fmt.Sprintf("api-transfer-%d", db.Connection().DB().Config.NowFunc().UnixNano()),
		Title: "API transfer test",
	})
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		_, err := conn.CreateHistory(&db.History{
			URL:         fmt.Sprintf("http://example.com/api-transfer/%d", i),
			StatusCode:  200,
			Method:      "GET",
			RawRequest:  []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"),
			RawResponse: []byte("HTTP/1.1 200 OK\r\n\r\nbody"),
			WorkspaceID: &workspace.ID,
			Source:      db.SourceScanner,
		})
		require.NoError(t, err)
	}
	return workspace
}

// Fiber only exposes a body stream once the upload outgrows its buffer, so a
// handler that reads only the stream rejects every ordinary upload. Exercising
// export and import over real HTTP is the only way that surfaces.
func TestWorkspaceExportImportOverHTTP(t *testing.T) {
	source := seedTransferWorkspace(t)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), source.ID, db.WorkspaceDeleteOptions{})
	})

	app := fiber.New()
	app.Get("/api/v1/workspaces/:id/export", ExportWorkspace)
	app.Post("/api/v1/workspaces/import", ImportWorkspace)

	exportResp, err := app.Test(
		httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%d/export", source.ID), nil),
		30000,
	)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, exportResp.StatusCode)
	assert.Equal(t, "application/zstd", exportResp.Header.Get("Content-Type"))
	assert.Contains(t, exportResp.Header.Get("Content-Disposition"), ".sukyan.zst")

	archive, err := io.ReadAll(exportResp.Body)
	require.NoError(t, err)
	require.NotEmpty(t, archive, "export streamed an empty archive")

	importReq := httptest.NewRequest("POST", "/api/v1/workspaces/import", bytes.NewReader(archive))
	importReq.Header.Set("Content-Type", "application/zstd")
	importResp, err := app.Test(importReq, 60000)
	require.NoError(t, err)
	require.Equal(t, fiber.StatusCreated, importResp.StatusCode, "import rejected a body produced by export")

	var result struct {
		WorkspaceID uint  `json:"workspace_id"`
		TotalRows   int64 `json:"total_rows"`
	}
	require.NoError(t, json.NewDecoder(importResp.Body).Decode(&result))
	require.NotZero(t, result.WorkspaceID)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), result.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	assert.NotEqual(t, source.ID, result.WorkspaceID)

	var copied int64
	require.NoError(t, db.Connection().DB().
		Raw("SELECT count(*) FROM histories WHERE workspace_id = ?", result.WorkspaceID).
		Scan(&copied).Error)
	assert.EqualValues(t, 3, copied, "the imported workspace should hold the same history rows")
}

func TestWorkspaceImportRejectsEmptyBody(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/workspaces/import", ImportWorkspace)

	resp, err := app.Test(httptest.NewRequest("POST", "/api/v1/workspaces/import", nil), 10000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestWorkspaceExportRejectsUnknownWorkspace(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/workspaces/:id/export", ExportWorkspace)

	resp, err := app.Test(httptest.NewRequest("GET", "/api/v1/workspaces/999999999/export", nil), 10000)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}
