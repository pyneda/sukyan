package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestFindHistory(t *testing.T) {
	workspace, err := db.Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)
	app := fiber.New()

	app.Get("/history", FindHistory)
	url := fmt.Sprintf("/history?page=1&page_size=10&status=200,404&workspace=%d&methods=GET,POST&sources=scan", workspace.ID)
	req := httptest.NewRequest("GET", url, nil)
	resp, _ := app.Test(req)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestFindHistoryPost(t *testing.T) {
	workspace, err := db.Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)

	app := fiber.New()
	app.Post("/api/v1/history/search", FindHistoryPost)

	tests := []struct {
		name           string
		payload        db.HistoryFilter
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid input",
			payload: db.HistoryFilter{
				StatusCodes: []int{200, 404},
				Methods:     []string{"GET", "POST"},
				Sources:     []string{"scan"},
				WorkspaceID: workspace.ID,
				Pagination: db.Pagination{
					Page:     1,
					PageSize: 10,
				},
				SortBy:    "id",
				SortOrder: "desc",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Valid input 2",
			payload: db.HistoryFilter{
				StatusCodes: []int{200, 500, 400},
				Methods:     []string{"GET", "POST", "HEAD"},
				Sources:     []string{"scan", "browser"},
				WorkspaceID: workspace.ID,
				Pagination: db.Pagination{
					Page:     3,
					PageSize: 100,
				},
				SortBy:    "response_body_size",
				SortOrder: "asc",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Invalid workspace ID",
			payload: db.HistoryFilter{
				WorkspaceID: 9999999999999,
				StatusCodes: []int{200, 404},
				Methods:     []string{"GET", "POST"},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid workspace",
		},
		{
			name: "Invalid HTTP method",
			payload: db.HistoryFilter{
				Methods: []string{"GET", "INVALID"},
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedError:  "Filters validation failed",
		},
		{
			name: "Invalid sort field",
			payload: db.HistoryFilter{
				SortBy: "invalid_field",
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedError:  "Filters validation failed",
		},
		{
			name: "Invalid sort order",
			payload: db.HistoryFilter{
				SortOrder: "invalid_order",
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedError:  "Filters validation failed",
		},
		{
			name: "Negative page number",
			payload: db.HistoryFilter{
				Pagination: db.Pagination{
					Page: -1,
				},
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedError:  "Filters validation failed",
		},
		{
			name: "Invalid status code",
			payload: db.HistoryFilter{
				StatusCodes: []int{200, 600},
			},
			expectedStatus: fiber.StatusBadRequest,
			expectedError:  "Filters validation failed",
		}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadBytes, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/api/v1/history/search", bytes.NewReader(payloadBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, _ := app.Test(req)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var errorResp map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&errorResp)
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedError, errorResp["error"])
			} else {
				var successResp map[string]interface{}
				err := json.NewDecoder(resp.Body).Decode(&successResp)
				assert.Nil(t, err)
				assert.Contains(t, successResp, "data")
				assert.Contains(t, successResp, "count")
			}
		})
	}

	// Clean up: Delete the workspace
	err = db.Connection().DeleteWorkspace(workspace.ID)
	assert.Nil(t, err)
}

func TestGetHistoryDetail(t *testing.T) {
	workspace, err := db.Connection().CreateDefaultWorkspace()
	assert.Nil(t, err)

	app := fiber.New()
	app.Get("/history/:id", GetHistoryDetail)

	// Create a test history item
	history := db.History{
		StatusCode:          200,
		URL:                 "https://example.com/test",
		CleanURL:            "https://example.com/test",
		Depth:               1,
		RawRequest:          []byte("GET /test HTTP/1.1\r\nHost: example.com\r\n\r\n"),
		RawResponse:         []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html>Test</html>"),
		Method:              "GET",
		Proto:               "HTTP/1.1",
		ParametersCount:     0,
		Evaluated:           false,
		Note:                "Test history item",
		Source:              "manual",
		WorkspaceID:         &workspace.ID,
		ResponseBodySize:    19,
		RequestBodySize:     0,
		RequestContentType:  "",
		ResponseContentType: "text/html",
		IsWebSocketUpgrade:  false,
	}
	createdHistory, err := db.Connection().CreateHistory(&history)
	assert.Nil(t, err)

	// Test successful retrieval
	t.Run("Get existing history", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/history/"+strconv.Itoa(int(createdHistory.ID)), nil)
		resp, _ := app.Test(req)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result db.History
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, createdHistory.ID, result.ID)
		assert.Equal(t, "https://example.com/test", result.URL)
		assert.Equal(t, "GET", result.Method)
		assert.Equal(t, 200, result.StatusCode)
	})

	// Test non-existent ID
	t.Run("Get non-existent history", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/history/999999999", nil)
		resp, _ := app.Test(req)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var result ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, "History not found", result.Error)
	})

	// Test invalid ID format
	t.Run("Invalid ID format", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/history/invalid", nil)
		resp, _ := app.Test(req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var result ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&result)
		assert.Nil(t, err)
		assert.Equal(t, "Invalid history ID", result.Error)
	})

	// Clean up
	db.Connection().DB().Delete(&createdHistory)
	db.Connection().DB().Delete(&workspace)
}
