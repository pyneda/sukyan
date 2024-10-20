package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
)

func TestJwtListHandler(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/tokens/jwts", JwtListHandler)

	tests := []struct {
		name           string
		payload        db.JwtFilters
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid input",
			payload: db.JwtFilters{
				Algorithm:   "HS256",
				Issuer:      "test-issuer",
				Subject:     "test-subject",
				Audience:    "test-audience",
				SortBy:      "token",
				SortOrder:   "asc",
				WorkspaceID: 1,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Invalid algorithm",
			payload: db.JwtFilters{
				Algorithm: "INVALID_ALG",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
		{
			name: "Invalid sort_by",
			payload: db.JwtFilters{
				SortBy: "invalid_field",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
		{
			name: "Invalid sort_order",
			payload: db.JwtFilters{
				SortOrder: "invalid_order",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
		{
			name: "Invalid workspace_id",
			payload: db.JwtFilters{
				WorkspaceID: 999999,
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid workspace",
		},
		{
			name:           "Empty payload",
			payload:        db.JwtFilters{},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payloadBytes, _ := json.Marshal(tt.payload)
			req := httptest.NewRequest("POST", "/api/v1/tokens/jwts", bytes.NewReader(payloadBytes))
			req.Header.Set("Content-Type", "application/json")

			resp, _ := app.Test(req)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.expectedError != "" {
				var errorResp map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&errorResp)
				assert.Equal(t, tt.expectedError, errorResp["error"])
			} else {
				var jwts []db.JsonWebToken
				err := json.NewDecoder(resp.Body).Decode(&jwts)
				assert.Nil(t, err)
			}
		})
	}
}
