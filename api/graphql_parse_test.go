package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGraphQLSchemaValidation(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/graphql/parse", ParseGraphQLSchema)

	tests := []struct {
		name           string
		input          map[string]interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing url",
			input:          map[string]interface{}{},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
		{
			name: "invalid url",
			input: map[string]interface{}{
				"url": "not-a-valid-url",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, _ := json.Marshal(tt.input)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/graphql/parse", bytes.NewReader(inputJSON))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var response map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&response)
			assert.Contains(t, response["error"], tt.expectedError)
		})
	}
}

func TestParseGraphQLSchemaInvalidJSON(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/graphql/parse", ParseGraphQLSchema)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/graphql/parse", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Contains(t, response["error"], "Cannot parse JSON")
}

func TestParseGraphQLFromIntrospectionValidation(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/graphql/parse-introspection", ParseGraphQLFromIntrospection)

	tests := []struct {
		name           string
		input          map[string]interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing introspection data",
			input:          map[string]interface{}{"base_url": "http://localhost/graphql"},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
		{
			name: "missing base_url",
			input: map[string]interface{}{
				"introspection_data": map[string]interface{}{},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
		{
			name: "invalid base_url",
			input: map[string]interface{}{
				"introspection_data": map[string]interface{}{},
				"base_url":           "not-a-url",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputJSON, _ := json.Marshal(tt.input)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/graphql/parse-introspection", bytes.NewReader(inputJSON))
			req.Header.Set("Content-Type", "application/json")

			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			var response map[string]interface{}
			json.NewDecoder(resp.Body).Decode(&response)
			assert.Contains(t, response["error"], tt.expectedError)
		})
	}
}

func TestParseGraphQLFromIntrospectionSuccess(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/graphql/parse-introspection", ParseGraphQLFromIntrospection)

	// Sample minimal introspection data
	introspectionData := map[string]interface{}{
		"data": map[string]interface{}{
			"__schema": map[string]interface{}{
				"queryType":        map[string]interface{}{"name": "Query"},
				"mutationType":     nil,
				"subscriptionType": nil,
				"types": []interface{}{
					map[string]interface{}{
						"kind":        "OBJECT",
						"name":        "Query",
						"description": nil,
						"fields": []interface{}{
							map[string]interface{}{
								"name":        "hello",
								"description": "Hello world query",
								"args":        []interface{}{},
								"type": map[string]interface{}{
									"kind":   "SCALAR",
									"name":   "String",
									"ofType": nil,
								},
								"isDeprecated":      false,
								"deprecationReason": nil,
							},
						},
						"inputFields":   nil,
						"interfaces":    []interface{}{},
						"enumValues":    nil,
						"possibleTypes": nil,
					},
					map[string]interface{}{
						"kind":          "SCALAR",
						"name":          "String",
						"description":   nil,
						"fields":        nil,
						"inputFields":   nil,
						"interfaces":    nil,
						"enumValues":    nil,
						"possibleTypes": nil,
					},
				},
				"directives": []interface{}{},
			},
		},
	}

	input := map[string]interface{}{
		"introspection_data": introspectionData,
		"base_url":           "http://localhost:4000/graphql",
		"include_optional":   true,
		"enable_fuzzing":     false,
	}

	inputJSON, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/graphql/parse-introspection", bytes.NewReader(inputJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response ParseGraphQLSchemaResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:4000/graphql", response.BaseURL)
	assert.Equal(t, 1, response.Count)
	assert.Len(t, response.Endpoints, 1)
	assert.Equal(t, "hello", response.Endpoints[0].Name)
	assert.Equal(t, "query", response.Endpoints[0].OperationType)
	assert.NotNil(t, response.Schema)
	assert.Equal(t, 1, response.Schema.QueryCount)
}

func TestParseGraphQLFromIntrospectionInvalidData(t *testing.T) {
	app := fiber.New()
	app.Post("/api/v1/playground/graphql/parse-introspection", ParseGraphQLFromIntrospection)

	input := map[string]interface{}{
		"introspection_data": map[string]interface{}{"invalid": "data"},
		"base_url":           "http://localhost:4000/graphql",
	}

	inputJSON, _ := json.Marshal(input)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playground/graphql/parse-introspection", bytes.NewReader(inputJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var response map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&response)
	assert.Contains(t, response["error"], "Failed to parse introspection data")
}
