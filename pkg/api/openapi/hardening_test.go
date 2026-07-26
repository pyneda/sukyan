package openapi

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
)

// buildFirst parses a spec and builds the request for the operation at the given
// path, which is the whole chain the scanner actually exercises.
func buildFirst(t *testing.T, spec, baseURL, sourceURL, path string) *http.Request {
	t.Helper()

	definition := &db.APIDefinition{
		Type:          db.APIDefinitionTypeOpenAPI,
		RawDefinition: []byte(spec),
		BaseURL:       baseURL,
		SourceURL:     sourceURL,
	}
	operations, err := NewParser().Parse(definition)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	for _, operation := range operations {
		if path != "" && operation.Path != path {
			continue
		}
		builder := NewRequestBuilder()
		req, err := builder.Build(context.Background(), operation, builder.GetDefaultParamValues(operation))
		if err != nil {
			t.Fatalf("build error for %s %s: %v", operation.Method, operation.Path, err)
		}
		return req
	}
	t.Fatalf("no operation found for path %q", path)
	return nil
}

func requestBody(t *testing.T, req *http.Request) string {
	t.Helper()
	if req.Body == nil {
		return ""
	}
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	return string(raw)
}

// TestParse_PathItemLevelParameters covers parameters declared on the path item and
// shared by every operation. Ignoring them leaves the template unsubstituted, so the
// request goes to a literal /users/%7BuserId%7D and never reaches the handler.
func TestParse_PathItemLevelParameters(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "t", "version": "1"},
	  "paths": {
	    "/users/{userId}/posts/{postId}": {
	      "parameters": [
	        {"name": "userId", "in": "path", "required": true, "schema": {"type": "integer", "default": 7}},
	        {"name": "postId", "in": "path", "required": true, "schema": {"type": "string", "default": "abc"}}
	      ],
	      "get": {"responses": {"200": {"description": "OK"}}}
	    }
	  }
	}`

	req := buildFirst(t, spec, "https://api.example.com", "", "/users/{userId}/posts/{postId}")
	if got := req.URL.Path; got != "/users/7/posts/abc" {
		t.Errorf("path = %q, want /users/7/posts/abc", got)
	}
}

func TestParse_OperationParameterOverridesPathItemParameter(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "t", "version": "1"},
	  "paths": {
	    "/items": {
	      "parameters": [{"name": "limit", "in": "query", "required": true, "schema": {"type": "string", "default": "shared"}}],
	      "get": {
	        "parameters": [{"name": "limit", "in": "query", "required": true, "schema": {"type": "string", "default": "operation"}}],
	        "responses": {"200": {"description": "OK"}}
	      }
	    }
	  }
	}`

	operations := mustParse(t, spec)
	if len(operations) != 1 {
		t.Fatalf("operations = %d, want 1", len(operations))
	}
	queryParams := 0
	for _, param := range operations[0].Parameters {
		if param.Location == core.ParameterLocationQuery {
			queryParams++
			if param.DefaultValue != "operation" {
				t.Errorf("default = %v, want the operation-level value to win", param.DefaultValue)
			}
		}
	}
	if queryParams != 1 {
		t.Errorf("query parameters = %d, want 1 deduplicated parameter", queryParams)
	}
}

const swagger2HardeningSpec = `{
  "swagger": "2.0",
  "info": {"title": "Legacy", "version": "1.0"},
  "host": "api.example.com",
  "basePath": "/v2",
  "schemes": ["https"],
  "securityDefinitions": {
    "basicAuth": {"type": "basic"},
    "apiKeyHeader": {"type": "apiKey", "name": "X-Token", "in": "header"}
  },
  "security": [{"basicAuth": []}],
  "paths": {
    "/pets/{petId}": {"get": {
      "parameters": [{"name": "petId", "in": "path", "required": true, "type": "integer"}],
      "responses": {"200": {"description": "OK"}}}},
    "/upload": {"post": {
      "consumes": ["application/x-www-form-urlencoded"],
      "parameters": [
        {"name": "username", "in": "formData", "type": "string", "required": true},
        {"name": "age", "in": "formData", "type": "integer", "required": true}
      ],
      "responses": {"200": {"description": "OK"}}}},
    "/public": {"get": {"security": [], "responses": {"200": {"description": "OK"}}}}
  }
}`

// TestParse_Swagger2 covers conversion to OpenAPI 3. Without it host/basePath/schemes
// are dropped so requests have no scheme or host, formData parameters are silently
// turned into query parameters, and typed parameters lose their schema.
func TestParse_Swagger2(t *testing.T) {
	req := buildFirst(t, swagger2HardeningSpec, "", "", "/pets/{petId}")
	if got := req.URL.String(); got != "https://api.example.com/v2/pets/1" {
		t.Errorf("URL = %q, want https://api.example.com/v2/pets/1", got)
	}

	upload := buildFirst(t, swagger2HardeningSpec, "", "", "/upload")
	if got := upload.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
	}
	body := requestBody(t, upload)
	for _, field := range []string{"username=", "age="} {
		if !strings.Contains(body, field) {
			t.Errorf("form body %q is missing %q; formData parameters must become a body", body, field)
		}
	}
	if strings.Contains(upload.URL.RawQuery, "username") {
		t.Errorf("formData parameter leaked into the query string: %s", upload.URL.RawQuery)
	}
	if strings.Contains(body, "<nil>") {
		t.Errorf("form body %q rendered a nil value; converted parameters must carry a schema", body)
	}
}

// TestParse_Swagger2SecurityTypes pins the scheme types the auth-enforcement audit
// switches on. A Swagger 2.0 "basic" type has no case there, so leaving it
// unconverted makes that audit silently do nothing.
func TestParse_Swagger2SecurityTypes(t *testing.T) {
	operations := mustParse(t, swagger2HardeningSpec)

	byPath := map[string]core.Operation{}
	for _, operation := range operations {
		byPath[operation.Path] = operation
	}

	secured, ok := byPath["/pets/{petId}"]
	if !ok {
		t.Fatal("missing /pets/{petId}")
	}
	if len(secured.Security) != 1 {
		t.Fatalf("security requirements = %d, want 1", len(secured.Security))
	}
	if secured.Security[0].Name != "basicAuth" || secured.Security[0].Type != "http" {
		t.Errorf("requirement = %+v, want basicAuth with type http", secured.Security[0])
	}

	public, ok := byPath["/public"]
	if !ok {
		t.Fatal("missing /public")
	}
	if len(public.Security) != 0 {
		t.Errorf(`"security": [] must clear the inherited requirement, got %+v`, public.Security)
	}
}

func TestParse_OpenAPI31(t *testing.T) {
	spec := `{
	  "openapi": "3.1.0",
	  "info": {"title": "t", "version": "1"},
	  "servers": [{"url": "https://api.example.com"}],
	  "paths": {"/items": {"get": {
	    "parameters": [{"name": "count", "in": "query", "required": true, "schema": {"type": ["integer", "null"], "exclusiveMinimum": 0}}],
	    "responses": {"200": {"description": "OK"}}}}}
	}`

	// kin-openapi models exclusiveMinimum as a bool, so an unnormalised 3.1 document
	// fails to unmarshal and the whole definition is discarded.
	req := buildFirst(t, spec, "", "", "/items")
	if !strings.Contains(req.URL.RawQuery, "count=") {
		t.Errorf("query %q should carry the count parameter", req.URL.RawQuery)
	}
}

// TestParse_DoesNotFetchExternalRefs is the SSRF and local file disclosure guard.
// kin-openapi's default reader resolves $refs through http.DefaultClient and
// os.ReadFile, and RawDefinition comes from a scan target.
func TestParse_DoesNotFetchExternalRefs(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"string","example":"LEAKED_REMOTE"}`)
	}))
	defer server.Close()

	secretFile := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(secretFile, []byte(`{"type":"string","example":"LEAKED_FILE"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, ref := range map[string]string{
		"remote": server.URL + "/internal.json",
		"file":   secretFile,
	} {
		t.Run(name, func(t *testing.T) {
			spec := fmt.Sprintf(`{
			  "openapi": "3.0.0",
			  "info": {"title": "t", "version": "1"},
			  "paths": {"/p": {"get": {
			    "parameters": [{"name": "q", "in": "query", "required": true, "schema": {"$ref": %q}}],
			    "responses": {"200": {"description": "OK"}}}}}
			}`, ref)

			// The document must still yield its endpoint: dropping an unresolvable
			// reference beats discarding every operation in the spec.
			req := buildFirst(t, spec, "http://target", "", "/p")
			full := req.URL.String()
			if strings.Contains(full, "LEAKED_REMOTE") || strings.Contains(full, "LEAKED_FILE") {
				t.Errorf("external reference content reached the request: %s", full)
			}
		})
	}

	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Errorf("parsing issued %d outbound requests, want 0", got)
	}
}

func TestBuildURL_RejectsUnusableBaseURL(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/ping":{"get":{"responses":{"200":{"description":"OK"}}}}}}`

	for _, baseURL := range []string{"", "/api/v3", "api.example.com"} {
		operations, err := NewParser().Parse(&db.APIDefinition{
			Type:          db.APIDefinitionTypeOpenAPI,
			RawDefinition: []byte(spec),
			BaseURL:       baseURL,
		})
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		builder := NewRequestBuilder()
		// A scheme-less base URL cannot address anything; reporting it beats emitting
		// a request to "/ping" that silently fails later.
		if _, err := builder.Build(context.Background(), operations[0], nil); err == nil {
			t.Errorf("Build with base URL %q succeeded, want an error", baseURL)
		}
	}
}

func TestBuildURL_FillsUndeclaredPathPlaceholder(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/users/{id}":{"get":{"responses":{"200":{"description":"OK"}}}}}}`

	req := buildFirst(t, spec, "http://target", "", "/users/{id}")
	if got := req.URL.Path; got != "/users/1" {
		t.Errorf("path = %q, want /users/1 rather than a literal placeholder", got)
	}
}

// TestBuild_NonObjectRequestBody covers scalar and array bodies. The parser gives
// them a single "body" parameter; wrapping that in an object would send
// {"body": [...]} where the API expects [...].
func TestBuild_NonObjectRequestBody(t *testing.T) {
	tests := []struct {
		name     string
		schema   string
		wantBody string
	}{
		{"array", `{"type":"array","items":{"type":"string"}}`, `["test"]`},
		{"scalar", `{"type":"string"}`, `"test"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
			  "requestBody":{"required":true,"content":{"application/json":{"schema":%s}}},
			  "responses":{"200":{"description":"OK"}}}}}}`, tt.schema)

			req := buildFirst(t, spec, "http://target", "", "/p")
			if got := requestBody(t, req); got != tt.wantBody {
				t.Errorf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}

func TestBuild_ObjectPropertyNamedBodyStaysStructured(t *testing.T) {
	// "body" is the sentinel name the parser uses for a whole-body parameter; a real
	// property with that name must still be encoded as an object member.
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{
	    "body":{"type":"string"}}}}}},
	  "responses":{"200":{"description":"OK"}}}}}}`

	req := buildFirst(t, spec, "http://target", "", "/p")
	body := requestBody(t, req)

	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("body %s is not a JSON object: %v", body, err)
	}
	if _, ok := decoded["body"]; !ok {
		t.Errorf("body %s should keep the property named \"body\"", body)
	}
}

// TestBuild_ScalarXMLBodyIsWellFormed covers the shape FastAPI emits for a raw XML
// body: content application/xml with a bare "type":"string" schema. Sending the
// generated scalar as-is produces a body no XML parser accepts, so the endpoint
// answers 400 and every XML-bodied operation goes untested.
func TestBuild_ScalarXMLBodyIsWellFormed(t *testing.T) {
	spec := `{"openapi":"3.1.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"application/xml":{"schema":{"type":"string"}}}},
	  "responses":{"200":{"description":"OK"}}}}}}`

	req := buildFirst(t, spec, "http://target", "", "/p")
	if got := req.Header.Get("Content-Type"); got != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", got)
	}

	body := requestBody(t, req)
	if err := xml.Unmarshal([]byte(body), new(any)); err != nil {
		t.Errorf("xml body %q is not well-formed XML: %v", body, err)
	}
}

// TestBuild_NullFirstTypeArrayUsesTheRealType covers the OpenAPI 3.1 union pydantic
// emits for optional fields. Taking the first entry of ["null","integer"] types the
// field as null, which falls through to a string, so every optional field is sent as
// "test" and a typed API rejects the request.
func TestBuild_NullFirstTypeArrayUsesTheRealType(t *testing.T) {
	spec := `{"openapi":"3.1.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
	  "parameters":[{"name":"page","in":"query","required":true,"schema":{"type":["null","integer"]}}],
	  "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{
	    "count":{"type":["null","integer"]},
	    "flag":{"type":["null","boolean"]}}}}}},
	  "responses":{"200":{"description":"OK"}}}}}}`

	req := buildFirst(t, spec, "http://target", "", "/p")

	var body map[string]any
	if err := json.Unmarshal([]byte(requestBody(t, req)), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if _, ok := body["count"].(float64); !ok {
		t.Errorf("count = %#v, want a number", body["count"])
	}
	if _, ok := body["flag"].(bool); !ok {
		t.Errorf("flag = %#v, want a boolean", body["flag"])
	}
	if got := req.URL.Query().Get("page"); got == "test" {
		t.Errorf("page = %q, want an integer value", got)
	}
}

// TestParse_EmptyCompositionVariantIsSkipped covers a oneOf whose first variant is
// object-typed but carries no properties. Accepting it marks the body structured with
// zero fields, so the request goes out with no body at all.
func TestParse_EmptyCompositionVariantIsSkipped(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"application/json":{"schema":{"oneOf":[
	    {"type":"object"},
	    {"type":"object","required":["user","pass"],"properties":{"user":{"type":"string"},"pass":{"type":"string"}}}]}}}},
	  "responses":{"200":{"description":"OK"}}}}}}`

	req := buildFirst(t, spec, "http://target", "", "/p")

	var body map[string]any
	if err := json.Unmarshal([]byte(requestBody(t, req)), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	for _, name := range []string{"user", "pass"} {
		if _, ok := body[name]; !ok {
			t.Errorf("body %v is missing %q from the populated oneOf variant", body, name)
		}
	}
}

func TestBuild_ContentTypeMatchesEncoding(t *testing.T) {
	const objectSchema = `{"type":"object","properties":{"a":{"type":"string"}}}`

	tests := []struct {
		name        string
		contentType string
		schema      string
		wantHeader  string
		check       func(t *testing.T, body string)
	}{
		{
			name:        "xml",
			contentType: "application/xml",
			schema:      objectSchema,
			wantHeader:  "application/xml",
			check: func(t *testing.T, body string) {
				if !strings.HasPrefix(body, "<") {
					t.Errorf("xml body %q is not XML", body)
				}
				if !strings.Contains(body, "<a>") {
					t.Errorf("xml body %q is missing the declared element", body)
				}
			},
		},
		{
			name:        "text",
			contentType: "text/plain",
			schema:      `{"type":"string"}`,
			wantHeader:  "text/plain",
			check: func(t *testing.T, body string) {
				if strings.HasPrefix(body, "{") || strings.HasPrefix(body, `"`) {
					t.Errorf("text/plain body %q should be the raw value", body)
				}
			},
		},
		{
			name:        "vendor json",
			contentType: "application/vnd.api+json",
			schema:      objectSchema,
			wantHeader:  "application/vnd.api+json",
			check: func(t *testing.T, body string) {
				var decoded map[string]any
				if err := json.Unmarshal([]byte(body), &decoded); err != nil {
					t.Errorf("vendor json body %q is not JSON: %v", body, err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := fmt.Sprintf(`{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
			  "requestBody":{"required":true,"content":{%q:{"schema":%s}}},
			  "responses":{"200":{"description":"OK"}}}}}}`, tt.contentType, tt.schema)

			req := buildFirst(t, spec, "http://target", "", "/p")
			if got := req.Header.Get("Content-Type"); got != tt.wantHeader {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantHeader)
			}
			tt.check(t, requestBody(t, req))
		})
	}
}

// TestBuild_NumericValuesAreNotScientific guards a bug class this repository has hit
// before: Go's %v renders large floats as 1e+07, which numeric parsers reject.
func TestBuild_NumericValuesAreNotScientific(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"get":{
	  "parameters":[
	    {"name":"big","in":"query","required":true,"schema":{"type":"number","default":10000000}},
	    {"name":"small","in":"query","required":true,"schema":{"type":"number","default":0.25}}
	  ],
	  "responses":{"200":{"description":"OK"}}}}}}`

	req := buildFirst(t, spec, "http://target", "", "/p")
	query := req.URL.Query()

	if got := query.Get("big"); strings.ContainsAny(got, "eE") {
		t.Errorf("big = %q, want plain decimal notation", got)
	}
	if got := query.Get("small"); got != "0.25" {
		t.Errorf("small = %q, want 0.25", got)
	}
}

func TestBuild_StripsControlCharactersFromHeadersAndCookies(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"get":{
	  "parameters":[
	    {"name":"X-Evil\r\nInjected","in":"header","required":true,"schema":{"type":"string","default":"a\r\nX-Other: b"}},
	    {"name":"sid\r\n","in":"cookie","required":true,"schema":{"type":"string","default":"v\r\nx"}}
	  ],
	  "responses":{"200":{"description":"OK"}}}}}}`

	req := buildFirst(t, spec, "http://target", "", "/p")
	for name, values := range req.Header {
		if strings.ContainsAny(name, "\r\n") {
			t.Errorf("header name %q carries control characters", name)
		}
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n") {
				t.Errorf("header %q value %q carries control characters", name, value)
			}
		}
	}
}

func TestParse_RecursiveSchemaProducesBoundedRequest(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "t", "version": "1"},
	  "paths": {"/node": {"post": {
	    "requestBody": {"required": true, "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Node"}}}},
	    "responses": {"200": {"description": "OK"}}}}},
	  "components": {"schemas": {"Node": {"type": "object", "properties": {
	    "name": {"type": "string"},
	    "parent": {"$ref": "#/components/schemas/Node"},
	    "children": {"type": "array", "items": {"$ref": "#/components/schemas/Node"}}}}}}
	}`

	req := buildFirst(t, spec, "http://target", "", "/node")
	if got := len(requestBody(t, req)); got > 1<<20 {
		t.Errorf("recursive schema produced a %d byte body; the walk must stay bounded", got)
	}
}

// TestParse_IsDeterministic pins that the same definition yields byte-identical
// requests. Ranging Go maps previously reordered operations, body properties and
// content types between runs.
func TestParse_IsDeterministic(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "t", "version": "1"},
	  "servers": [{"url": "https://api.example.com"}],
	  "paths": {
	    "/alpha": {"get": {"responses": {"200": {"description": "OK"}}}, "post": {
	      "requestBody": {"required": true, "content": {"application/x-www-form-urlencoded": {"schema": {"type": "object", "properties": {
	        "a": {"type": "string"}, "b": {"type": "string"}, "c": {"type": "string"}, "d": {"type": "string"}}}}}},
	      "responses": {"200": {"description": "OK"}}}},
	    "/beta": {"get": {"parameters": [{"name": "q", "in": "query", "required": true, "schema": {"type": "string"}}],
	      "responses": {"200": {"description": "OK"}}}}
	  }
	}`

	render := func() string {
		operations, err := NewParser().Parse(newTestDefinition(spec))
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		var rendered []string
		for _, operation := range operations {
			builder := NewRequestBuilder()
			req, err := builder.Build(context.Background(), operation, builder.GetDefaultParamValues(operation))
			if err != nil {
				t.Fatalf("build error: %v", err)
			}
			rendered = append(rendered, fmt.Sprintf("%s %s %s", req.Method, req.URL.String(), requestBody(t, req)))
		}
		encoded, err := json.Marshal(rendered)
		if err != nil {
			t.Fatal(err)
		}
		return string(encoded)
	}

	want := render()
	for i := 0; i < 25; i++ {
		if got := render(); got != want {
			t.Fatalf("run %d differed from run 0:\n%s\n%s", i+1, want, got)
		}
	}
}

func TestParse_RejectsNonSpecInput(t *testing.T) {
	// The crawler feeds arbitrary response bodies here; anything without a version
	// field must be an error rather than a definition with zero operations.
	for name, content := range map[string]string{
		"plain json": `{"hello":"world"}`,
		"empty":      `{}`,
		"html":       `<!DOCTYPE html><html></html>`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseFromRawDefinition([]byte(content)); err == nil {
				t.Errorf("ParseFromRawDefinition(%q) succeeded, want an error", content)
			}
		})
	}
}

// TestParse_RepeatedSchemaRefInSiblingBranches guards a silent data-loss bug: a type
// used twice in the same subtree used to be pruned by a visited set that was never
// unwound, so one of the two fields vanished from every request body (and which one
// depended on map iteration order).
func TestParse_RepeatedSchemaRefInSiblingBranches(t *testing.T) {
	spec := `{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/order":{"post":{
	    "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{
	      "customer":{"type":"object","properties":{
	        "billing":{"$ref":"#/components/schemas/Address"},
	        "shipping":{"$ref":"#/components/schemas/Address"}}}}}}}},
	    "responses":{"200":{"description":"OK"}}}}},
	  "components":{"schemas":{"Address":{"type":"object","properties":{
	    "street":{"type":"string"},"city":{"type":"string"}}}}}
	}`

	req := buildFirst(t, spec, "http://target", "", "/order")

	var decoded struct {
		Customer struct {
			Billing  map[string]any `json:"billing"`
			Shipping map[string]any `json:"shipping"`
		} `json:"customer"`
	}
	body := requestBody(t, req)
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("body %s is not JSON: %v", body, err)
	}
	if len(decoded.Customer.Billing) == 0 {
		t.Errorf("body %s dropped the billing address", body)
	}
	if len(decoded.Customer.Shipping) == 0 {
		t.Errorf("body %s dropped the shipping address", body)
	}
}

func TestParse_ContentTypesAreSortedAndDeduplicated(t *testing.T) {
	spec := `{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/p":{"post":{
	    "requestBody":{"required":true,"content":{
	      "application/json":{"schema":{"type":"object","properties":{"a":{"type":"string"}}}},
	      "application/xml":{"schema":{"type":"object","properties":{"a":{"type":"string"}}}},
	      "text/plain":{"schema":{"type":"string"}}}},
	    "responses":{
	      "200":{"description":"OK","content":{"application/json":{},"application/xml":{}}},
	      "404":{"description":"missing","content":{"application/json":{}}}}}}}
	}`

	want := `{"request":["application/json","application/xml","text/plain"],"response":["application/json","application/xml"]}`
	for i := 0; i < 25; i++ {
		operations, err := ParseFromRawDefinition([]byte(spec))
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		encoded, err := json.Marshal(operations[0].ContentTypes)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != want {
			t.Fatalf("run %d: content types = %s, want %s", i, encoded, want)
		}
	}
}

// TestBuild_QueryStyleAndExplode covers the OpenAPI serialization rules. Repeating a
// key for a non-exploded array, or JSON-encoding a deepObject, sends values the API
// parses differently from what the spec declares.
func TestBuild_QueryStyleAndExplode(t *testing.T) {
	spec := `{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/p":{"get":{"parameters":[
	    {"name":"csv","in":"query","required":true,"style":"form","explode":false,
	     "schema":{"type":"array","items":{"type":"string"},"default":["a","b"]}},
	    {"name":"pipe","in":"query","required":true,"style":"pipeDelimited","explode":false,
	     "schema":{"type":"array","items":{"type":"string"},"default":["a","b"]}},
	    {"name":"multi","in":"query","required":true,"style":"form","explode":true,
	     "schema":{"type":"array","items":{"type":"string"},"default":["a","b"]}},
	    {"name":"deep","in":"query","required":true,"style":"deepObject","explode":true,
	     "schema":{"type":"object","properties":{"k":{"type":"string"}}}}
	  ],"responses":{"200":{"description":"OK"}}}}}
	}`

	req := buildFirst(t, spec, "http://target", "", "/p")
	query := req.URL.Query()

	if got := query.Get("csv"); got != "a,b" {
		t.Errorf("csv = %q, want a,b", got)
	}
	if got := query.Get("pipe"); got != "a|b" {
		t.Errorf("pipe = %q, want a|b", got)
	}
	if got := query["multi"]; len(got) != 2 {
		t.Errorf("multi = %v, want two repeated values", got)
	}
	if got := query.Get("deep[k]"); got == "" {
		t.Errorf("deepObject parameter not expanded, query = %s", req.URL.RawQuery)
	}
}

// TestParse_ReadOnlyPropertiesAreNotSent covers properties the API populates itself;
// including them makes many APIs reject the whole request.
func TestParse_ReadOnlyPropertiesAreNotSent(t *testing.T) {
	spec := `{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/p":{"post":{
	    "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{
	      "id":{"type":"integer","readOnly":true},
	      "name":{"type":"string"}}}}}},
	    "responses":{"200":{"description":"OK"}}}}}
	}`

	req := buildFirst(t, spec, "http://target", "", "/p")

	var decoded map[string]any
	body := requestBody(t, req)
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatalf("body %s is not JSON: %v", body, err)
	}
	if _, ok := decoded["id"]; ok {
		t.Errorf("body %s includes the readOnly property", body)
	}
	if _, ok := decoded["name"]; !ok {
		t.Errorf("body %s is missing the writable property", body)
	}
}

func TestParse_ServerMetadataIsResolved(t *testing.T) {
	spec := `{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "servers":[{"url":"https://{env}.example.com/{ver}","variables":{"env":{"default":"prod"},"ver":{"default":"v2"}}}],
	  "paths":{"/p":{"get":{"responses":{"200":{"description":"OK"}}}}}
	}`

	operations, err := NewParser().Parse(&db.APIDefinition{
		Type:          db.APIDefinitionTypeOpenAPI,
		RawDefinition: []byte(spec),
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if got := operations[0].BaseURL; got != "https://prod.example.com/v2" {
		t.Errorf("base URL = %q, want the substituted server URL", got)
	}
	for _, server := range operations[0].OpenAPI.Servers {
		if strings.ContainsAny(server, "{}") {
			t.Errorf("server metadata %q still contains an unsubstituted variable", server)
		}
	}
}

// TestBuild_AuthConfigWinsOverSpecHeader pins that real credentials are not
// overwritten by a placeholder header parameter declared in the spec.
func TestBuild_AuthConfigWinsOverSpecHeader(t *testing.T) {
	spec := `{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/p":{"get":{"parameters":[
	    {"name":"Authorization","in":"header","required":true,"schema":{"type":"string","default":"SPEC_VALUE"}}
	  ],"responses":{"200":{"description":"OK"}}}}}
	}`

	operations, err := NewParser().Parse(&db.APIDefinition{
		Type:          db.APIDefinitionTypeOpenAPI,
		RawDefinition: []byte(spec),
		BaseURL:       "http://target",
	})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	builder := NewRequestBuilder().WithAuth(&AuthConfig{BearerToken: "REAL_TOKEN"})
	req, err := builder.Build(context.Background(), operations[0], builder.GetDefaultParamValues(operations[0]))
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer REAL_TOKEN" {
		t.Errorf("Authorization = %q, want the configured credential to win", got)
	}
}

// TestParse_BoundsCompositionFanOut covers an allOf DAG on the scan path. Neither a
// depth limit nor a cycle set stops it — nothing is its own ancestor — so only a node
// budget prevents the parse from running forever and holding the worker slot.
func TestParse_BoundsCompositionFanOut(t *testing.T) {
	var schemas strings.Builder
	schemas.WriteString(`"L0":{"type":"object","properties":{"x":{"type":"string"}}}`)
	const levels, fanout = 10, 6
	for level := 1; level <= levels; level++ {
		fmt.Fprintf(&schemas, `,"L%d":{"allOf":[`, level)
		for f := 0; f < fanout; f++ {
			if f > 0 {
				schemas.WriteString(",")
			}
			fmt.Fprintf(&schemas, `{"$ref":"#/components/schemas/L%d"}`, level-1)
		}
		schemas.WriteString("]}")
	}

	spec := fmt.Sprintf(`{
	  "openapi":"3.0.0","info":{"title":"bomb","version":"1"},
	  "servers":[{"url":"https://api.example.com"}],
	  "paths":{"/b":{"post":{
	    "requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/L%d"}}}},
	    "responses":{"200":{"description":"ok"}}}}},
	  "components":{"schemas":{%s}}}`, levels, schemas.String())

	done := make(chan error, 1)
	go func() {
		_, err := ParseFromRawDefinition([]byte(spec))
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Parse did not finish: allOf composition expansion is unbounded")
	}
}

// TestParse_BoundsNestedParameterExpansion covers a properties DAG. Tracking only the
// current path lets a type legitimately reused in sibling branches keep its nested
// parameters, but without a node budget the expansion is combinatorial.
func TestParse_BoundsNestedParameterExpansion(t *testing.T) {
	var schemas strings.Builder
	schemas.WriteString(`"L0":{"type":"string"}`)
	const levels, fanout = 9, 6
	for level := 1; level <= levels; level++ {
		fmt.Fprintf(&schemas, `,"L%d":{"type":"object","properties":{`, level)
		for f := 0; f < fanout; f++ {
			if f > 0 {
				schemas.WriteString(",")
			}
			fmt.Fprintf(&schemas, `"p%d":{"$ref":"#/components/schemas/L%d"}`, f, level-1)
		}
		schemas.WriteString("}}")
	}

	spec := fmt.Sprintf(`{
	  "openapi":"3.0.0","info":{"title":"dag","version":"1"},
	  "servers":[{"url":"https://api.example.com"}],
	  "paths":{"/d":{"post":{
	    "requestBody":{"content":{"application/json":{"schema":{"type":"object","properties":{
	      "root":{"$ref":"#/components/schemas/L%d"}}}}}},
	    "responses":{"200":{"description":"ok"}}}}},
	  "components":{"schemas":{%s}}}`, levels, schemas.String())

	var before, after runtime.MemStats
	done := make(chan error, 1)
	go func() {
		runtime.ReadMemStats(&before)
		_, err := ParseFromRawDefinition([]byte(spec))
		runtime.ReadMemStats(&after)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if allocated := (after.TotalAlloc - before.TotalAlloc) / (1 << 20); allocated > 512 {
			t.Errorf("parse allocated %d MB; nested parameter expansion must stay bounded", allocated)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Parse did not finish: nested parameter expansion is unbounded")
	}
}

// TestParse_BoundsExpansionAcrossTheWholeDocument covers the multiplication the
// per-traversal budget cannot see: every parameter gets its own full budget, so a
// spec that points hundreds of parameters at one wide $ref graph costs the product
// of the two. Only a document-wide ceiling bounds the parse.
func TestParse_BoundsExpansionAcrossTheWholeDocument(t *testing.T) {
	var schemas strings.Builder
	schemas.WriteString(`"L0":{"type":"string"}`)
	const levels, fanout = 9, 6
	for level := 1; level <= levels; level++ {
		fmt.Fprintf(&schemas, `,"L%d":{"type":"object","properties":{`, level)
		for f := 0; f < fanout; f++ {
			if f > 0 {
				schemas.WriteString(",")
			}
			fmt.Fprintf(&schemas, `"p%d":{"$ref":"#/components/schemas/L%d"}`, f, level-1)
		}
		schemas.WriteString("}}")
	}

	var params strings.Builder
	const paramCount = 300
	for i := 0; i < paramCount; i++ {
		if i > 0 {
			params.WriteString(",")
		}
		fmt.Fprintf(&params, `{"name":"q%d","in":"query","schema":{"$ref":"#/components/schemas/L%d"}}`, i, levels)
	}

	var paths strings.Builder
	const pathCount = 20
	for i := 0; i < pathCount; i++ {
		if i > 0 {
			paths.WriteString(",")
		}
		fmt.Fprintf(&paths, `"/p%d":{"get":{"parameters":[%s],"responses":{"200":{"description":"ok"}}}}`, i, params.String())
	}

	spec := fmt.Sprintf(`{
	  "openapi":"3.0.0","info":{"title":"fanout","version":"1"},
	  "servers":[{"url":"https://api.example.com"}],
	  "paths":{%s},
	  "components":{"schemas":{%s}}}`, paths.String(), schemas.String())

	done := make(chan struct {
		operations []core.Operation
		err        error
	}, 1)
	go func() {
		operations, err := ParseFromRawDefinition([]byte(spec))
		done <- struct {
			operations []core.Operation
			err        error
		}{operations, err}
	}()

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("parse error: %v", result.err)
		}
		total := 0
		for i := range result.operations {
			total += countParameterNodes(result.operations[i].Parameters)
		}
		// Without a document-wide ceiling this spec expands to pathCount * paramCount
		// * maxSchemaNodes nodes. The slack over the ceiling covers the parameters
		// materialised at each call site before the budget is consulted.
		if unbounded := pathCount * paramCount * maxSchemaNodes; total >= unbounded {
			t.Errorf("parse expanded %d parameter nodes, the unbounded product is %d", total, unbounded)
		}
		if total > 2*maxDocumentSchemaNodes {
			t.Errorf("parse expanded %d parameter nodes, want within the document budget of %d", total, maxDocumentSchemaNodes)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("Parse did not finish: expansion is unbounded across the document")
	}
}

func countParameterNodes(params []core.Parameter) int {
	total := 0
	for i := range params {
		total += 1 + countParameterNodes(params[i].NestedParams)
	}
	return total
}

func TestParse_SecurityRequirementOrderIsStable(t *testing.T) {
	spec := `{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/p":{"get":{
	    "security":[{"apiKeyQuery":[],"apiKeyHeader":[],"bearerAuth":[]}],
	    "responses":{"200":{"description":"OK"}}}}},
	  "components":{"securitySchemes":{
	    "apiKeyHeader":{"type":"apiKey","name":"X-Key","in":"header"},
	    "apiKeyQuery":{"type":"apiKey","name":"key","in":"query"},
	    "bearerAuth":{"type":"http","scheme":"bearer"}}}
	}`

	var want []string
	for i := 0; i < 25; i++ {
		operations, err := ParseFromRawDefinition([]byte(spec))
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		var names []string
		for _, requirement := range operations[0].Security {
			names = append(names, requirement.Name)
		}
		if want == nil {
			want = names
			continue
		}
		if strings.Join(names, ",") != strings.Join(want, ",") {
			t.Fatalf("run %d gave %v, run 0 gave %v", i, names, want)
		}
	}
}

// TestBuild_WholeBodyOnlyUnwrapsASingleBodyParameter guards the Structured flag
// against operations built by hand or stored before the field existed: the flag is
// false there while the body genuinely holds named fields.
func TestBuild_WholeBodyOnlyUnwrapsASingleBodyParameter(t *testing.T) {
	operation := core.Operation{
		Method:  "POST",
		Path:    "/p",
		BaseURL: "http://target",
		Parameters: []core.Parameter{
			{Name: "grant_type", Location: core.ParameterLocationBody, DataType: core.DataTypeString, DefaultValue: "password"},
			{Name: "username", Location: core.ParameterLocationBody, DataType: core.DataTypeString, DefaultValue: "u"},
		},
		OpenAPI: &core.OpenAPIMetadata{
			RequestBody: &core.RequestBodyInfo{ContentType: "application/x-www-form-urlencoded"},
		},
	}

	builder := NewRequestBuilder()
	req, err := builder.Build(context.Background(), operation, builder.GetDefaultParamValues(operation))
	if err != nil {
		t.Fatalf("build error: %v", err)
	}
	body := requestBody(t, req)
	for _, field := range []string{"grant_type=", "username="} {
		if !strings.Contains(body, field) {
			t.Errorf("form body %q lost field %q", body, field)
		}
	}
}
