package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/pkg/api/core"
)

func TestBuildJSONBody(t *testing.T) {
	tests := []struct {
		name           string
		params         []core.Parameter
		paramValues    map[string]any
		expectedBody   map[string]any
		expectNoBody   bool
	}{
		{
			name: "single string field",
			params: []core.Parameter{
				{Name: "username", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
			},
			paramValues:  map[string]any{"username": "admin"},
			expectedBody: map[string]any{"username": "admin"},
		},
		{
			name: "multiple fields with mixed types",
			params: []core.Parameter{
				{Name: "name", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
				{Name: "age", Location: core.ParameterLocationBody, DataType: core.DataTypeInteger},
				{Name: "active", Location: core.ParameterLocationBody, DataType: core.DataTypeBoolean},
			},
			paramValues: map[string]any{
				"name":   "alice",
				"age":    float64(30),
				"active": true,
			},
			expectedBody: map[string]any{
				"name":   "alice",
				"age":    float64(30),
				"active": true,
			},
		},
		{
			name: "uses effective value when param value is nil",
			params: []core.Parameter{
				{Name: "role", Location: core.ParameterLocationBody, DataType: core.DataTypeString, ExampleValue: "editor"},
			},
			paramValues:  map[string]any{},
			expectedBody: map[string]any{"role": "editor"},
		},
		{
			name: "nested object body param",
			params: []core.Parameter{
				{
					Name:     "address",
					Location: core.ParameterLocationBody,
					DataType: core.DataTypeObject,
					NestedParams: []core.Parameter{
						{Name: "city", DataType: core.DataTypeString, ExampleValue: "NYC"},
						{Name: "zip", DataType: core.DataTypeString, ExampleValue: "10001"},
					},
				},
			},
			paramValues: map[string]any{},
			expectedBody: map[string]any{
				"address": map[string]any{"city": "NYC", "zip": "10001"},
			},
		},
		{
			name:         "no body params produces no body",
			params:       []core.Parameter{{Name: "q", Location: core.ParameterLocationQuery, DataType: core.DataTypeString}},
			paramValues:  map[string]any{"q": "search"},
			expectNoBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := core.Operation{
				Method:     "POST",
				Path:       "/test",
				BaseURL:    "https://api.example.com",
				Parameters: tt.params,
			}

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			body, _ := io.ReadAll(req.Body)
			if tt.expectNoBody {
				if len(body) != 0 {
					t.Fatalf("expected no body, got %s", string(body))
				}
				return
			}

			var got map[string]any
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("failed to unmarshal body: %v", err)
			}

			if err := compareMaps(tt.expectedBody, got); err != nil {
				t.Errorf("body mismatch: %v\ngot:  %v\nwant: %v", err, got, tt.expectedBody)
			}

			ct := req.Header.Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", ct)
			}
		})
	}
}

func TestBuildFormURLEncodedBody(t *testing.T) {
	tests := []struct {
		name         string
		params       []core.Parameter
		paramValues  map[string]any
		expectedVals url.Values
	}{
		{
			name: "single field",
			params: []core.Parameter{
				{Name: "grant_type", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
			},
			paramValues:  map[string]any{"grant_type": "client_credentials"},
			expectedVals: url.Values{"grant_type": {"client_credentials"}},
		},
		{
			name: "multiple fields",
			params: []core.Parameter{
				{Name: "username", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
				{Name: "password", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
			},
			paramValues: map[string]any{"username": "admin", "password": "secret"},
			expectedVals: url.Values{
				"username": {"admin"},
				"password": {"secret"},
			},
		},
		{
			name: "integer value is serialized",
			params: []core.Parameter{
				{Name: "count", Location: core.ParameterLocationBody, DataType: core.DataTypeInteger},
			},
			paramValues:  map[string]any{"count": 42},
			expectedVals: url.Values{"count": {"42"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := core.Operation{
				Method:     "POST",
				Path:       "/token",
				BaseURL:    "https://auth.example.com",
				Parameters: tt.params,
				OpenAPI: &core.OpenAPIMetadata{
					RequestBody: &core.RequestBodyInfo{
						ContentType: "application/x-www-form-urlencoded",
					},
				},
			}

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			body, _ := io.ReadAll(req.Body)
			parsed, err := url.ParseQuery(string(body))
			if err != nil {
				t.Fatalf("failed to parse form body: %v", err)
			}

			for key, wantVals := range tt.expectedVals {
				gotVals, ok := parsed[key]
				if !ok {
					t.Errorf("missing form field %q", key)
					continue
				}
				if len(gotVals) != len(wantVals) {
					t.Errorf("field %q: got %d values, want %d", key, len(gotVals), len(wantVals))
					continue
				}
				for i := range wantVals {
					if gotVals[i] != wantVals[i] {
						t.Errorf("field %q[%d]: got %q, want %q", key, i, gotVals[i], wantVals[i])
					}
				}
			}

			ct := req.Header.Get("Content-Type")
			if ct != "application/x-www-form-urlencoded" {
				t.Errorf("expected Content-Type application/x-www-form-urlencoded, got %s", ct)
			}
		})
	}
}

func TestBuildMultipartBody(t *testing.T) {
	tests := []struct {
		name         string
		params       []core.Parameter
		paramValues  map[string]any
		expectedVals map[string]string
	}{
		{
			name: "single field",
			params: []core.Parameter{
				{Name: "file_name", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
			},
			paramValues:  map[string]any{"file_name": "report.pdf"},
			expectedVals: map[string]string{"file_name": "report.pdf"},
		},
		{
			name: "multiple fields",
			params: []core.Parameter{
				{Name: "title", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
				{Name: "description", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
			},
			paramValues:  map[string]any{"title": "My Upload", "description": "A test upload"},
			expectedVals: map[string]string{"title": "My Upload", "description": "A test upload"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := core.Operation{
				Method:     "POST",
				Path:       "/upload",
				BaseURL:    "https://api.example.com",
				Parameters: tt.params,
				OpenAPI: &core.OpenAPIMetadata{
					RequestBody: &core.RequestBodyInfo{
						ContentType: "multipart/form-data",
					},
				},
			}

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			ct := req.Header.Get("Content-Type")
			mediaType, params, err := mime.ParseMediaType(ct)
			if err != nil {
				t.Fatalf("failed to parse Content-Type: %v", err)
			}
			if mediaType != "multipart/form-data" {
				t.Fatalf("expected multipart/form-data, got %s", mediaType)
			}

			boundary := params["boundary"]
			if boundary == "" {
				t.Fatal("missing multipart boundary")
			}

			body, _ := io.ReadAll(req.Body)
			reader := multipart.NewReader(strings.NewReader(string(body)), boundary)

			gotFields := make(map[string]string)
			for {
				part, err := reader.NextPart()
				if err != nil {
					break
				}
				val, _ := io.ReadAll(part)
				gotFields[part.FormName()] = string(val)
			}

			for key, want := range tt.expectedVals {
				got, ok := gotFields[key]
				if !ok {
					t.Errorf("missing multipart field %q", key)
					continue
				}
				if got != want {
					t.Errorf("field %q: got %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestBuildURLPathSubstitution(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		params       []core.Parameter
		paramValues  map[string]any
		expectedPath string
	}{
		{
			name: "single path param",
			path: "/users/{user_id}",
			params: []core.Parameter{
				{Name: "user_id", Location: core.ParameterLocationPath, DataType: core.DataTypeInteger, ExampleValue: 1},
			},
			paramValues:  map[string]any{"user_id": 42},
			expectedPath: "/users/42",
		},
		{
			name: "multiple path params",
			path: "/orgs/{org_id}/repos/{repo_id}",
			params: []core.Parameter{
				{Name: "org_id", Location: core.ParameterLocationPath, DataType: core.DataTypeString},
				{Name: "repo_id", Location: core.ParameterLocationPath, DataType: core.DataTypeString},
			},
			paramValues:  map[string]any{"org_id": "acme", "repo_id": "scanner"},
			expectedPath: "/orgs/acme/repos/scanner",
		},
		{
			name: "uses effective value when not provided",
			path: "/items/{item_id}",
			params: []core.Parameter{
				{Name: "item_id", Location: core.ParameterLocationPath, DataType: core.DataTypeInteger, ExampleValue: 99},
			},
			paramValues:  map[string]any{},
			expectedPath: "/items/99",
		},
		{
			name:         "no path params leaves path unchanged",
			path:         "/health",
			params:       []core.Parameter{},
			paramValues:  map[string]any{},
			expectedPath: "/health",
		},
		{
			name: "path without leading slash gets normalized",
			path: "users/{id}",
			params: []core.Parameter{
				{Name: "id", Location: core.ParameterLocationPath, DataType: core.DataTypeInteger},
			},
			paramValues:  map[string]any{"id": 7},
			expectedPath: "/users/7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := core.Operation{
				Method:     "GET",
				Path:       tt.path,
				BaseURL:    "https://api.example.com",
				Parameters: tt.params,
			}

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			gotPath := req.URL.Path
			if gotPath != tt.expectedPath {
				t.Errorf("got path %q, want %q", gotPath, tt.expectedPath)
			}
		})
	}
}

func TestPathParamNoDoubleSubstitution(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Method:  "GET",
		Path:    "/users/{user_id}/items/{item_id}",
		BaseURL: "https://api.example.com",
		Parameters: []core.Parameter{
			{Name: "user_id", Location: core.ParameterLocationPath, DataType: core.DataTypeString, Required: true},
			{Name: "item_id", Location: core.ParameterLocationPath, DataType: core.DataTypeString, Required: true},
		},
	}

	paramValues := map[string]any{
		"user_id": "{item_id}",
		"item_id": "real-item",
	}

	req, err := builder.Build(context.Background(), op, paramValues)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	gotURL := req.URL.Path
	if strings.Contains(gotURL, "real-item/items/real-item") {
		t.Errorf("double substitution detected: got path %q", gotURL)
	}
	if !strings.Contains(gotURL, "/items/real-item") {
		t.Errorf("expected /items/real-item in path, got %q", gotURL)
	}
}

func TestBuildURLQueryParams(t *testing.T) {
	tests := []struct {
		name          string
		params        []core.Parameter
		paramValues   map[string]any
		expectedQuery url.Values
	}{
		{
			name: "single query param",
			params: []core.Parameter{
				{Name: "q", Location: core.ParameterLocationQuery, DataType: core.DataTypeString, Required: true},
			},
			paramValues:   map[string]any{"q": "test"},
			expectedQuery: url.Values{"q": {"test"}},
		},
		{
			name: "multiple query params",
			params: []core.Parameter{
				{Name: "page", Location: core.ParameterLocationQuery, DataType: core.DataTypeInteger, Required: true},
				{Name: "limit", Location: core.ParameterLocationQuery, DataType: core.DataTypeInteger, Required: true},
			},
			paramValues: map[string]any{"page": 2, "limit": 50},
			expectedQuery: url.Values{
				"page":  {"2"},
				"limit": {"50"},
			},
		},
		{
			name: "array query param with []any",
			params: []core.Parameter{
				{Name: "tags", Location: core.ParameterLocationQuery, DataType: core.DataTypeArray, Required: true},
			},
			paramValues:   map[string]any{"tags": []any{"go", "security"}},
			expectedQuery: url.Values{"tags": {"go", "security"}},
		},
		{
			name: "array query param with []string",
			params: []core.Parameter{
				{Name: "ids", Location: core.ParameterLocationQuery, DataType: core.DataTypeArray, Required: true},
			},
			paramValues:   map[string]any{"ids": []string{"abc", "def", "ghi"}},
			expectedQuery: url.Values{"ids": {"abc", "def", "ghi"}},
		},
		{
			name: "optional query param omitted when nil",
			params: []core.Parameter{
				{Name: "filter", Location: core.ParameterLocationQuery, DataType: core.DataTypeString, Required: false},
			},
			paramValues:   map[string]any{},
			expectedQuery: url.Values{},
		},
		{
			name: "required query param uses effective value when nil",
			params: []core.Parameter{
				{Name: "status", Location: core.ParameterLocationQuery, DataType: core.DataTypeString, Required: true, ExampleValue: "active"},
			},
			paramValues:   map[string]any{},
			expectedQuery: url.Values{"status": {"active"}},
		},
		{
			name: "mixed required and optional",
			params: []core.Parameter{
				{Name: "sort", Location: core.ParameterLocationQuery, DataType: core.DataTypeString, Required: true},
				{Name: "filter", Location: core.ParameterLocationQuery, DataType: core.DataTypeString, Required: false},
			},
			paramValues:   map[string]any{"sort": "name"},
			expectedQuery: url.Values{"sort": {"name"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := core.Operation{
				Method:     "GET",
				Path:       "/search",
				BaseURL:    "https://api.example.com",
				Parameters: tt.params,
			}

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			gotQuery := req.URL.Query()
			for key, wantVals := range tt.expectedQuery {
				gotVals, ok := gotQuery[key]
				if !ok {
					t.Errorf("missing query param %q", key)
					continue
				}
				if len(gotVals) != len(wantVals) {
					t.Errorf("param %q: got %d values, want %d", key, len(gotVals), len(wantVals))
					continue
				}
				for i := range wantVals {
					if gotVals[i] != wantVals[i] {
						t.Errorf("param %q[%d]: got %q, want %q", key, i, gotVals[i], wantVals[i])
					}
				}
			}

			for key := range gotQuery {
				if _, expected := tt.expectedQuery[key]; !expected {
					t.Errorf("unexpected query param %q", key)
				}
			}
		})
	}
}

func TestBuildHeaderParams(t *testing.T) {
	tests := []struct {
		name            string
		params          []core.Parameter
		paramValues     map[string]any
		expectedHeaders map[string]string
	}{
		{
			name: "single required header",
			params: []core.Parameter{
				{Name: "X-Request-ID", Location: core.ParameterLocationHeader, DataType: core.DataTypeString, Required: true},
			},
			paramValues:     map[string]any{"X-Request-ID": "req-123"},
			expectedHeaders: map[string]string{"X-Request-ID": "req-123"},
		},
		{
			name: "multiple headers",
			params: []core.Parameter{
				{Name: "X-Api-Version", Location: core.ParameterLocationHeader, DataType: core.DataTypeString, Required: true},
				{Name: "X-Trace-ID", Location: core.ParameterLocationHeader, DataType: core.DataTypeString, Required: true},
			},
			paramValues: map[string]any{
				"X-Api-Version": "v2",
				"X-Trace-ID":    "trace-456",
			},
			expectedHeaders: map[string]string{
				"X-Api-Version": "v2",
				"X-Trace-ID":    "trace-456",
			},
		},
		{
			name: "optional header omitted when nil",
			params: []core.Parameter{
				{Name: "X-Optional", Location: core.ParameterLocationHeader, DataType: core.DataTypeString, Required: false},
			},
			paramValues:     map[string]any{},
			expectedHeaders: map[string]string{},
		},
		{
			name: "required header uses effective value when nil",
			params: []core.Parameter{
				{Name: "X-Required", Location: core.ParameterLocationHeader, DataType: core.DataTypeString, Required: true, DefaultValue: "default-val"},
			},
			paramValues:     map[string]any{},
			expectedHeaders: map[string]string{"X-Required": "default-val"},
		},
		{
			name: "header does not override default headers when set explicitly",
			params: []core.Parameter{
				{Name: "Accept", Location: core.ParameterLocationHeader, DataType: core.DataTypeString, Required: true},
			},
			paramValues:     map[string]any{"Accept": "text/html"},
			expectedHeaders: map[string]string{"Accept": "text/html"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := core.Operation{
				Method:     "GET",
				Path:       "/resource",
				BaseURL:    "https://api.example.com",
				Parameters: tt.params,
			}

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			for key, want := range tt.expectedHeaders {
				got := req.Header.Get(key)
				if got != want {
					t.Errorf("header %q: got %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestBuildCookieParams(t *testing.T) {
	tests := []struct {
		name            string
		params          []core.Parameter
		paramValues     map[string]any
		expectedCookies map[string]string
	}{
		{
			name: "single cookie",
			params: []core.Parameter{
				{Name: "session_id", Location: core.ParameterLocationCookie, DataType: core.DataTypeString, Required: true},
			},
			paramValues:     map[string]any{"session_id": "abc123"},
			expectedCookies: map[string]string{"session_id": "abc123"},
		},
		{
			name: "optional cookie omitted when nil",
			params: []core.Parameter{
				{Name: "tracking", Location: core.ParameterLocationCookie, DataType: core.DataTypeString, Required: false},
			},
			paramValues:     map[string]any{},
			expectedCookies: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := core.Operation{
				Method:     "GET",
				Path:       "/resource",
				BaseURL:    "https://api.example.com",
				Parameters: tt.params,
			}

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			for name, want := range tt.expectedCookies {
				cookie, err := req.Cookie(name)
				if err != nil {
					t.Errorf("missing cookie %q: %v", name, err)
					continue
				}
				if cookie.Value != want {
					t.Errorf("cookie %q: got %q, want %q", name, cookie.Value, want)
				}
			}
		})
	}
}

func TestBuildDefaultHeaders(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Method:     "GET",
		Path:       "/test",
		BaseURL:    "https://api.example.com",
		Parameters: []core.Parameter{},
	}

	req, err := builder.Build(context.Background(), op, map[string]any{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	ua := req.Header.Get("User-Agent")
	if ua == "" {
		t.Error("expected User-Agent default header to be set")
	}

	accept := req.Header.Get("Accept")
	if accept == "" {
		t.Error("expected Accept default header to be set")
	}
}

func TestBuildDefaultMethodIsGET(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Method:     "",
		Path:       "/test",
		BaseURL:    "https://api.example.com",
		Parameters: []core.Parameter{},
	}

	req, err := builder.Build(context.Background(), op, map[string]any{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if req.Method != "GET" {
		t.Errorf("expected method GET, got %s", req.Method)
	}
}

func TestBuildMethodUppercased(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Method:     "post",
		Path:       "/test",
		BaseURL:    "https://api.example.com",
		Parameters: []core.Parameter{},
	}

	req, err := builder.Build(context.Background(), op, map[string]any{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if req.Method != "POST" {
		t.Errorf("expected method POST, got %s", req.Method)
	}
}

func TestBuildWithAuth(t *testing.T) {
	tests := []struct {
		name          string
		authConfig    *AuthConfig
		checkRequest  func(t *testing.T, req *http.Request)
	}{
		{
			name: "bearer token",
			authConfig: &AuthConfig{
				BearerToken: "my-token-123",
			},
			checkRequest: func(t *testing.T, req *http.Request) {
				got := req.Header.Get("Authorization")
				want := "Bearer my-token-123"
				if got != want {
					t.Errorf("Authorization header: got %q, want %q", got, want)
				}
			},
		},
		{
			name: "basic auth",
			authConfig: &AuthConfig{
				BasicUsername: "user",
				BasicPassword: "pass",
			},
			checkRequest: func(t *testing.T, req *http.Request) {
				u, p, ok := req.BasicAuth()
				if !ok {
					t.Fatal("expected basic auth to be set")
				}
				if u != "user" || p != "pass" {
					t.Errorf("basic auth: got %s:%s, want user:pass", u, p)
				}
			},
		},
		{
			name: "api key in header",
			authConfig: &AuthConfig{
				APIKey:       "key-abc",
				APIKeyHeader: "X-API-Key",
				APIKeyIn:     "header",
			},
			checkRequest: func(t *testing.T, req *http.Request) {
				got := req.Header.Get("X-API-Key")
				if got != "key-abc" {
					t.Errorf("API key header: got %q, want %q", got, "key-abc")
				}
			},
		},
		{
			name: "api key in query",
			authConfig: &AuthConfig{
				APIKey:       "key-xyz",
				APIKeyHeader: "api_key",
				APIKeyIn:     "query",
			},
			checkRequest: func(t *testing.T, req *http.Request) {
				got := req.URL.Query().Get("api_key")
				if got != "key-xyz" {
					t.Errorf("API key query: got %q, want %q", got, "key-xyz")
				}
			},
		},
		{
			name: "api key in cookie",
			authConfig: &AuthConfig{
				APIKey:       "key-cookie",
				APIKeyHeader: "auth_token",
				APIKeyIn:     "cookie",
			},
			checkRequest: func(t *testing.T, req *http.Request) {
				cookie, err := req.Cookie("auth_token")
				if err != nil {
					t.Fatalf("missing auth cookie: %v", err)
				}
				if cookie.Value != "key-cookie" {
					t.Errorf("API key cookie: got %q, want %q", cookie.Value, "key-cookie")
				}
			},
		},
		{
			name: "custom headers",
			authConfig: &AuthConfig{
				CustomHeaders: map[string]string{
					"X-Custom-Auth": "custom-value",
				},
			},
			checkRequest: func(t *testing.T, req *http.Request) {
				got := req.Header.Get("X-Custom-Auth")
				if got != "custom-value" {
					t.Errorf("custom header: got %q, want %q", got, "custom-value")
				}
			},
		},
		{
			name:       "nil auth config",
			authConfig: nil,
			checkRequest: func(t *testing.T, req *http.Request) {
				got := req.Header.Get("Authorization")
				if got != "" {
					t.Errorf("expected no Authorization header, got %q", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			if tt.authConfig != nil {
				builder.WithAuth(tt.authConfig)
			}

			op := core.Operation{
				Method:     "GET",
				Path:       "/secured",
				BaseURL:    "https://api.example.com",
				Parameters: []core.Parameter{},
			}

			req, err := builder.Build(context.Background(), op, map[string]any{})
			if err != nil {
				t.Fatalf("Build returned error: %v", err)
			}

			tt.checkRequest(t, req)
		})
	}
}

func TestBuildWithModifiedParam(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Method:  "GET",
		Path:    "/search",
		BaseURL: "https://api.example.com",
		Parameters: []core.Parameter{
			{Name: "q", Location: core.ParameterLocationQuery, DataType: core.DataTypeString, Required: true},
			{Name: "page", Location: core.ParameterLocationQuery, DataType: core.DataTypeInteger, Required: true},
		},
	}

	original := map[string]any{"q": "original", "page": 1}
	req, err := builder.BuildWithModifiedParam(context.Background(), op, "q", "modified", original)
	if err != nil {
		t.Fatalf("BuildWithModifiedParam returned error: %v", err)
	}

	gotQ := req.URL.Query().Get("q")
	if gotQ != "modified" {
		t.Errorf("modified param q: got %q, want %q", gotQ, "modified")
	}

	gotPage := req.URL.Query().Get("page")
	if gotPage != "1" {
		t.Errorf("unmodified param page: got %q, want %q", gotPage, "1")
	}

	if original["q"] != "original" {
		t.Error("original map was mutated")
	}
}

func TestGetDefaultParamValues(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Parameters: []core.Parameter{
			{Name: "name", DataType: core.DataTypeString, ExampleValue: "alice"},
			{Name: "count", DataType: core.DataTypeInteger, DefaultValue: 10},
			{Name: "active", DataType: core.DataTypeBoolean},
		},
	}

	values := builder.GetDefaultParamValues(op)

	if values["name"] != "alice" {
		t.Errorf("name: got %v, want alice", values["name"])
	}
	if values["count"] != 10 {
		t.Errorf("count: got %v, want 10", values["count"])
	}
	if values["active"] != true {
		t.Errorf("active: got %v, want true", values["active"])
	}
}

func TestBuildCombinedParamLocations(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Method:  "POST",
		Path:    "/orgs/{org_id}/users",
		BaseURL: "https://api.example.com",
		Parameters: []core.Parameter{
			{Name: "org_id", Location: core.ParameterLocationPath, DataType: core.DataTypeString, Required: true},
			{Name: "page", Location: core.ParameterLocationQuery, DataType: core.DataTypeInteger, Required: true},
			{Name: "X-Request-ID", Location: core.ParameterLocationHeader, DataType: core.DataTypeString, Required: true},
			{Name: "username", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
			{Name: "email", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
		},
	}

	paramValues := map[string]any{
		"org_id":       "acme",
		"page":         1,
		"X-Request-ID": "req-789",
		"username":     "newuser",
		"email":        "new@example.com",
	}

	req, err := builder.Build(context.Background(), op, paramValues)
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if req.URL.Path != "/orgs/acme/users" {
		t.Errorf("path: got %q, want /orgs/acme/users", req.URL.Path)
	}

	if req.URL.Query().Get("page") != "1" {
		t.Errorf("query page: got %q, want 1", req.URL.Query().Get("page"))
	}

	if req.Header.Get("X-Request-ID") != "req-789" {
		t.Errorf("header X-Request-ID: got %q, want req-789", req.Header.Get("X-Request-ID"))
	}

	body, _ := io.ReadAll(req.Body)
	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if bodyMap["username"] != "newuser" {
		t.Errorf("body username: got %v, want newuser", bodyMap["username"])
	}
	if bodyMap["email"] != "new@example.com" {
		t.Errorf("body email: got %v, want new@example.com", bodyMap["email"])
	}

	if req.Method != "POST" {
		t.Errorf("method: got %s, want POST", req.Method)
	}
}

func TestBuildBaseURLTrailingSlash(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Method:     "GET",
		Path:       "/test",
		BaseURL:    "https://api.example.com/",
		Parameters: []core.Parameter{},
	}

	req, err := builder.Build(context.Background(), op, map[string]any{})
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}

	if req.URL.String() != "https://api.example.com/test" {
		t.Errorf("got URL %q, want https://api.example.com/test", req.URL.String())
	}
}

func TestBuildRequestFunction(t *testing.T) {
	op := core.Operation{
		Method:  "GET",
		Path:    "/ping",
		BaseURL: "https://api.example.com",
		Parameters: []core.Parameter{
			{Name: "v", Location: core.ParameterLocationQuery, DataType: core.DataTypeString, Required: true},
		},
	}

	req, err := BuildRequest(context.Background(), op, map[string]any{"v": "1"})
	if err != nil {
		t.Fatalf("BuildRequest returned error: %v", err)
	}

	if req.URL.Query().Get("v") != "1" {
		t.Errorf("query v: got %q, want 1", req.URL.Query().Get("v"))
	}
}

func compareMaps(want, got map[string]any) error {
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			return fmt.Errorf("missing key %q", k)
		}

		wMap, wIsMap := wv.(map[string]any)
		gMap, gIsMap := gv.(map[string]any)
		if wIsMap && gIsMap {
			if err := compareMaps(wMap, gMap); err != nil {
				return fmt.Errorf("key %q: %w", k, err)
			}
			continue
		}

		wJSON, _ := json.Marshal(wv)
		gJSON, _ := json.Marshal(gv)
		if string(wJSON) != string(gJSON) {
			return fmt.Errorf("key %q: got %v, want %v", k, gv, wv)
		}
	}
	return nil
}
