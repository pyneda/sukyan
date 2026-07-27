package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/pyneda/sukyan/pkg/internal/openapifixtures"
)

func compositionSpecs(t *testing.T) map[string]string {
	t.Helper()
	shapes, err := openapifixtures.CompositionShapes()
	if err != nil {
		t.Fatal(err)
	}
	specs := make(map[string]string, len(shapes))
	for _, shape := range shapes {
		spec, err := shape.Spec()
		if err != nil {
			t.Fatalf("building the %s spec: %v", shape.Name, err)
		}
		specs[shape.Name] = spec
	}
	return specs
}

func decodeBody(t *testing.T, raw string) any {
	t.Helper()
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("body %q is not JSON: %v", raw, err)
	}
	return decoded
}

func wrappedValue(t *testing.T, body any, path string) any {
	t.Helper()
	object, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("%s body is %T, want an object wrapping the schema under test", path, body)
	}
	value, ok := object["value"]
	if !ok {
		t.Fatalf("%s body %+v has no value field", path, object)
	}
	return value
}

func encode(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// TestCompositionResolvesTheSameAtEveryDepth is the invariant the resolver exists for:
// what a schema describes cannot depend on where it sits. The regressions this
// replaces were all the same failure — a shape understood at a body root and
// serialised as null one level down.
func TestCompositionResolvesTheSameAtEveryDepth(t *testing.T) {
	for name, spec := range compositionSpecs(t) {
		t.Run(name, func(t *testing.T) {
			root := decodeBody(t, requestBody(t, buildFirst(t, spec, "http://api.test", "", "/root")))
			nested := wrappedValue(t, decodeBody(t, requestBody(t, buildFirst(t, spec, "http://api.test", "", "/nested"))), "/nested")
			array := wrappedValue(t, decodeBody(t, requestBody(t, buildFirst(t, spec, "http://api.test", "", "/array"))), "/array")

			if root == nil {
				t.Fatal("/root sent no body — the schema resolved to nothing")
			}
			if encode(t, root) != encode(t, nested) {
				t.Errorf("nested value = %s, root = %s", encode(t, nested), encode(t, root))
			}

			items, ok := array.([]any)
			if !ok || len(items) != 1 {
				t.Fatalf("/array value = %s, want a one-element array", encode(t, array))
			}
			if encode(t, root) != encode(t, items[0]) {
				t.Errorf("array item = %s, root = %s", encode(t, items[0]), encode(t, root))
			}
		})
	}
}

// A resolved value has to be something the endpoint can accept. Null leaves are what a
// cut composition used to serialise: the field is present, typed as nothing, and the
// API rejects the whole request before the handler is reached.
func TestCompositionEmitsNoNullLeaves(t *testing.T) {
	for name, spec := range compositionSpecs(t) {
		t.Run(name, func(t *testing.T) {
			for _, path := range []string{"/root", "/nested", "/array"} {
				body := requestBody(t, buildFirst(t, spec, "http://api.test", "", path))
				if strings.Contains(body, "null") {
					t.Errorf("%s body %s carries a null leaf", path, body)
				}
			}
		})
	}
}

// A mutually recursive union is cut by the on-path guard partway down. The cut used to
// leave the parameter with no data type at all, so a field the spec marks required
// went on the wire as JSON null and the endpoint rejected the request before the
// handler saw it. What the schema describes is known even where it cannot be expanded.
func TestCompositionTypesACutReferenceCycle(t *testing.T) {
	spec := `{
		"openapi": "3.1.0",
		"info": {"title": "Recursive union", "version": "1.0.0"},
		"servers": [{"url": "http://api.test"}],
		"paths": {"/eval": {"post": {
			"operationId": "eval",
			"requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Expr"}}}},
			"responses": {"200": {"description": "ok"}}
		}}},
		"components": {"schemas": {
			"Expr": {"type": "object", "required": ["op", "arg"], "properties": {
				"op": {"type": "string"},
				"arg": {"oneOf": [{"$ref": "#/components/schemas/Expr"}, {"$ref": "#/components/schemas/Lit"}]}
			}},
			"Lit": {"type": "object", "properties": {"value": {"type": "string"}}}
		}}
	}`

	operation := parseOnlyOperation(t, spec)
	var walk func(params []core.Parameter, path string)
	walk = func(params []core.Parameter, path string) {
		for _, param := range params {
			where := path + "." + param.Name
			if param.DataType == "" {
				t.Errorf("%s has no data type, so it serialises as null", where)
			}
			walk(param.NestedParams, where)
		}
	}
	walk(operation.Parameters, "")

	body := requestBody(t, buildFirst(t, spec, "http://api.test", "", "/eval"))
	if strings.Contains(body, "null") {
		t.Errorf("body %s carries a null leaf where the cycle was cut", body)
	}
}
