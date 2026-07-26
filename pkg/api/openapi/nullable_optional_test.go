package openapi

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
)

// FastAPI, and every other generator that emits OpenAPI 3.1, spells Optional[X] as
// anyOf:[{type:X},{type:null}]. Left unresolved the parameter carries no data type,
// so the body goes out with the field null and type-driven payloads skip it.
const nullableOptionalSpec = `{
	"openapi": "3.1.0",
	"info": {"title": "Nullable", "version": "1.0.0"},
	"servers": [{"url": "http://api.test"}],
	"paths": {
		"/register": {
			"post": {
				"operationId": "register",
				"parameters": [
					{"name": "trace", "in": "query", "schema": {"anyOf": [{"type": "integer"}, {"type": "null"}]}}
				],
				"requestBody": {
					"content": {
						"application/json": {
							"schema": {
								"type": "object",
								"required": ["username"],
								"properties": {
									"username": {"type": "string"},
									"email": {"anyOf": [{"type": "string", "format": "email"}, {"type": "null"}]},
									"age": {"anyOf": [{"type": "null"}, {"type": "integer"}]},
									"role": {"oneOf": [{"type": "string", "enum": ["admin", "user"]}, {"type": "null"}]},
									"tags": {"anyOf": [{"type": "array", "items": {"type": "string"}}, {"type": "null"}]}
								}
							}
						}
					}
				},
				"responses": {"200": {"description": "ok"}}
			}
		}
	}
}`

func parseOnlyOperation(t *testing.T, spec string) core.Operation {
	t.Helper()

	definition := &db.APIDefinition{
		Type:          db.APIDefinitionTypeOpenAPI,
		SourceURL:     "http://api.test/openapi.json",
		BaseURL:       "http://api.test",
		RawDefinition: []byte(spec),
	}

	operations, err := NewParser().Parse(definition)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(operations) != 1 {
		t.Fatalf("Parse() returned %d operations, want 1", len(operations))
	}
	return operations[0]
}

func paramByName(t *testing.T, op core.Operation, name string) core.Parameter {
	t.Helper()
	for _, param := range op.Parameters {
		if param.Name == name {
			return param
		}
	}
	t.Fatalf("operation has no parameter %q", name)
	return core.Parameter{}
}

func TestNullableOptionalResolvesToTypedBranch(t *testing.T) {
	op := parseOnlyOperation(t, nullableOptionalSpec)

	tests := []struct {
		param    string
		dataType core.DataType
		nullable bool
	}{
		{"trace", core.DataTypeInteger, true},
		{"email", core.DataTypeString, true},
		{"age", core.DataTypeInteger, true},
		{"role", core.DataTypeString, true},
		{"tags", core.DataTypeArray, true},
	}

	for _, tt := range tests {
		t.Run(tt.param, func(t *testing.T) {
			param := paramByName(t, op, tt.param)
			if param.DataType != tt.dataType {
				t.Errorf("DataType = %q, want %q", param.DataType, tt.dataType)
			}
			if param.Nullable != tt.nullable {
				t.Errorf("Nullable = %v, want %v", param.Nullable, tt.nullable)
			}
			if param.GetEffectiveValue() == nil {
				t.Error("GetEffectiveValue() = nil, want a value the endpoint can accept")
			}
		})
	}
}

func TestNullableOptionalKeepsBranchConstraints(t *testing.T) {
	op := parseOnlyOperation(t, nullableOptionalSpec)

	if got := paramByName(t, op, "email").Constraints.Format; got != "email" {
		t.Errorf("email format = %q, want email", got)
	}
	if got := len(paramByName(t, op, "role").Constraints.Enum); got != 2 {
		t.Errorf("role enum length = %d, want 2", got)
	}
	if got := paramByName(t, op, "role").GetEffectiveValue(); got != "admin" {
		t.Errorf("role value = %v, want admin", got)
	}
	if got := len(paramByName(t, op, "tags").NestedParams); got != 1 {
		t.Fatalf("tags nested params = %d, want 1 (the array item schema)", got)
	}
	if got := paramByName(t, op, "tags").NestedParams[0].DataType; got != core.DataTypeString {
		t.Errorf("tags item DataType = %q, want string", got)
	}
}
