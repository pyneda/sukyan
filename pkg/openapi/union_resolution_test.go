package openapi

import (
	"encoding/json"
	"strings"
	"testing"
)

func generateHappyPath(t *testing.T, spec string) []Endpoint {
	t.Helper()
	doc, err := ParseWithOptions([]byte(spec), ParseOptions{SourceURL: "http://api.test/openapi.json"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	endpoints, err := GenerateRequests(doc, GenerationConfig{BaseURL: "http://api.test", IncludeOptionalParams: true})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return endpoints
}

// objectSchema merges allOf before falling back to a oneOf/anyOf branch, so the union
// resolution has to run after it. Resolving the union first discards the allOf base of
// a base-plus-variant schema — exactly the fields such specs mark required.
func TestUnionResolutionKeepsAllOfBase(t *testing.T) {
	spec := `{
		"openapi": "3.0.3",
		"info": {"title": "Composed", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/p": {"get": {"operationId": "g", "parameters": [{"name": "filter", "in": "query", "schema": {
			"allOf": [{"type": "object", "properties": {"kind": {"type": "string"}, "id": {"type": "integer"}}}],
			"oneOf": [{"type": "object", "properties": {"bark": {"type": "string"}}},
			          {"type": "object", "properties": {"meow": {"type": "string"}}}]
		}}], "responses": {"200": {"description": "ok"}}}}}
	}`

	endpoints := generateHappyPath(t, spec)
	if len(endpoints) != 1 || len(endpoints[0].Parameters) != 1 {
		t.Fatalf("expected one endpoint with one parameter, got %+v", endpoints)
	}

	encoded, err := json.Marshal(endpoints[0].Parameters[0].Schema)
	if err != nil {
		t.Fatal(err)
	}
	schema := string(encoded)
	for _, want := range []string{`"kind"`, `"id"`} {
		if !strings.Contains(schema, want) {
			t.Errorf("parameter schema %s is missing the allOf base field %s", schema, want)
		}
	}
}

// The 3.1 nullable optional still has to resolve — that is what the union handling was
// added for, and it must survive being reordered after objectSchema.
func TestUnionResolutionStillTypesNullableOptionals(t *testing.T) {
	spec := `{
		"openapi": "3.1.0",
		"info": {"title": "Nullable", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/l": {"post": {"operationId": "l", "requestBody": {"content": {"application/json": {"schema": {
			"type": "object",
			"properties": {
				"age": {"anyOf": [{"type": "integer"}, {"type": "null"}]},
				"role": {"anyOf": [{"type": "string", "enum": ["admin", "user"]}, {"type": "null"}]}
			}
		}}}}, "responses": {"200": {"description": "ok"}}}}}
	}`

	endpoints := generateHappyPath(t, spec)
	var body string
	for _, request := range endpoints[0].Requests {
		if request.Label == "Happy Path" {
			body = string(request.Body)
		}
	}

	if !strings.Contains(body, `"age":1`) {
		t.Errorf("body %s types the nullable integer as something other than an integer", body)
	}
	if !strings.Contains(body, `"role":"admin"`) {
		t.Errorf("body %s ignores the nullable enum", body)
	}
}
