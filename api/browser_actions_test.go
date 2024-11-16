package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/stretchr/testify/assert"
)

func TestCreateStoredBrowserActions(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/browser-actions", CreateStoredBrowserActions)

	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-browser-actions",
		Title:       "Test Browser Actions Workspace",
		Description: "Temporary workspace for browser actions tests",
	})
	assert.Nil(t, err)
	assert.NotNil(t, workspace)

	input := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title: "Test Actions",
			Actions: []actions.Action{
				{Type: actions.ActionClick, Selector: "#button"},
				{Type: actions.ActionFill, Selector: "#input", Value: "test"},
			},
		},
		Scope:       db.BrowserActionScopeWorkspace,
		WorkspaceID: &workspace.ID,
	}

	body, err := json.Marshal(input)
	assert.Nil(t, err)

	req := httptest.NewRequest("POST", "/api/v1/browser-actions", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var result db.StoredBrowserActions
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	assert.NotEqual(t, 0, result.ID)
	assert.Equal(t, input.Title, result.Title)
	assert.Equal(t, len(input.Actions), len(result.Actions))
	assert.Equal(t, input.Scope, result.Scope)
	assert.Equal(t, workspace.ID, *result.WorkspaceID)
}

func TestUpdateStoredBrowserActions(t *testing.T) {
	app := fiber.New()
	app.Put("/api/v1/browser-actions/:id", UpdateStoredBrowserActions)

	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-browser-actions-update",
		Title:       "Test Browser Actions Update Workspace",
		Description: "Temporary workspace for browser actions update tests",
	})
	assert.Nil(t, err)

	initialInput := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title: "Test Actions for Update",
			Actions: []actions.Action{
				{Type: actions.ActionWait, Duration: 5000},
			},
		},
		Scope:       db.BrowserActionScopeWorkspace,
		WorkspaceID: &workspace.ID,
	}

	createdSBA, err := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:       initialInput.Title,
		Actions:     initialInput.Actions,
		Scope:       initialInput.Scope,
		WorkspaceID: initialInput.WorkspaceID,
	})
	assert.Nil(t, err)

	updatedInput := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title:   "Updated Test Actions",
			Actions: []actions.Action{{Type: actions.ActionNavigate, URL: "https://example.com"}},
		},
		Scope:       db.BrowserActionScopeWorkspace,
		WorkspaceID: &workspace.ID,
	}

	body, err := json.Marshal(updatedInput)
	assert.Nil(t, err)

	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/browser-actions/%d", createdSBA.ID), strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result db.StoredBrowserActions
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, createdSBA.ID, result.ID)
	assert.Equal(t, updatedInput.Title, result.Title)
	assert.Equal(t, len(updatedInput.Actions), len(result.Actions))
	assert.Equal(t, updatedInput.Scope, result.Scope)
}

func TestCreateStoredBrowserActionsValidation(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/browser-actions", CreateStoredBrowserActions)

	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-browser-actions-validation",
		Title:       "Test Browser Actions Validation",
		Description: "Workspace for browser actions validation tests",
	})
	assert.Nil(t, err)

	testCases := []struct {
		name  string
		input BrowserActionsInput
	}{
		{
			name: "Empty title",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "", Actions: []actions.Action{{Type: "click", Selector: "#button"}}},
				Scope:          db.BrowserActionScopeGlobal,
				WorkspaceID:    &workspace.ID,
			},
		},
		{
			name: "No actions",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{}},
				Scope:          db.BrowserActionScopeGlobal,
				WorkspaceID:    &workspace.ID,
			},
		},
		{
			name: "Invalid action type",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{{Type: "invalid_type"}}},
				Scope:          db.BrowserActionScopeGlobal,
				WorkspaceID:    &workspace.ID,
			},
		},
		{
			name: "Missing required field for action",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{{Type: "click"}}},
				Scope:          db.BrowserActionScopeGlobal,
				WorkspaceID:    &workspace.ID,
			},
		},
		{
			name: "Missing workspace_id for workspace scope",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{{Type: "click", Selector: "#button"}}},
				Scope:          db.BrowserActionScopeWorkspace,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.input)
			req := httptest.NewRequest("POST", "/api/v1/browser-actions", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")

			resp, _ := app.Test(req)
			assert.NotEqual(t, fiber.StatusCreated, resp.StatusCode)
		})
	}
}

func TestUpdateStoredBrowserActionsValidation(t *testing.T) {
	app := fiber.New()
	app.Put("/api/v1/browser-actions/:id", UpdateStoredBrowserActions)

	workspace, err := db.Connection.GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-browser-actions-update-validation",
		Title:       "Test Browser Actions Update Validation",
		Description: "Workspace for browser actions update validation tests",
	})
	assert.Nil(t, err)

	validInput := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title: "Valid Actions",
			Actions: []actions.Action{
				{Type: "click", Selector: "#button"},
			},
		},
		Scope:       db.BrowserActionScopeWorkspace,
		WorkspaceID: &workspace.ID,
	}

	createdSBA, err := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:       validInput.Title,
		Actions:     validInput.Actions,
		Scope:       validInput.Scope,
		WorkspaceID: validInput.WorkspaceID,
	})
	assert.Nil(t, err)

	testCases := []struct {
		name  string
		input BrowserActionsInput
	}{
		{
			name: "Empty title",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "", Actions: []actions.Action{{Type: "click", Selector: "#button"}}},
				Scope:          db.BrowserActionScopeGlobal,
			},
		},
		{
			name: "No actions",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Updated Actions", Actions: []actions.Action{}},
				Scope:          db.BrowserActionScopeGlobal,
			},
		},
		{
			name: "Invalid action type",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Updated Actions", Actions: []actions.Action{{Type: "invalid_type"}}},
				Scope:          db.BrowserActionScopeGlobal,
			},
		},
		{
			name: "Missing required field for action",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Updated Actions", Actions: []actions.Action{{Type: "input"}}},
				Scope:          db.BrowserActionScopeGlobal,
			},
		},
		{
			name: "Missing workspace_id for workspace scope",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Updated Actions", Actions: []actions.Action{{Type: "click", Selector: "#button"}}},
				Scope:          db.BrowserActionScopeWorkspace,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.input)
			req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/browser-actions/%d", createdSBA.ID), strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")

			resp, _ := app.Test(req)
			assert.NotEqual(t, fiber.StatusOK, resp.StatusCode)
		})
	}
}
