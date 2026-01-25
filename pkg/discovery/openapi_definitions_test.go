package discovery

import (
	"testing"

	"github.com/pyneda/sukyan/db"
)

func TestIsOpenAPIValidationFunc(t *testing.T) {
	tests := []struct {
		name       string
		history    *db.History
		body       string
		shouldPass bool
	}{
		{
			name: "Valid OpenAPI 3.0 spec",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/openapi.json",
			},
			body: `{
				"openapi": "3.0.0",
				"info": {"title": "Test API", "version": "1.0.0"},
				"paths": {"/users": {"get": {"summary": "Get users"}}}
			}`,
			shouldPass: true,
		},
		{
			name: "Valid Swagger 2.0 spec",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/swagger.json",
			},
			body: `{
				"swagger": "2.0",
				"info": {"title": "Test API", "version": "1.0.0"},
				"basePath": "/api",
				"paths": {"/users": {"get": {"summary": "Get users"}}}
			}`,
			shouldPass: true,
		},
		{
			name: "Generic JSON API response - should NOT match",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/api/data.json",
			},
			body: `{
				"data": [{"id": 1, "type": "user", "description": "Admin user"}],
				"meta": {"total": 100}
			}`,
			shouldPass: false,
		},
		{
			name: "JSON with generic fields - should NOT match",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/config.json",
			},
			body: `{
				"type": "configuration",
				"description": "App config",
				"properties": {"theme": "dark"},
				"required": ["theme"]
			}`,
			shouldPass: false,
		},
		{
			name: "404 response - should NOT match",
			history: &db.History{
				StatusCode:          404,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/openapi.json",
			},
			body: `{"error": "Not found"}`,
			shouldPass: false,
		},
		{
			name: "HTML response - should NOT match",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "text/html",
				URL:                 "https://example.com/swagger.json",
			},
			body: `<html><body>Swagger documentation</body></html>`,
			shouldPass: false,
		},
		{
			name: "Empty body - should NOT match",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/openapi.json",
			},
			body:       ``,
			shouldPass: false,
		},
		{
			name: "Invalid JSON - should NOT match",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/openapi.json",
			},
			body:       `not valid json`,
			shouldPass: false,
		},
		{
			name: "JSON Schema (not OpenAPI) - should NOT match",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/schema.json",
			},
			body: `{
				"$schema": "http://json-schema.org/draft-07/schema#",
				"type": "object",
				"properties": {"name": {"type": "string"}},
				"required": ["name"]
			}`,
			shouldPass: false,
		},
		{
			name: "Minimal valid OpenAPI 3.1",
			history: &db.History{
				StatusCode:          200,
				ResponseContentType: "application/json",
				URL:                 "https://example.com/api.json",
			},
			body: `{
				"openapi": "3.1.0",
				"info": {"title": "API", "version": "1.0"},
				"paths": {}
			}`,
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.history.RawResponse = []byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n" + tt.body)

			passed, details, confidence := IsOpenAPIValidationFunc(tt.history, nil)

			if passed != tt.shouldPass {
				t.Errorf("Expected pass=%v, got pass=%v (confidence=%d, details=%s)",
					tt.shouldPass, passed, confidence, details)
			}

			if passed && confidence < 60 {
				t.Errorf("Expected confidence >= 60 for passing validation, got %d", confidence)
			}
		})
	}
}
