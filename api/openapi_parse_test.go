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
	"github.com/stretchr/testify/require"
)

func TestParseOpenAPISpec_ValidInput(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	// Create a test server to serve a mock OpenAPI spec
	mockSpec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"paths": {
			"/test": {
				"get": {
					"summary": "Test endpoint",
					"responses": {"200": {"description": "Success"}}
				}
			}
		}
	}`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockSpec))
	}))
	defer mockServer.Close()

	input := ParseOpenAPISpecInput{
		URL:             mockServer.URL,
		IncludeOptional: false,
		EnableFuzzing:   false,
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader(inputJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var response ParseOpenAPISpecResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.NotEmpty(t, response.Endpoints)
	assert.Greater(t, response.Count, 0)
	assert.NotEmpty(t, response.BaseURL)
}

func TestParseOpenAPISpec_InvalidURL(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	tests := []struct {
		name  string
		input ParseOpenAPISpecInput
	}{
		{
			name: "missing URL",
			input: ParseOpenAPISpecInput{
				URL: "",
			},
		},
		{
			name: "malformed URL",
			input: ParseOpenAPISpecInput{
				URL: "not-a-valid-url",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, err := json.Marshal(tt.input)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader(inputJSON))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestParseOpenAPISpec_404Response(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	input := ParseOpenAPISpecInput{
		URL: mockServer.URL,
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader(inputJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestParseOpenAPISpec_InvalidSpec(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"invalid": "spec"}`))
	}))
	defer mockServer.Close()

	input := ParseOpenAPISpecInput{
		URL: mockServer.URL,
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader(inputJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestParseOpenAPISpec_WithFuzzing(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	mockSpec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"paths": {
			"/users/{id}": {
				"get": {
					"summary": "Get user by ID",
					"parameters": [
						{
							"name": "id",
							"in": "path",
							"required": true,
							"schema": {"type": "integer"}
						}
					],
					"responses": {"200": {"description": "Success"}}
				}
			}
		}
	}`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockSpec))
	}))
	defer mockServer.Close()

	input := ParseOpenAPISpecInput{
		URL:             mockServer.URL,
		IncludeOptional: false,
		EnableFuzzing:   true,
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader(inputJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var response ParseOpenAPISpecResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// With fuzzing enabled, we should have more request variations
	assert.NotEmpty(t, response.Endpoints)
	if len(response.Endpoints) > 0 {
		// The first endpoint should have multiple request variations (happy path + fuzz variations)
		assert.Greater(t, len(response.Endpoints[0].Requests), 1, "Expected multiple request variations with fuzzing enabled")
	}
}

func TestParseOpenAPISpec_WithBaseURLOverride(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	mockSpec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://api.example.com"}],
		"paths": {
			"/test": {
				"get": {
					"summary": "Test endpoint",
					"responses": {"200": {"description": "Success"}}
				}
			}
		}
	}`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockSpec))
	}))
	defer mockServer.Close()

	customBaseURL := "https://custom.example.com"
	input := ParseOpenAPISpecInput{
		URL:     mockServer.URL,
		BaseURL: customBaseURL,
	}
	inputJSON, err := json.Marshal(input)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader(inputJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var response ParseOpenAPISpecResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	// The response should use the custom base URL
	assert.Equal(t, customBaseURL, response.BaseURL)
}

func TestParseOpenAPISpec_MalformedJSON(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

// Benchmark for parsing a reasonably sized OpenAPI spec
func BenchmarkParseOpenAPISpec(b *testing.B) {
	app := fiber.New()
	app.Post("/api/v1/playground/openapi/parse", ParseOpenAPISpec)

	mockSpec := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"paths": {
			"/test": {
				"get": {
					"summary": "Test endpoint",
					"responses": {"200": {"description": "Success"}}
				}
			}
		}
	}`

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockSpec))
	}))
	defer mockServer.Close()

	input := ParseOpenAPISpecInput{
		URL: mockServer.URL,
	}
	inputJSON, _ := json.Marshal(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/openapi/parse", bytes.NewReader(inputJSON))
		req.Header.Set("Content-Type", "application/json")
		resp, _ := app.Test(req)
		resp.Body.Close()
	}
}

func init() {
	// Initialize the database connection for tests
	db.Connection()
}
