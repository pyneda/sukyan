package openapi

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mustParse(t *testing.T, spec string, opts ...ParseOptions) *Document {
	t.Helper()
	options := ParseOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}
	doc, err := ParseWithOptions([]byte(spec), options)
	if err != nil {
		t.Fatalf("ParseWithOptions: %v", err)
	}
	return doc
}

func TestParseRejectsNonSpecInput(t *testing.T) {
	// A discovery crawler feeds arbitrary response bodies here. Anything without a
	// version field must be rejected rather than persisted as an empty definition.
	tests := []struct {
		name    string
		content string
	}{
		{"empty", ""},
		{"whitespace", "   \n\t "},
		{"plain json object", `{"hello":"world"}`},
		{"empty object", `{}`},
		{"json array", `[1,2,3]`},
		{"html", `<!DOCTYPE html><html><body>hi</body></html>`},
		{"truncated json", `{"openapi":"3.0.0","paths":{`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.content)); err == nil {
				t.Errorf("Parse(%q) succeeded, expected an error", tt.content)
			}
		})
	}
}

func TestParseRejectsOversizedDocument(t *testing.T) {
	content := []byte(`{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{}}`)
	if _, err := ParseWithOptions(content, ParseOptions{MaxSize: 10}); err == nil {
		t.Fatal("expected an error for a document over MaxSize")
	}
}

func TestParseAcceptsYAMLAndBOM(t *testing.T) {
	yamlSpec := `openapi: 3.0.0
info:
  title: YAML API
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /ping:
    get:
      responses:
        "200":
          description: ok
`

	for _, content := range [][]byte{[]byte(yamlSpec), append(append([]byte{}, utf8BOM...), []byte(yamlSpec)...)} {
		doc, err := Parse(content)
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if got := doc.BaseURL(); got != "https://api.example.com" {
			t.Errorf("BaseURL() = %q, want https://api.example.com", got)
		}
		if len(doc.Operations()) != 1 {
			t.Errorf("Operations() = %d, want 1", len(doc.Operations()))
		}
	}
}

const swagger2Spec = `{
  "swagger": "2.0",
  "info": {"title": "Legacy", "version": "1.0"},
  "host": "api.example.com",
  "basePath": "/v2",
  "schemes": ["https"],
  "securityDefinitions": {
    "basicAuth": {"type": "basic"},
    "apiKeyHeader": {"type": "apiKey", "name": "X-Token", "in": "header"}
  },
  "paths": {
    "/pets/{petId}": {
      "get": {
        "operationId": "getPet",
        "security": [{"basicAuth": []}],
        "parameters": [{"name": "petId", "in": "path", "required": true, "type": "integer"}],
        "responses": {"200": {"description": "ok"}}
      }
    },
    "/upload": {
      "post": {
        "consumes": ["application/x-www-form-urlencoded"],
        "parameters": [
          {"name": "username", "in": "formData", "type": "string", "required": true},
          {"name": "age", "in": "formData", "type": "integer", "required": true}
        ],
        "responses": {"200": {"description": "ok"}}
      }
    },
    "/public": {"get": {"security": [], "responses": {"200": {"description": "ok"}}}}
  }
}`

// TestParseSwagger2 covers the conversion to OpenAPI 3. Without it, host, basePath
// and schemes are dropped, so every discovered endpoint is registered against the
// wrong URL and the whole API goes untested.
func TestParseSwagger2(t *testing.T) {
	doc := mustParse(t, swagger2Spec)

	if got := doc.BaseURL(); got != "https://api.example.com/v2" {
		t.Errorf("BaseURL() = %q, want https://api.example.com/v2", got)
	}

	if got := len(doc.Operations()); got != 3 {
		t.Fatalf("Operations() = %d, want 3", got)
	}

	// "type: basic" must become the OpenAPI 3 form, otherwise the authentication
	// enforcement audit has no case for it and silently skips the endpoint.
	schemes := doc.GetSecuritySchemes()
	byName := make(map[string]SecurityScheme, len(schemes))
	for _, scheme := range schemes {
		byName[scheme.Name] = scheme
	}
	if basic := byName["basicAuth"]; basic.Type != "http" || basic.Scheme != "basic" {
		t.Errorf("basicAuth = {type:%q scheme:%q}, want {type:http scheme:basic}", basic.Type, basic.Scheme)
	}
	if apiKey := byName["apiKeyHeader"]; apiKey.Type != "apiKey" || apiKey.In != "header" || apiKey.ParameterName != "X-Token" {
		t.Errorf("apiKeyHeader = %+v, want apiKey in header named X-Token", apiKey)
	}
}

func TestGenerateRequestsSwagger2(t *testing.T) {
	doc := mustParse(t, swagger2Spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: doc.BaseURL()})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	byPath := endpointsByPath(endpoints)

	pet := requireEndpoint(t, byPath, "/pets/{petId}")
	if got := pet.Requests[0].URL; got != "https://api.example.com/v2/pets/1" {
		t.Errorf("path parameter URL = %q, want https://api.example.com/v2/pets/1", got)
	}
	if got := pet.Requests[0].Headers["Authorization"]; got != "Basic <BASE64_CREDENTIALS>" {
		t.Errorf("Authorization = %q, want Basic <BASE64_CREDENTIALS>", got)
	}

	// formData parameters convert into a form-encoded request body.
	upload := requireEndpoint(t, byPath, "/upload")
	if got := upload.Requests[0].Headers["Content-Type"]; got != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", got)
	}
	if got := string(upload.Requests[0].Body); got != "age=1&username=string_value" {
		t.Errorf("body = %q, want age=1&username=string_value", got)
	}
}

// TestParseOpenAPI31 pins the 3.1 keywords kin-openapi still models the 3.0 way.
// Before normalisation a numeric exclusiveMinimum failed to unmarshal and the entire
// definition was discarded.
func TestParseOpenAPI31(t *testing.T) {
	spec := `{
	  "openapi": "3.1.0",
	  "info": {"title": "Modern", "version": "1.0"},
	  "servers": [{"url": "https://api.example.com"}],
	  "paths": {
	    "/items": {
	      "get": {
	        "parameters": [
	          {"name": "count", "in": "query", "required": true, "schema": {"type": ["integer", "null"], "exclusiveMinimum": 0}},
	          {"name": "kind", "in": "query", "required": true, "schema": {"type": "string", "enum": ["alpha", "beta"]}},
	          {"name": "mode", "in": "query", "required": true, "schema": {"const": "fixed"}}
	        ],
	        "responses": {"200": {"description": "ok"}}
	      }
	    }
	  }
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: doc.BaseURL()})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %d, want 1", len(endpoints))
	}

	url := endpoints[0].Requests[0].URL
	// A ["integer","null"] union must resolve to integer, not null.
	if !strings.Contains(url, "count=1") {
		t.Errorf("URL %q should carry an integer count", url)
	}
	// enum and const values are known-good inputs and beat synthesised placeholders.
	if !strings.Contains(url, "kind=alpha") {
		t.Errorf("URL %q should use the first enum value", url)
	}
	if !strings.Contains(url, "mode=fixed") {
		t.Errorf("URL %q should use the const value", url)
	}
}

// TestParseRecursiveSchemaTerminates guards the crash that a self-referential schema
// used to cause. Go stack exhaustion is fatal and unrecoverable, so this must never
// regress.
func TestParseRecursiveSchemaTerminates(t *testing.T) {
	spec := `{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {
	    "/node": {
	      "post": {
	        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Node"}}}},
	        "responses": {"200": {"description": "ok"}}
	      }
	    }
	  },
	  "components": {
	    "schemas": {
	      "Node": {
	        "type": "object",
	        "properties": {
	          "name": {"type": "string"},
	          "parent": {"$ref": "#/components/schemas/Node"},
	          "children": {"type": "array", "items": {"$ref": "#/components/schemas/Node"}}
	        }
	      }
	    }
	  }
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target", FuzzingEnabled: true})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %d, want 1", len(endpoints))
	}
	if len(endpoints[0].Requests) == 0 {
		t.Fatal("expected at least one generated request")
	}
}

// TestParseRefFanoutIsBounded guards the expansion bomb: a small spec whose schemas
// reference each other several times per level expands combinatorially unless the
// walk has a node budget.
func TestParseRefFanoutIsBounded(t *testing.T) {
	var schemas strings.Builder
	schemas.WriteString(`"L0":{"type":"string"}`)
	const levels, fanout = 12, 4
	for level := 1; level <= levels; level++ {
		schemas.WriteString(fmt.Sprintf(`,"L%d":{"type":"object","properties":{`, level))
		for f := 0; f < fanout; f++ {
			if f > 0 {
				schemas.WriteString(",")
			}
			schemas.WriteString(fmt.Sprintf(`"p%d":{"$ref":"#/components/schemas/L%d"}`, f, level-1))
		}
		schemas.WriteString("}}")
	}

	spec := fmt.Sprintf(`{
	  "openapi": "3.0.0",
	  "info": {"title": "bomb", "version": "1"},
	  "paths": {"/b": {"post": {
	    "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/L%d"}}}},
	    "responses": {"200": {"description": "ok"}}
	  }}},
	  "components": {"schemas": {%s}}
	}`, levels, schemas.String())

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	body := endpoints[0].Requests[0].Body
	if len(body) > 1<<20 {
		t.Errorf("generated body is %d bytes; the schema walk budget should keep it small", len(body))
	}
}

// TestParseDoesNotFetchExternalRefs is the SSRF and local file disclosure guard.
// kin-openapi's default reader resolves $refs through http.DefaultClient and the
// filesystem, and specs come from scan targets.
func TestParseDoesNotFetchExternalRefs(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"string","example":"LEAKED"}`)
	}))
	defer server.Close()

	secretDir := t.TempDir()
	secretFile := filepath.Join(secretDir, "secret.json")
	if err := os.WriteFile(secretFile, []byte(`{"type":"string","example":"FILE_CONTENTS"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for name, ref := range map[string]string{
		"remote": server.URL + "/internal/secret.json",
		"file":   secretFile,
	} {
		t.Run(name, func(t *testing.T) {
			spec := fmt.Sprintf(`{
			  "openapi": "3.0.0",
			  "info": {"title": "x", "version": "1"},
			  "paths": {"/p": {"get": {
			    "parameters": [{"name": "q", "in": "query", "required": true, "schema": {"$ref": %q}}],
			    "responses": {"200": {"description": "ok"}}
			  }}}
			}`, ref)

			// The document still parses: dropping an unresolvable reference is better
			// than discarding every endpoint in the spec.
			doc := mustParse(t, spec)
			endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
			if err != nil {
				t.Fatalf("GenerateRequests: %v", err)
			}
			if len(endpoints) != 1 {
				t.Fatalf("endpoints = %d, want 1", len(endpoints))
			}
			for _, req := range endpoints[0].Requests {
				if strings.Contains(req.URL, "LEAKED") || strings.Contains(req.URL, "FILE_CONTENTS") {
					t.Errorf("external reference content reached the request: %s", req.URL)
				}
			}
		})
	}

	if got := atomic.LoadInt64(&hits); got != 0 {
		t.Errorf("parsing issued %d outbound requests, want 0", got)
	}
}

func TestParseFetchesSameOriginRefsOnly(t *testing.T) {
	var allowedHits, foreignHits int64

	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&foreignHits, 1)
		fmt.Fprint(w, `{"type":"string","example":"FOREIGN"}`)
	}))
	defer foreign.Close()

	var allowedURL string
	allowed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/shared.json" {
			atomic.AddInt64(&allowedHits, 1)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"type":"string","example":"SAME_ORIGIN"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer allowed.Close()
	allowedURL = allowed.URL

	spec := fmt.Sprintf(`{
	  "openapi": "3.0.0",
	  "info": {"title": "x", "version": "1"},
	  "paths": {"/p": {"get": {
	    "parameters": [
	      {"name": "same", "in": "query", "required": true, "schema": {"$ref": "shared.json"}},
	      {"name": "cross", "in": "query", "required": true, "schema": {"$ref": %q}}
	    ],
	    "responses": {"200": {"description": "ok"}}
	  }}}
	}`, foreign.URL+"/evil.json")

	doc := mustParse(t, spec, ParseOptions{SourceURL: allowedURL + "/openapi.json", AllowRemoteRefs: true})
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	if got := atomic.LoadInt64(&foreignHits); got != 0 {
		t.Errorf("cross-origin reference was fetched %d times, want 0", got)
	}
	if len(endpoints) != 1 {
		t.Fatalf("endpoints = %d, want 1", len(endpoints))
	}
}

func endpointsByPath(endpoints []Endpoint) map[string]Endpoint {
	byPath := make(map[string]Endpoint, len(endpoints))
	for _, endpoint := range endpoints {
		byPath[endpoint.Path] = endpoint
	}
	return byPath
}

func requireEndpoint(t *testing.T, byPath map[string]Endpoint, path string) Endpoint {
	t.Helper()
	endpoint, ok := byPath[path]
	if !ok {
		t.Fatalf("no endpoint generated for %s", path)
	}
	if len(endpoint.Requests) == 0 {
		t.Fatalf("no requests generated for %s", path)
	}
	return endpoint
}

func TestXMLRequestBody(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"x","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"application/xml":{"schema":{"type":"object","properties":{
	    "name":{"type":"string","default":"a<b"},"id":{"type":"integer"}}}}}},
	  "responses":{"200":{"description":"ok"}}}}}}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	request := endpoints[0].Requests[0]
	if got := request.Headers["Content-Type"]; got != "application/xml" {
		t.Errorf("Content-Type = %q, want application/xml", got)
	}
	body := string(request.Body)
	if !strings.Contains(body, "<name>") || !strings.Contains(body, "<id>1</id>") {
		t.Errorf("XML body %q is missing the declared elements", body)
	}
	if strings.Contains(body, "a<b") {
		t.Errorf("XML body %q does not escape the value", body)
	}
}

// TestSwagger2SecurityDefinitionsFallback covers the path taken when conversion to
// OpenAPI 3 fails (here an invalid host); the security schemes must still be
// recovered from the raw securityDefinitions so the auth model is not lost.
func TestSwagger2SecurityDefinitionsFallback(t *testing.T) {
	spec := `{
	  "swagger": "2.0",
	  "info": {"title": "Legacy", "version": "1.0"},
	  "host": "api.example.com/not-just-a-host",
	  "securityDefinitions": {"apiKeyHeader": {"type": "apiKey", "name": "X-Token", "in": "header", "description": "key"}},
	  "paths": {"/items": {"get": {"responses": {"200": {"description": "ok"}}}}}
	}`

	doc := mustParse(t, spec)

	if got := len(doc.Operations()); got != 1 {
		t.Errorf("Operations() = %d, want 1 (a bad host must not discard the paths)", got)
	}

	schemes := doc.GetSecuritySchemes()
	if len(schemes) != 1 {
		t.Fatalf("schemes = %d, want 1 recovered from securityDefinitions", len(schemes))
	}
	if schemes[0].Name != "apiKeyHeader" || schemes[0].Type != "apiKey" ||
		schemes[0].In != "header" || schemes[0].ParameterName != "X-Token" {
		t.Errorf("recovered scheme = %+v", schemes[0])
	}
}

// TestParseRefusesCrossOriginRedirect guards the SSRF hole that validating only the
// initial $ref URL leaves open: a target can answer a same-origin reference with a
// 302 to an internal address.
func TestParseRefusesCrossOriginRedirect(t *testing.T) {
	var internalHits int64
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&internalHits, 1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"type":"string","example":"INTERNAL_CREDENTIALS"}`)
	}))
	defer internal.Close()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir.json" {
			http.Redirect(w, r, internal.URL+"/latest/meta-data/creds", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer target.Close()

	spec := fmt.Sprintf(`{
	  "openapi":"3.0.0","info":{"title":"t","version":"1"},
	  "paths":{"/p":{"get":{
	    "parameters":[{"name":"q","in":"query","required":true,"schema":{"$ref":"%s/redir.json"}}],
	    "responses":{"200":{"description":"ok"}}}}}
	}`, target.URL)

	doc := mustParse(t, spec, ParseOptions{
		SourceURL:       target.URL + "/openapi.json",
		AllowRemoteRefs: true,
	})

	if got := atomic.LoadInt64(&internalHits); got != 0 {
		t.Errorf("the redirect target was fetched %d times, want 0", got)
	}

	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}
	for _, endpoint := range endpoints {
		for _, request := range endpoint.Requests {
			if strings.Contains(request.URL, "INTERNAL_CREDENTIALS") {
				t.Errorf("redirected content reached the request: %s", request.URL)
			}
		}
	}
}

// TestParseBoundsCompositionFanOut covers an allOf DAG. A depth limit and a cycle set
// are both insufficient: nothing is its own ancestor, so only a node budget stops the
// combinatorial expansion.
func TestParseBoundsCompositionFanOut(t *testing.T) {
	var schemas strings.Builder
	schemas.WriteString(`"L0":{"type":"object","properties":{"x":{"type":"string"}}}`)
	const levels, fanout = 12, 6
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
	  "paths":{"/b":{"post":{
	    "requestBody":{"content":{"application/json":{"schema":{"$ref":"#/components/schemas/L%d"}}}},
	    "responses":{"200":{"description":"ok"}}}}},
	  "components":{"schemas":{%s}}}`, levels, schemas.String())

	doc := mustParse(t, spec)

	done := make(chan int, 1)
	go func() {
		endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
		if err != nil || len(endpoints) == 0 || len(endpoints[0].Requests) == 0 {
			done <- -1
			return
		}
		done <- len(endpoints[0].Requests[0].Body)
	}()

	select {
	case size := <-done:
		if size < 0 {
			t.Fatal("GenerateRequests failed")
		}
		if size > 1<<20 {
			t.Errorf("body is %d bytes; composition expansion must stay bounded", size)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("GenerateRequests did not finish: composition expansion is unbounded")
	}
}

// TestParseOpenAPI31PreservesPropertyNames guards against the 3.1 normalisation
// treating a property NAMED like a keyword as the keyword itself.
func TestParseOpenAPI31PreservesPropertyNames(t *testing.T) {
	spec := `{
	  "openapi":"3.1.0","info":{"title":"t","version":"1"},
	  "servers":[{"url":"https://api.example.com"}],
	  "paths":{"/p":{"post":{
	    "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{
	      "exclusiveMinimum":{"type":"string"},
	      "const":{"type":"string"},
	      "ok":{"type":"string"}}}}}},
	    "responses":{"200":{"description":"ok"}}}}}
	}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	var body map[string]interface{}
	raw := endpoints[0].Requests[0].Body
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("body %s is not JSON: %v", raw, err)
	}
	for _, name := range []string{"exclusiveMinimum", "const", "ok"} {
		if _, ok := body[name]; !ok {
			t.Errorf("body %s lost the property %q", raw, name)
		}
	}
	for _, injected := range []string{"minimum", "enum"} {
		if _, ok := body[injected]; ok {
			t.Errorf("body %s gained a property %q invented by 3.1 normalisation", raw, injected)
		}
	}
}

func TestMultipartContentTypeCarriesBoundary(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"multipart/form-data":{"schema":{"type":"object","properties":{"f":{"type":"string"}}}}}},
	  "responses":{"200":{"description":"ok"}}}}}}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	request := endpoints[0].Requests[0]
	contentType := request.Headers["Content-Type"]
	if !strings.Contains(contentType, "boundary=") {
		t.Fatalf("Content-Type %q has no boundary parameter", contentType)
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("Content-Type %q is not parseable: %v", contentType, err)
	}
	if !strings.HasPrefix(string(request.Body), "--"+params["boundary"]) {
		t.Errorf("body does not start with the declared boundary %q", params["boundary"])
	}
}

func TestGeneratedBodyOmitsReadOnlyProperties(t *testing.T) {
	spec := `{"openapi":"3.0.0","info":{"title":"t","version":"1"},"paths":{"/p":{"post":{
	  "requestBody":{"required":true,"content":{"application/json":{"schema":{"type":"object","properties":{
	    "id":{"type":"integer","readOnly":true},"name":{"type":"string"}}}}}},
	  "responses":{"200":{"description":"ok"}}}}}}`

	doc := mustParse(t, spec)
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://target"})
	if err != nil {
		t.Fatalf("GenerateRequests: %v", err)
	}

	var body map[string]interface{}
	raw := endpoints[0].Requests[0].Body
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("body %s is not JSON: %v", raw, err)
	}
	if _, ok := body["id"]; ok {
		t.Errorf("body %s includes the readOnly property", raw)
	}
	if _, ok := body["name"]; !ok {
		t.Errorf("body %s is missing the writable property", raw)
	}
}
