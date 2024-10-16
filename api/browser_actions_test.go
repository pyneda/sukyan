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

	input := actions.BrowserActions{
		Title: "Test Actions",
		Actions: []actions.Action{
			{Type: actions.ActionClick, Selector: "#button"},
			{Type: actions.ActionFill, Selector: "#input", Value: "test"},
		},
	}

	body, _ := json.Marshal(input)
	req := httptest.NewRequest("POST", "/api/v1/browser-actions?workspace_id=1&scope=workspace", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	resp, _ := app.Test(req)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)

	var result db.StoredBrowserActions
	json.NewDecoder(resp.Body).Decode(&result)

	assert.NotEqual(t, 0, result.ID)
	assert.Equal(t, input.Title, result.Title)
	assert.Equal(t, len(input.Actions), len(result.Actions))
}

func TestGetStoredBrowserActions(t *testing.T) {
	app := setupTestApp()
	app.Get("/api/v1/browser-actions/:id", GetStoredBrowserActions)

	// First, create a StoredBrowserActions to retrieve
	input := actions.BrowserActions{
		Title: "Test Actions for Get",
		Actions: []actions.Action{
			{Type: actions.ActionNavigate, URL: "https://example.com"},
		},
	}
	createdSBA, err := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:   input.Title,
		Actions: input.Actions,
	})
	assert.Nil(t, err)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-actions/%d", createdSBA.ID), nil)

	resp, _ := app.Test(req)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result db.StoredBrowserActions
	json.NewDecoder(resp.Body).Decode(&result)

	assert.Equal(t, createdSBA.ID, result.ID)
	assert.Equal(t, input.Title, result.Title)
}

func TestUpdateStoredBrowserActions(t *testing.T) {
	app := setupTestApp()
	app.Put("/api/v1/browser-actions/:id", UpdateStoredBrowserActions)

	// First, create a StoredBrowserActions to update
	initialInput := actions.BrowserActions{
		Title: "Test Actions for Update",
		Actions: []actions.Action{
			{Type: actions.ActionWait, Duration: 5000},
		},
	}
	createdSBA, err := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:   initialInput.Title,
		Actions: initialInput.Actions,
	})
	assert.Nil(t, err)

	// Now, update it
	updatedInput := actions.BrowserActions{
		Title: "Updated Test Actions",
		Actions: []actions.Action{
			{Type: actions.ActionNavigate, URL: "https://example.com"},
		},
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
}

func TestDeleteStoredBrowserActions(t *testing.T) {
	app := setupTestApp()
	app.Delete("/api/v1/browser-actions/:id", DeleteStoredBrowserActions)

	// First, create a StoredBrowserActions to delete
	input := actions.BrowserActions{
		Title: "Test Actions for Delete",
		Actions: []actions.Action{
			{Type: actions.ActionNavigate, URL: "https://example.com"},
		},
	}
	createdSBA, err := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:   input.Title,
		Actions: input.Actions,
	})
	assert.Nil(t, err)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/browser-actions/%d", createdSBA.ID), nil)

	resp, _ := app.Test(req)

	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)

	// Verify that it's been deleted
	_, err = db.Connection.GetStoredBrowserActionsByID(createdSBA.ID)
	assert.NotNil(t, err) // Should return an error as it's not found
}

func TestListStoredBrowserActions(t *testing.T) {
	app := setupTestApp()
	app.Get("/api/v1/browser-actions", ListStoredBrowserActions)

	// First, create some StoredBrowserActions to list
	for i := 0; i < 3; i++ {
		input := actions.BrowserActions{
			Title: fmt.Sprintf("Test Actions %d", i),
			Actions: []actions.Action{
				{Type: actions.ActionClick, Selector: "#button"},
				{Type: actions.ActionFill, Selector: "#input", Value: fmt.Sprintf("test%d", i)},
			},
		}
		_, err := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
			Title:   input.Title,
			Actions: input.Actions,
		})
		assert.Nil(t, err)
	}

	req := httptest.NewRequest("GET", "/api/v1/browser-actions?page=1&page_size=10", nil)

	resp, _ := app.Test(req)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result struct {
		Data  []db.StoredBrowserActions `json:"data"`
		Count int64                     `json:"count"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	assert.GreaterOrEqual(t, len(result.Data), 3)
	assert.GreaterOrEqual(t, result.Count, int64(3))
}

func TestCreateStoredBrowserActionsValidation(t *testing.T) {
	app := setupTestApp()
	app.Post("/api/v1/browser-actions", CreateStoredBrowserActions)

	testCases := []struct {
		name  string
		input actions.BrowserActions
	}{
		{
			name:  "Empty title",
			input: actions.BrowserActions{Title: "", Actions: []actions.Action{{Type: "click", Selector: "#button"}}},
		},
		{
			name:  "No actions",
			input: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{}},
		},
		{
			name:  "Invalid action type",
			input: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{{Type: "invalid_type"}}},
		},
		{
			name:  "Missing required field for action",
			input: actions.BrowserActions{Title: "Test Actions", Actions: []actions.Action{{Type: "click"}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(tc.input)
			req := httptest.NewRequest("POST", "/api/v1/browser-actions?workspace_id=1&scope=workspace", strings.NewReader(string(body)))
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
	validInput := actions.BrowserActions{
		Title: "Valid Actions",
		Actions: []actions.Action{
			{Type: "click", Selector: "#button"},
		},
	}
	createdSBA, _ := db.Connection.CreateStoredBrowserActions(&db.StoredBrowserActions{
		Title:   validInput.Title,
		Actions: validInput.Actions,
	})

	testCases := []struct {
		name  string
		input actions.BrowserActions
	}{
		{
			name:  "Empty title",
			input: actions.BrowserActions{Title: "", Actions: []actions.Action{{Type: "click", Selector: "#button"}}},
		},
		{
			name:  "No actions",
			input: actions.BrowserActions{Title: "Updated Actions", Actions: []actions.Action{}},
		},
		{
			name:  "Invalid action type",
			input: actions.BrowserActions{Title: "Updated Actions", Actions: []actions.Action{{Type: "invalid_type"}}},
		},
		{
			name:  "Missing required field for action",
			input: actions.BrowserActions{Title: "Updated Actions", Actions: []actions.Action{{Type: "input"}}},
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

func TestListStoredBrowserActionsValidation(t *testing.T) {
	app := setupTestApp()
	app.Get("/api/v1/browser-actions", ListStoredBrowserActions)

	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "Invalid page number",
			query: "page=invalid&page_size=10",
		},
		{
			name:  "Invalid page size",
			query: "page=1&page_size=invalid",
		},
		{
			name:  "Invalid scope",
			query: "scope=invalid",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/browser-actions?"+tc.query, nil)

			resp, _ := app.Test(req)

			assert.NotEqual(t, fiber.StatusOK, resp.StatusCode, "Expected an error response")
		})
	}
}
