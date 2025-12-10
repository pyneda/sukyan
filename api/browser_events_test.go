package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindBrowserEvents(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/browser-events", FindBrowserEvents)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-api-browser-events-find",
		Title:       "Test API Browser Events Find Workspace",
		Description: "Temporary workspace for API browser events find tests",
	})
	require.Nil(t, err)

	// Create a scan
	scan := &db.Scan{
		WorkspaceID: workspace.ID,
		Status:      db.ScanStatusCompleted,
		Title:       "API Test Scan",
	}
	scan, err = db.Connection().CreateScan(scan)
	require.Nil(t, err)

	// Create test events
	events := []*db.BrowserEvent{
		{
			EventType:   db.BrowserEventConsole,
			Category:    db.BrowserEventCategoryRuntime,
			URL:         "https://example.com/api-test-1",
			Data:        []byte(`{"msg": "console log 1"}`),
			WorkspaceID: workspace.ID,
			ScanID:      &scan.ID,
			Source:      "crawler",
		},
		{
			EventType:   db.BrowserEventSecurity,
			Category:    db.BrowserEventCategorySecurity,
			URL:         "https://example.com/api-test-2",
			Data:        []byte(`{"issue": "cors"}`),
			WorkspaceID: workspace.ID,
			ScanID:      &scan.ID,
			Source:      "audit",
		},
		{
			EventType:   db.BrowserEventDOMStorage,
			Category:    db.BrowserEventCategoryStorage,
			URL:         "https://example.com/api-test-3",
			Data:        []byte(`{"key": "session"}`),
			WorkspaceID: workspace.ID,
			ScanID:      &scan.ID,
			Source:      "crawler",
		},
	}

	for _, event := range events {
		err = db.Connection().SaveBrowserEvent(event)
		require.Nil(t, err)
	}

	// Test basic request with workspace
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d", workspace.ID), nil)
	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	assert.NotNil(t, result["data"])
	assert.NotNil(t, result["count"])
	count := result["count"].(float64)
	assert.GreaterOrEqual(t, int(count), 3)

	// Test with scan_id filter
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&scan_id=%d", workspace.ID, scan.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with event_types filter
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&event_types=console", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	data := result["data"].([]interface{})
	for _, item := range data {
		eventMap := item.(map[string]interface{})
		assert.Equal(t, "console", eventMap["event_type"])
	}

	// Test with categories filter
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&categories=security", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	data = result["data"].([]interface{})
	for _, item := range data {
		eventMap := item.(map[string]interface{})
		assert.Equal(t, "security", eventMap["category"])
	}

	// Test with sources filter
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&sources=crawler", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test with URL filter
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&url=api-test", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Test pagination
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&page=1&page_size=2", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	data = result["data"].([]interface{})
	assert.LessOrEqual(t, len(data), 2)

	// Test sorting
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&sort_by=event_type&sort_order=asc", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestFindBrowserEventsValidation(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/browser-events", FindBrowserEvents)

	// Test without workspace (required)
	req := httptest.NewRequest("GET", "/api/v1/browser-events", nil)
	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test with invalid workspace
	req = httptest.NewRequest("GET", "/api/v1/browser-events?workspace=invalid", nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test with non-existent workspace
	req = httptest.NewRequest("GET", "/api/v1/browser-events?workspace=999999", nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test with invalid page_size
	workspace, _ := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:  "test-api-browser-events-validation",
		Title: "Test Validation Workspace",
	})
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&page_size=invalid", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test with invalid page
	req = httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events?workspace=%d&page=invalid", workspace.ID), nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetBrowserEventByID(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/browser-events/:id", GetBrowserEventByID)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-api-browser-events-get",
		Title:       "Test API Browser Events Get Workspace",
		Description: "Temporary workspace for API browser events get tests",
	})
	require.Nil(t, err)

	// Create a test event with unique URL to avoid aggregation with previous test runs
	uniqueURL := fmt.Sprintf("https://example.com/get-test-%d", time.Now().UnixNano())
	event := &db.BrowserEvent{
		EventType:   db.BrowserEventConsole,
		Category:    db.BrowserEventCategoryRuntime,
		URL:         uniqueURL,
		Description: "Test event for get by ID",
		Data:        []byte(`{"msg": "test message"}`),
		WorkspaceID: workspace.ID,
		Source:      "test",
	}
	err = db.Connection().SaveBrowserEvent(event)
	require.Nil(t, err)
	require.NotEqual(t, event.ID.String(), "00000000-0000-0000-0000-000000000000", "Event ID should be populated after save")

	// Test valid ID
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events/%s", event.ID.String()), nil)
	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result BrowserEventResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	assert.Equal(t, event.ID, result.ID)
	assert.Equal(t, string(event.EventType), result.EventType)
	assert.Equal(t, string(event.Category), result.Category)
	assert.Equal(t, uniqueURL, result.URL)

	// Test invalid UUID format
	req = httptest.NewRequest("GET", "/api/v1/browser-events/invalid-uuid", nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test non-existent UUID
	req = httptest.NewRequest("GET", "/api/v1/browser-events/00000000-0000-0000-0000-000000000000", nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetBrowserEventStats(t *testing.T) {
	app := fiber.New()
	app.Get("/api/v1/browser-events/stats", GetBrowserEventStats)

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Code:        "test-api-browser-events-stats",
		Title:       "Test API Browser Events Stats Workspace",
		Description: "Temporary workspace for API browser events stats tests",
	})
	require.Nil(t, err)

	// Create a scan
	scan := &db.Scan{
		WorkspaceID: workspace.ID,
		Status:      db.ScanStatusCompleted,
		Title:       "API Stats Test Scan",
	}
	scan, err = db.Connection().CreateScan(scan)
	require.Nil(t, err)

	// Create test events with different types
	events := []*db.BrowserEvent{
		{EventType: db.BrowserEventConsole, Category: db.BrowserEventCategoryRuntime, URL: "https://example.com/stats-1", Data: []byte(`{"a":1}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
		{EventType: db.BrowserEventConsole, Category: db.BrowserEventCategoryRuntime, URL: "https://example.com/stats-2", Data: []byte(`{"a":2}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
		{EventType: db.BrowserEventSecurity, Category: db.BrowserEventCategorySecurity, URL: "https://example.com/stats-3", Data: []byte(`{"a":3}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
		{EventType: db.BrowserEventDOMStorage, Category: db.BrowserEventCategoryStorage, URL: "https://example.com/stats-4", Data: []byte(`{"a":4}`), WorkspaceID: workspace.ID, ScanID: &scan.ID, Source: "test"},
	}

	for _, event := range events {
		err = db.Connection().SaveBrowserEvent(event)
		require.Nil(t, err)
	}

	// Test valid request
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/browser-events/stats?scan_id=%d", scan.ID), nil)
	resp, err := app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	assert.Nil(t, err)
	assert.NotNil(t, result["total_count"])
	assert.NotNil(t, result["by_event_type"])
	assert.NotNil(t, result["by_category"])

	totalCount := result["total_count"].(float64)
	assert.GreaterOrEqual(t, int(totalCount), 4)

	byEventType := result["by_event_type"].(map[string]interface{})
	assert.GreaterOrEqual(t, int(byEventType["console"].(float64)), 2)

	byCategory := result["by_category"].(map[string]interface{})
	assert.GreaterOrEqual(t, int(byCategory["runtime"].(float64)), 2)

	// Test without scan_id (required)
	req = httptest.NewRequest("GET", "/api/v1/browser-events/stats", nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	// Test with invalid scan_id
	req = httptest.NewRequest("GET", "/api/v1/browser-events/stats?scan_id=invalid", nil)
	resp, err = app.Test(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestGetValidBrowserEventTypes(t *testing.T) {
	types := GetValidBrowserEventTypes()
	assert.Contains(t, types, "console")
	assert.Contains(t, types, "dialog")
	assert.Contains(t, types, "dom_storage")
	assert.Contains(t, types, "security")
	assert.Contains(t, types, "certificate")
	assert.Contains(t, types, "audit")
	assert.Contains(t, types, "indexeddb")
	assert.Contains(t, types, "cache_storage")
	assert.Contains(t, types, "background_service")
	assert.Contains(t, types, "database")
	assert.Contains(t, types, "network_auth")
	assert.Len(t, types, 11)
}

func TestGetValidBrowserEventCategories(t *testing.T) {
	categories := GetValidBrowserEventCategories()
	assert.Contains(t, categories, "runtime")
	assert.Contains(t, categories, "storage")
	assert.Contains(t, categories, "security")
	assert.Contains(t, categories, "network")
	assert.Contains(t, categories, "audit")
	assert.Len(t, categories, 5)
}
