package openapi

import (
	"encoding/json"
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

// A model reached through a nullable wrapper is a cycle like any other. Before the
// wrapper was resolved the walk stopped at the untyped wrapper by accident; resolving
// it without carrying the branch's $ref through re-expanded the model at every depth
// level and drained the budget every later operation still needs.
func TestNullableOptionalCutsRecursiveReferences(t *testing.T) {
	spec := `{
		"openapi": "3.1.0",
		"info": {"title": "Recursive", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/n": {"post": {
			"operationId": "n",
			"requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Node"}}}},
			"responses": {"200": {"description": "ok"}}
		}}},
		"components": {"schemas": {"Node": {
			"type": "object",
			"properties": {
				"id": {"type": "integer"},
				"parent": {"anyOf": [{"$ref": "#/components/schemas/Node"}, {"type": "null"}]},
				"children": {"anyOf": [{"type": "array", "items": {"$ref": "#/components/schemas/Node"}}, {"type": "null"}]}
			}
		}}}
	}`

	op := parseOnlyOperation(t, spec)

	encoded, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// The unguarded walk produced 133075 bytes for this spec; a correct one-level
	// expansion is a few kilobytes. The bound is deliberately loose — it is guarding
	// against combinatorial blow-up, not pinning an exact encoding.
	if len(encoded) > 20000 {
		t.Errorf("operation encodes to %d bytes, want a bounded expansion — the reference cycle is not being cut", len(encoded))
	}

	if got := paramByName(t, op, "parent").DataType; got != core.DataTypeObject {
		t.Errorf("parent DataType = %q, want object", got)
	}
	if got := paramByName(t, op, "children").DataType; got != core.DataTypeArray {
		t.Errorf("children DataType = %q, want array", got)
	}
}

// JSON Schema allows validation keywords beside anyOf, where they constrain whichever
// branch applies. Resolving the wrapper must not discard them.
func TestNullableOptionalKeepsWrapperConstraints(t *testing.T) {
	spec := `{
		"openapi": "3.1.0",
		"info": {"title": "Wrapper", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/c": {"post": {
			"operationId": "c",
			"requestBody": {"content": {"application/json": {"schema": {"type": "object", "properties": {
				"role": {"anyOf": [{"type": "string"}, {"type": "null"}], "enum": ["admin", "user"], "maxLength": 8, "pattern": "^[a-z]+$"}
			}}}}},
			"responses": {"200": {"description": "ok"}}
		}}}
	}`

	role := paramByName(t, parseOnlyOperation(t, spec), "role")

	if got := len(role.Constraints.Enum); got != 2 {
		t.Errorf("wrapper enum length = %d, want 2", got)
	}
	if got := role.Constraints.Pattern; got != "^[a-z]+$" {
		t.Errorf("wrapper pattern = %q, want ^[a-z]+$", got)
	}
	if role.Constraints.MaxLength == nil || *role.Constraints.MaxLength != 8 {
		t.Errorf("wrapper maxLength = %v, want 8", role.Constraints.MaxLength)
	}
	if got := role.GetEffectiveValue(); got != "admin" {
		t.Errorf("role value = %v, want admin (the wrapper enum)", got)
	}
}

// Several generators emit an object branch without an explicit "type": object. The
// body-root resolver already expands such a schema, so the nested resolver must too —
// otherwise the same schema yields different requests depending on its depth.
func TestNullableOptionalExpandsUntypedObjectBranch(t *testing.T) {
	spec := `{
		"openapi": "3.1.0",
		"info": {"title": "Untyped", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/o": {"post": {
			"operationId": "o",
			"requestBody": {"content": {"application/json": {"schema": {"type": "object", "properties": {
				"addr": {"anyOf": [{"properties": {"street": {"type": "string"}}}, {"type": "null"}]}
			}}}}},
			"responses": {"200": {"description": "ok"}}
		}}}
	}`

	addr := paramByName(t, parseOnlyOperation(t, spec), "addr")

	if addr.DataType != core.DataTypeObject {
		t.Errorf("addr DataType = %q, want object", addr.DataType)
	}
	if len(addr.NestedParams) != 1 || addr.NestedParams[0].Name != "street" {
		t.Fatalf("addr nested params = %+v, want one named street", addr.NestedParams)
	}
	if addr.GetEffectiveValue() == nil {
		t.Error("addr GetEffectiveValue() = nil, want an object the endpoint can accept")
	}
}

// A union with no null branch resolves to one concrete member: a request the endpoint
// can accept, where an unresolved wrapper sends null. Nullable must stay false so the
// value is not treated as optional.
func TestNonNullUnionResolvesToAConcreteMember(t *testing.T) {
	spec := `{
		"openapi": "3.1.0",
		"info": {"title": "Union", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/u": {"post": {
			"operationId": "u",
			"requestBody": {"content": {"application/json": {"schema": {"type": "object", "properties": {
				"pay": {"oneOf": [{"$ref": "#/components/schemas/Card"}, {"$ref": "#/components/schemas/Bank"}]},
				"scalar": {"oneOf": [{"type": "string"}, {"type": "integer"}]}
			}}}}},
			"responses": {"200": {"description": "ok"}}
		}}},
		"components": {"schemas": {
			"Card": {"type": "object", "properties": {"kind": {"type": "string"}, "pan": {"type": "string"}}},
			"Bank": {"type": "object", "properties": {"kind": {"type": "string"}, "iban": {"type": "string"}}}
		}}
	}`

	op := parseOnlyOperation(t, spec)

	pay := paramByName(t, op, "pay")
	if pay.DataType != core.DataTypeObject {
		t.Errorf("pay DataType = %q, want object", pay.DataType)
	}
	if pay.Nullable {
		t.Error("pay Nullable = true, want false — the union declares no null branch")
	}
	names := make([]string, 0, len(pay.NestedParams))
	for _, nested := range pay.NestedParams {
		names = append(names, nested.Name)
	}
	if len(names) != 2 || names[0] != "kind" {
		t.Errorf("pay nested params = %v, want the first member's fields including its discriminator", names)
	}

	if got := paramByName(t, op, "scalar").DataType; got != core.DataTypeString {
		t.Errorf("scalar DataType = %q, want string", got)
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
