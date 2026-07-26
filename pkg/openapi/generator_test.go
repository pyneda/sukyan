package openapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPathLevelParameters covers parameters declared on the path item and shared by
// every operation. When they are ignored the placeholder survives into the URL as
// %7BuserId%7D and the endpoint is never actually reached.
func TestPathLevelParameters(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "servers": [{"url": "https://api.example.com/v1"}],
	  "paths": {
	    "/users/{userId}/posts/{postId}": {
	      "parameters": [
	        {"name": "userId", "in": "path", "required": true, "schema": {"type": "integer"}},
	        {"name": "postId", "in": "path", "required": true, "schema": {"type": "string", "default": "abc"}}
	      ],
	      "get": {"responses": {"200": {"description": "ok"}}}
	    }
	  }
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: doc.BaseURL()})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	if got := len(endpoints[0].Parameters); got != 2 {
		t.Errorf("Parameters = %d, want 2 (path item parameters must be reported)", got)
	}
	if got := endpoints[0].Requests[0].URL; got != "https://api.example.com/v1/users/1/posts/abc" {
		t.Errorf("URL = %q, want https://api.example.com/v1/users/1/posts/abc", got)
	}
}

func TestOperationParameterOverridesPathParameter(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {
	    "/items": {
	      "parameters": [{"name": "limit", "in": "query", "required": true, "schema": {"type": "string", "default": "shared"}}],
	      "get": {
	        "parameters": [{"name": "limit", "in": "query", "required": true, "schema": {"type": "string", "default": "operation"}}],
	        "responses": {"200": {"description": "ok"}}
	      }
	    }
	  }
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	if got := len(endpoints[0].Parameters); got != 1 {
		t.Fatalf("Parameters = %d, want 1 deduplicated parameter", got)
	}
	if got := endpoints[0].Requests[0].URL; !strings.Contains(got, "limit=operation") {
		t.Errorf("URL = %q, want the operation-level default to win", got)
	}
}

func TestServerURLResolution(t *testing.T) {
	tests := []struct {
		name      string
		servers   string
		sourceURL string
		wantBase  string
	}{
		{
			name:     "templated variables are substituted",
			servers:  `[{"url": "https://{env}.example.com/{version}", "variables": {"env": {"default": "prod"}, "version": {"default": "v2"}}}]`,
			wantBase: "https://prod.example.com/v2",
		},
		{
			name:     "variable enum is used when no default",
			servers:  `[{"url": "https://{env}.example.com", "variables": {"env": {"enum": ["stage"], "default": ""}}}]`,
			wantBase: "https://stage.example.com",
		},
		{
			name:      "relative server resolves against the source url",
			servers:   `[{"url": "/api/v3"}]`,
			sourceURL: "https://target.example.com/openapi.json",
			wantBase:  "https://target.example.com/api/v3",
		},
		{
			name:     "relative server without a source url is not reported as usable",
			servers:  `[{"url": "/api/v3"}]`,
			wantBase: "",
		},
		{
			name:     "first absolute server wins over an unusable one",
			servers:  `[{"url": "/relative"}, {"url": "https://api.example.com"}]`,
			wantBase: "https://api.example.com",
		},
		{
			name:     "no servers",
			servers:  `[]`,
			wantBase: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},"servers":` + tt.servers +
				`,"paths":{"/pet":{"get":{"responses":{"200":{"description":"ok"}}}}}}`

			doc := mustParse(t, spec, ParseOptions{SourceURL: tt.sourceURL})
			if got := doc.BaseURL(); got != tt.wantBase {
				t.Errorf("BaseURL() = %q, want %q", got, tt.wantBase)
			}

			// Whatever the server declaration, generation must not panic and must
			// produce an absolute URL.
			endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: doc.BaseURL()})
			if err != nil {
				t.Fatalf("GenerateRequests: %v", err)
			}
			if url := endpoints[0].Requests[0].URL; !strings.HasPrefix(url, "http") {
				t.Errorf("generated URL %q is not absolute", url)
			}
		})
	}
}

func TestGenerateRequestsRejectsUnusableBaseURL(t *testing.T) {
	doc := mustParse(t, `{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{"/p":{"get":{"responses":{"200":{"description":"ok"}}}}}}`)

	for _, baseURL := range []string{"https://{env}.example.com", "/api/v3", "::not a url"} {
		if _, err := GenerateRequests(doc, GenerationConfig{BaseURL: baseURL}); err == nil {
			t.Errorf("GenerateRequests with base %q succeeded, want an error", baseURL)
		}
	}
}

func TestRequestBodyContentTypes(t *testing.T) {
	tests := []struct {
		name            string
		content         string
		wantContentType string
		wantBody        string
	}{
		{
			name:            "json",
			content:         `{"application/json": {"schema": {"type": "object", "properties": {"a": {"type": "string"}}}}}`,
			wantContentType: "application/json",
			wantBody:        `{"a":"string_value"}`,
		},
		{
			name:            "vendor json",
			content:         `{"application/vnd.api+json": {"schema": {"type": "object", "properties": {"a": {"type": "string"}}}}}`,
			wantContentType: "application/vnd.api+json",
			wantBody:        `{"a":"string_value"}`,
		},
		{
			name:            "form urlencoded",
			content:         `{"application/x-www-form-urlencoded": {"schema": {"type": "object", "properties": {"user": {"type": "string"}, "id": {"type": "integer"}}}}}`,
			wantContentType: "application/x-www-form-urlencoded",
			wantBody:        "id=1&user=string_value",
		},
		{
			name:            "text plain",
			content:         `{"text/plain": {"schema": {"type": "string"}}}`,
			wantContentType: "text/plain",
			wantBody:        "string_value",
		},
		{
			name:            "json preferred over other declared types",
			content:         `{"application/xml": {"schema": {"type": "object", "properties": {"a": {"type": "string"}}}}, "application/json": {"schema": {"type": "object", "properties": {"a": {"type": "string"}}}}}`,
			wantContentType: "application/json",
			wantBody:        `{"a":"string_value"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{"/p":{"post":{` +
				`"requestBody":{"required":true,"content":` + tt.content + `},` +
				`"responses":{"200":{"description":"ok"}}}}}}`

			doc := mustParse(t, spec)
			endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
			if err != nil {
				t.Fatalf("GenerateRequests: %v", err)
			}

			request := endpoints[0].Requests[0]
			if got := request.Headers["Content-Type"]; got != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantContentType)
			}
			if got := string(request.Body); got != tt.wantBody {
				t.Errorf("body = %q, want %q", got, tt.wantBody)
			}
		})
	}
}

func TestMultipartRequestBody(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"multipart/form-data":{"schema":{"type":"object","properties":{"file":{"type":"string"}}}}}},
	  "responses":{"200":{"description":"ok"}}}}}}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	body := string(endpoints[0].Requests[0].Body)
	if !strings.Contains(body, `name="file"`) || !strings.Contains(body, "string_value") {
		t.Errorf("multipart body missing the declared field: %q", body)
	}
}

// TestComposedRequestBody covers allOf/oneOf/anyOf. A composed schema has no direct
// properties, so without composition resolution the body serialises to "{}" and the
// endpoint is effectively untested.
func TestComposedRequestBody(t *testing.T) {
	tests := []struct {
		name    string
		schema  string
		wantKey []string
	}{
		{
			name:    "allOf merges every subschema",
			schema:  `{"allOf": [{"type": "object", "properties": {"id": {"type": "integer"}}}, {"type": "object", "properties": {"name": {"type": "string"}}}]}`,
			wantKey: []string{"id", "name"},
		},
		{
			name:    "oneOf uses the first object variant",
			schema:  `{"oneOf": [{"type": "object", "properties": {"email": {"type": "string"}}}, {"type": "object", "properties": {"phone": {"type": "string"}}}]}`,
			wantKey: []string{"email"},
		},
		{
			name:    "anyOf uses the first object variant",
			schema:  `{"anyOf": [{"type": "object", "properties": {"token": {"type": "string"}}}]}`,
			wantKey: []string{"token"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{"/p":{"post":{` +
				`"requestBody":{"required":true,"content":{"application/json":{"schema":` + tt.schema + `}}},` +
				`"responses":{"200":{"description":"ok"}}}}}}`

			doc := mustParse(t, spec)
			endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
			if err != nil {
				t.Fatalf("GenerateRequests: %v", err)
			}

			var body map[string]interface{}
			if err := json.Unmarshal(endpoints[0].Requests[0].Body, &body); err != nil {
				t.Fatalf("body is not a JSON object: %v", err)
			}
			for _, key := range tt.wantKey {
				if _, ok := body[key]; !ok {
					t.Errorf("body %s is missing composed property %q", endpoints[0].Requests[0].Body, key)
				}
			}
		})
	}
}

func TestNonObjectRequestBody(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"array","items":{"type":"string"}}}}},
	  "responses":{"200":{"description":"ok"}}}}}}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	if got := string(endpoints[0].Requests[0].Body); got != `["string_value"]` {
		t.Errorf("body = %q, want an array body rather than an empty object", got)
	}
}

func TestParameterSerialization(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {"/p": {"get": {"parameters": [
	    {"name": "tags", "in": "query", "required": true, "schema": {"type": "array", "items": {"type": "string", "default": "a"}}},
	    {"name": "csv", "in": "query", "required": true, "style": "form", "explode": false, "schema": {"type": "array", "items": {"type": "string", "default": "b"}}},
	    {"name": "ratio", "in": "query", "required": true, "schema": {"type": "number", "default": 1500000.5}},
	    {"name": "example_param", "in": "query", "required": true, "example": "from-example", "schema": {"type": "string"}}
	  ], "responses": {"200": {"description": "ok"}}}}}
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	url := endpoints[0].Requests[0].URL
	// Arrays must not be rendered with Go's %v as "[a]".
	if strings.Contains(url, "%5B") || strings.Contains(url, "[a]") {
		t.Errorf("URL %q renders an array with Go formatting", url)
	}
	if !strings.Contains(url, "tags=a") {
		t.Errorf("URL %q should carry the exploded array value", url)
	}
	// Scientific notation is rejected by many numeric parsers.
	if strings.Contains(url, "e%2B") || strings.Contains(url, "e+") {
		t.Errorf("URL %q renders a number in scientific notation", url)
	}
	if !strings.Contains(url, "ratio=1500000.5") {
		t.Errorf("URL %q should carry the plain decimal number", url)
	}
	if !strings.Contains(url, "example_param=from-example") {
		t.Errorf("URL %q should prefer the declared example", url)
	}
}

func TestPathParameterValuesAreEscaped(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {"/files/{name}": {"get": {
	    "parameters": [{"name": "name", "in": "path", "required": true, "schema": {"type": "string", "default": "a/../b?x=1"}}],
	    "responses": {"200": {"description": "ok"}}}}}
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	url := endpoints[0].Requests[0].URL
	if strings.Contains(url, "?") {
		t.Errorf("URL %q lets a path parameter start a query string", url)
	}
	if !strings.Contains(url, "%2F") {
		t.Errorf("URL %q should escape the slash inside the path parameter", url)
	}
}

func TestUnsubstitutedPathPlaceholderIsFilled(t *testing.T) {
	// A spec that never declares a parameter for its own template is common enough
	// that requesting a literal "{id}" would just 404.
	spec := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},
	  "paths":{"/users/{id}":{"get":{"responses":{"200":{"description":"ok"}}}}}}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	if got := endpoints[0].Requests[0].URL; got != "http://target/users/1" {
		t.Errorf("URL = %q, want http://target/users/1", got)
	}
}

func TestHeaderAndCookieValuesAreSanitized(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {"/p": {"get": {
	    "parameters": [{"name": "X-Evil\r\nInjected", "in": "header", "required": true, "schema": {"type": "string", "default": "a\r\nX-Other: b"}}],
	    "responses": {"200": {"description": "ok"}}}}}
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	for name, value := range endpoints[0].Requests[0].Headers {
		if strings.ContainsAny(name, "\r\n") || strings.ContainsAny(value, "\r\n") {
			t.Errorf("header %q = %q still carries control characters", name, value)
		}
	}
}

func TestGenerationIsDeterministic(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "servers": [{"url": "https://api.example.com"}],
	  "security": [{"bearerAuth": []}],
	  "paths": {
	    "/alpha": {"get": {"responses": {"200": {"description": "ok"}}}, "post": {
	      "requestBody": {"required": true, "content": {"application/json": {"schema": {"type": "object", "properties": {
	        "a": {"type": "string"}, "b": {"type": "integer"}, "c": {"type": "boolean"}, "d": {"type": "number"}}}}}},
	      "responses": {"200": {"description": "ok"}}}},
	    "/beta": {"get": {"parameters": [
	      {"name": "q", "in": "query", "required": true, "schema": {"type": "string"}}],
	      "responses": {"200": {"description": "ok"}}}}
	  },
	  "components": {"securitySchemes": {"bearerAuth": {"type": "http", "scheme": "bearer"}}}
	}`

	doc := mustParse(t, spec)
	config := GenerationConfig{BaseURL: doc.BaseURL(), FuzzingEnabled: true}

	first, err := GenerateRequests(doc, config)
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	want, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 30; i++ {
		next, err := GenerateRequests(doc, config)
		if err != nil {
			t.Fatalf("GenerateRequests: %v", err)
		}
		got, err := json.Marshal(next)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Fatalf("run %d produced different output than run 0", i+1)
		}
	}
}

func TestIncludeOptionalParams(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {"/search": {"get": {"parameters": [
	    {"name": "q", "in": "query", "required": true, "schema": {"type": "string"}},
	    {"name": "limit", "in": "query", "schema": {"type": "integer"}}
	  ], "responses": {"200": {"description": "ok"}}}}}
	}`

	doc := mustParse(t, spec)

	minimal, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if got := minimal[0].Requests[0].URL; strings.Contains(got, "limit=") {
		t.Errorf("happy path %q should omit the optional parameter", got)
	}
	// The optional parameter is still attack surface, so a full variation exists.
	if !anyRequestContains(minimal[0], "limit=") {
		t.Error("expected a full-parameter variation carrying the optional parameter")
	}

	full, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target", IncludeOptionalParams: true})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if got := full[0].Requests[0].URL; !strings.Contains(got, "limit=") {
		t.Errorf("URL %q should include the optional parameter", got)
	}
}

func TestFuzzingCoversOptionalParameters(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {"/search": {"get": {"parameters": [
	    {"name": "limit", "in": "query", "schema": {"type": "integer", "format": "int64"}}
	  ], "responses": {"200": {"description": "ok"}}}}}
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target", FuzzingEnabled: true})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	if !anyRequestContains(endpoints[0], "limit=9223372036854775807") {
		t.Error("fuzzing should reach optional parameters with int64 boundary values")
	}
}

func TestMaxRequestsPerEndpoint(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {"/search": {"get": {"parameters": [
	    {"name": "a", "in": "query", "required": true, "schema": {"type": "string"}},
	    {"name": "b", "in": "query", "required": true, "schema": {"type": "string"}},
	    {"name": "c", "in": "query", "required": true, "schema": {"type": "string"}}
	  ], "responses": {"200": {"description": "ok"}}}}}
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{
		BaseURL:                "http://target",
		FuzzingEnabled:         true,
		MaxRequestsPerEndpoint: 4,
	})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if got := len(endpoints[0].Requests); got != 4 {
		t.Errorf("requests = %d, want the configured cap of 4", got)
	}
}

func TestOperationsHandlesDocumentWithoutPaths(t *testing.T) {
	doc := mustParse(t, `{"openapi":"3.0.0","info":{"title":"x","version":"1"}}`)

	if got := len(doc.Operations()); got != 0 {
		t.Errorf("Operations() = %d, want 0", got)
	}
	if got := len(doc.GetOperations()); got != 0 {
		t.Errorf("GetOperations() = %d, want 0", got)
	}
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("endpoints = %d, want 0", len(endpoints))
	}
}

func anyRequestContains(endpoint Endpoint, needle string) bool {
	for _, request := range endpoint.Requests {
		if strings.Contains(request.URL, needle) || strings.Contains(string(request.Body), needle) {
			return true
		}
	}
	return false
}
