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

func setupTestApp() *fiber.App {
	app := fiber.New()
	return app
}

func TestCreateStoredBrowserActions(t *testing.T) {
	app := setupTestApp()
	app.Post("/api/v1/browser-actions", CreateStoredBrowserActions)

	input := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title: "Test Actions",
			Actions: []actions.Action{
				{Type: actions.ActionClick, Selector: "#button"},
				{Type: actions.ActionFill, Selector: "#input", Value: "test"},
			},
		},
		Scope:       db.BrowserActionScopeWorkspace,
		WorkspaceID: new(uint),
	}
	*input.WorkspaceID = 1

	body, _ := json.Marshal(input)
	req := httptest.NewRequest("POST", "/api/v1/browser-actions", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var result db.StoredBrowserActions
	json.NewDecoder(resp.Body).Decode(&result)

	assert.NotEqual(t, 0, result.ID)
	assert.Equal(t, input.Title, result.Title)
	assert.Equal(t, len(input.Actions), len(result.Actions))
	assert.Equal(t, input.Scope, result.Scope)
	assert.Equal(t, input.WorkspaceID, result.WorkspaceID)
}

func TestUpdateStoredBrowserActions(t *testing.T) {
	app := setupTestApp()
	app.Put("/api/v1/browser-actions/:id", UpdateStoredBrowserActions)

	// First, create a StoredBrowserActions to update
	initialInput := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title: "Test Actions for Update",
			Actions: []actions.Action{
				{Type: actions.ActionWait, Duration: 5000},
			},
		},
		Scope: db.BrowserActionScopeGlobal,
	}
	createdSBA, err := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:   initialInput.Title,
		Actions: initialInput.Actions,
		Scope:   initialInput.Scope,
	})
	assert.Nil(t, err)

	// Now, update it
	updatedInput := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title:   "Updated Test Actions",
			Actions: []actions.Action{{Type: actions.ActionNavigate, URL: "https://example.com"}},
		},
		Scope: db.BrowserActionScopeGlobal,
	}

	body, _ := json.Marshal(updatedInput)
	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/browser-actions/%d", createdSBA.ID), strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result db.StoredBrowserActions
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Equal(t, createdSBA.ID, result.ID)
	assert.Equal(t, updatedInput.Title, result.Title)
	assert.Equal(t, len(updatedInput.Actions), len(result.Actions))
	assert.Equal(t, updatedInput.Scope, result.Scope)
}

func TestCreateStoredBrowserActionsValidation(t *testing.T) {
	app := setupTestApp()
	app.Post("/api/v1/browser-actions", CreateStoredBrowserActions)

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
				BrowserActions: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{}},
				Scope:          db.BrowserActionScopeGlobal,
			},
		},
		{
			name: "Invalid action type",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{{Type: "invalid_type"}}},
				Scope:          db.BrowserActionScopeGlobal,
			},
		},
		{
			name: "Missing required field for action",
			input: BrowserActionsInput{
				BrowserActions: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{{Type: "click"}}},
				Scope:          db.BrowserActionScopeGlobal,
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

			assert.NotEqual(t, fiber.StatusCreated, resp.StatusCode, "Expected an error response")
		})
	}
}

func TestUpdateStoredBrowserActionsValidation(t *testing.T) {
	app := setupTestApp()
	app.Put("/api/v1/browser-actions/:id", UpdateStoredBrowserActions)

	// Create a valid StoredBrowserActions to update
	validInput := BrowserActionsInput{
		BrowserActions: actions.BrowserActions{
			Title: "Valid Actions",
			Actions: []actions.Action{
				{Type: "click", Selector: "#button"},
			},
		},
		Scope: db.BrowserActionScopeGlobal,
	}
	createdSBA, _ := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:   validInput.Title,
		Actions: validInput.Actions,
		Scope:   validInput.Scope,
	})

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

			assert.NotEqual(t, fiber.StatusOK, resp.StatusCode, "Expected an error response")
		})
	}
}
