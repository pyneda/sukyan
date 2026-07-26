package openapi

import (
	"context"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
)

func bodyParamsByName(ops []core.Operation) map[string]core.Parameter {
	out := map[string]core.Parameter{}
	if len(ops) == 0 {
		return out
	}
	for _, p := range ops[0].Parameters {
		if p.Location == core.ParameterLocationBody {
			out[p.Name] = p
		}
	}
	return out
}

// D13: composed schemas (allOf/oneOf/anyOf) must yield their real properties, not a
// single opaque "body" param with a null value.
func TestParse_ComposedRequestBodySchemas(t *testing.T) {
	t.Run("allOf merges all subschema properties", func(t *testing.T) {
		spec := `{
			"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"paths":{"/pets":{"post":{"requestBody":{"content":{"application/json":{"schema":{
				"allOf":[
					{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]},
					{"type":"object","properties":{"age":{"type":"integer"}}}
				]}}}},"responses":{"200":{"description":"OK"}}}}}
		}`
		params := bodyParamsByName(mustParse(t, spec))
		if _, ok := params["body"]; ok {
			t.Fatalf("allOf collapsed into opaque body param: %v", params)
		}
		if p, ok := params["name"]; !ok || p.DataType != core.DataTypeString || !p.Required {
			t.Errorf("expected required string 'name', got %+v (present=%v)", p, ok)
		}
		if p, ok := params["age"]; !ok || p.DataType != core.DataTypeInteger {
			t.Errorf("expected integer 'age', got %+v (present=%v)", p, ok)
		}
	})

	t.Run("oneOf uses the first object variant", func(t *testing.T) {
		spec := `{
			"openapi":"3.0.0","info":{"title":"t","version":"1"},
			"paths":{"/x":{"post":{"requestBody":{"content":{"application/json":{"schema":{
				"oneOf":[
					{"type":"object","properties":{"a":{"type":"string"}}},
					{"type":"object","properties":{"b":{"type":"integer"}}}
				]}}}},"responses":{"200":{"description":"OK"}}}}}
		}`
		params := bodyParamsByName(mustParse(t, spec))
		if _, ok := params["body"]; ok {
			t.Fatalf("oneOf collapsed into opaque body param: %v", params)
		}
		if _, ok := params["a"]; !ok {
			t.Errorf("expected property 'a' from first oneOf variant, got %v", params)
		}
	})
}

// D14: a value-less path param (e.g. a Swagger 2.0 param with type at the param
// level and thus no schema) must not render as the literal "<nil>".
func TestBuildURL_NilPathParamNeverRendersAsNil(t *testing.T) {
	op := core.Operation{
		Method:  "GET",
		Path:    "/pet/{petId}",
		BaseURL: "http://api.example.com",
		Parameters: []core.Parameter{
			{Name: "petId", Location: core.ParameterLocationPath}, // no DataType, no example
		},
	}

	req, err := BuildRequest(context.Background(), op, nil)
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}
	full := req.URL.String()
	if strings.Contains(full, "<nil>") || strings.Contains(full, "%3Cnil%3E") {
		t.Fatalf("path rendered a nil placeholder: %s", full)
	}
	if req.URL.Path != "/pet/1" {
		t.Errorf("expected /pet/1, got %s", req.URL.Path)
	}
}

// D15: a relative servers[0].url must be resolved against the definition source URL
// so the endpoint is reachable, and absolute server URLs must pass through unchanged.
func TestParse_RelativeServerURLResolvedAgainstSource(t *testing.T) {
	tests := []struct {
		name, server, source, wantBase string
	}{
		{"relative resolved against source", "/api/v3", "http://petstore.example.com/openapi.json", "http://petstore.example.com/api/v3"},
		{"absolute passes through", "https://api.example.com/v2", "http://petstore.example.com/openapi.json", "https://api.example.com/v2"},
		// A scheme-less base URL cannot be requested, so it is reported as absent
		// rather than carried forward; callers then fall back to a usable origin.
		{"relative with no usable source yields no base url", "/api/v3", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := `{
				"openapi":"3.0.0","info":{"title":"t","version":"1"},
				"servers":[{"url":"` + tt.server + `"}],
				"paths":{"/ping":{"get":{"responses":{"200":{"description":"OK"}}}}}
			}`
			def := &db.APIDefinition{
				Type:          db.APIDefinitionTypeOpenAPI,
				SourceURL:     tt.source,
				RawDefinition: []byte(spec),
			}
			ops, err := NewParser().Parse(def)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if len(ops) == 0 {
				t.Fatal("no operations parsed")
			}
			if ops[0].BaseURL != tt.wantBase {
				t.Errorf("base url = %q, want %q", ops[0].BaseURL, tt.wantBase)
			}
		})
	}
}

// D16: content-type selection must be deterministic and prefer application/json.
func TestSelectBodyContentType(t *testing.T) {
	jsonAndXML := openapi3.Content{
		"application/xml":  openapi3.NewMediaType(),
		"application/json": openapi3.NewMediaType(),
	}
	for i := 0; i < 50; i++ {
		if got := selectBodyContentType(jsonAndXML); got != "application/json" {
			t.Fatalf("iteration %d: want application/json, got %q", i, got)
		}
	}

	vendorJSON := openapi3.Content{
		"application/xml":          openapi3.NewMediaType(),
		"application/vnd.api+json": openapi3.NewMediaType(),
	}
	if got := selectBodyContentType(vendorJSON); got != "application/vnd.api+json" {
		t.Errorf("want vendor json, got %q", got)
	}

	noJSON := openapi3.Content{
		"text/xml":        openapi3.NewMediaType(),
		"application/xml": openapi3.NewMediaType(),
	}
	first := selectBodyContentType(noJSON)
	if first != "application/xml" { // lexicographically smallest, deterministic
		t.Errorf("want application/xml (smallest), got %q", first)
	}
	for i := 0; i < 50; i++ {
		if got := selectBodyContentType(noJSON); got != first {
			t.Fatalf("non-deterministic selection: %q vs %q", got, first)
		}
	}

	if got := selectBodyContentType(openapi3.Content{}); got != "" {
		t.Errorf("empty content should yield empty string, got %q", got)
	}
}

func mustParse(t *testing.T, spec string) []core.Operation {
	t.Helper()
	ops, err := NewParser().Parse(newTestDefinition(spec))
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	return ops
}
