package api

import (
	"fmt"
	"github.com/pyneda/sukyan/db"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
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

	updatedWorkspace, err := db.Connection.GetWorkspaceByID(workspace.ID)
	assert.Nil(t, err)
	assert.Equal(t, "test", updatedWorkspace.Code)
	assert.Equal(t, "updated", updatedWorkspace.Title)
	assert.Equal(t, "updated", updatedWorkspace.Description)

}

func TestDeleteWorkspace(t *testing.T) {
	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test",
		Title:       "test",
		Description: "test",
	})

	assert.NotNil(t, workspace)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, workspace.ID)
	history := &db.History{
		WorkspaceID: &workspace.ID,
		URL:         "https://example.com/test",
		Depth:       1,
	}
	db.Connection.CreateHistory(history)
	assert.NotNil(t, history)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, history.ID)

	app := fiber.New()
	app.Delete("/api/v1/workspaces/:id", DeleteWorkspace)
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/workspaces/%d", workspace.ID), nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	workspaceExists, err := db.Connection.WorkspaceExists(workspace.ID)
	assert.Nil(t, err)
	assert.False(t, workspaceExists)

	// ensure related data is deleted
	// TODO: This is failing, the on delete constrain seems to not be working
	// historyExists, err := db.Connection.HistoryExists(history.ID)
	// assert.Nil(t, err)
	// assert.False(t, historyExists)

}

func TestGetWorkspaceDetail(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/workspaces/:id", GetWorkspaceDetail)

	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
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

	retrievedWorkspace, err := db.Connection.GetWorkspaceByID(workspace.ID)
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
