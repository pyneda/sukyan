package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
)

func TestFindWorkspaces(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/workspaces", FindWorkspaces)

	req := httptest.NewRequest("GET", "/api/v1/workspaces", nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestUpdateWorkspace(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test",
		Title:       "test",
		Description: "test",
	})
	assert.NotNil(t, workspace)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, workspace.ID)
	workspace.Description = "updated"
	workspace.Title = "updated"
	app := fiber.New()
	app.Put("/api/v1/workspaces/:id", UpdateWorkspace)
	updateData := `{"code": "test", "title": "updated", "description": "updated"}`
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/workspaces/%d", workspace.ID), strings.NewReader(updateData))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := app.Test(req)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	updatedWorkspace, err := db.Connection().GetWorkspaceByID(workspace.ID)
	assert.Nil(t, err)
	assert.Equal(t, "test", updatedWorkspace.Code)
	assert.Equal(t, "updated", updatedWorkspace.Title)
	assert.Equal(t, "updated", updatedWorkspace.Description)

}

func TestDeleteWorkspace(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "TestDeleteWorkspace",
		Title:       "TestDeleteWorkspace",
		Description: "TestDeleteWorkspace",
	})

	assert.NotNil(t, workspace)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, workspace.ID)
	history := &db.History{
		WorkspaceID: &workspace.ID,
		URL:         "https://example.com/test",
		Depth:       1,
	}
	db.Connection().CreateHistory(history)
	assert.NotNil(t, history)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, history.ID)

	app := fiber.New()
	app.Delete("/api/v1/workspaces/:id", DeleteWorkspace)
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%d", workspace.ID), nil)
	resp, err := app.Test(req, 10000) // 10 second timeout

	assert.Nil(t, err)
	assert.NotNil(t, resp, "Response should not be nil")

	// Purging runs in the background, so the request only acknowledges it.
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// The workspace leaves every listing straight away, before its rows are gone.
	workspaceExists, err := db.Connection().WorkspaceExists(workspace.ID)
	assert.Nil(t, err)
	assert.False(t, workspaceExists)

	assert.Eventually(t, func() bool {
		var remaining int64
		db.Connection().DB().Raw("SELECT count(*) FROM histories WHERE workspace_id = ?", workspace.ID).Scan(&remaining)
		return remaining == 0
	}, 30*time.Second, 100*time.Millisecond, "the background purge should remove the workspace's history")

	var workspaceRows int64
	db.Connection().DB().Raw("SELECT count(*) FROM workspaces WHERE id = ?", workspace.ID).Scan(&workspaceRows)
	assert.Zero(t, workspaceRows, "the workspace row itself should be hard deleted once the purge finishes")
}

func TestGetWorkspaceDetail(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/workspaces/:id", GetWorkspaceDetail)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-get",
		Title:       "test-get",
		Description: "test-get",
	})
	assert.NotNil(t, workspace)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, workspace.ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/workspaces/%d", workspace.ID), nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	retrievedWorkspace, err := db.Connection().GetWorkspaceByID(workspace.ID)
	assert.Nil(t, err)
	assert.Equal(t, "test-get", retrievedWorkspace.Code)
	assert.Equal(t, "test-get", retrievedWorkspace.Title)
	assert.Equal(t, "test-get", retrievedWorkspace.Description)
}

func TestGetWorkspaceDetailInvalidIDFormat(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/workspaces/:id", GetWorkspaceDetail)
	req := httptest.NewRequest("GET", "/api/v1/workspaces/abc", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, fiber.StatusUnprocessableEntity, resp.StatusCode)
}

func TestGetWorkspaceDetailNonExistentID(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/workspaces/:id", GetWorkspaceDetail)
	req := httptest.NewRequest("GET", "/api/v1/workspaces/99999", nil)
	resp, _ := app.Test(req)
	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
}
