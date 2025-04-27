package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
				json.NewDecoder(resp.Body).Decode(&errorResp)
				assert.Equal(t, tt.expectedError, errorResp["error"])
			} else {
				var successResp map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&successResp)
				assert.Contains(t, successResp, "data")
				assert.Contains(t, successResp, "count")
			}
		})
	}

	// Clean up: Delete the workspace
	err = db.Connection().DeleteWorkspace(workspace.ID)
	assert.Nil(t, err)
}
