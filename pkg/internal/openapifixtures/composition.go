// Package openapifixtures holds spec fixtures shared by the two OpenAPI parsers'
// tests. pkg/openapi feeds the playground, report and CLI while pkg/api/openapi feeds
// the scanner; the two describing a schema differently is the defect class these
// fixtures guard, so both are held to one list rather than to copies that can drift.
package openapifixtures

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Shape is one way a spec can describe a value indirectly. Every one of them used to
// be understood at a request body's root and ignored one level down, because the body
// root had a composition resolver and the nested walk read a schema's type, properties
// and items directly.
type Shape struct {
	Name       string                     `json:"name"`
	Schema     json.RawMessage            `json:"schema"`
	Components map[string]json.RawMessage `json:"components"`
}

// CompositionShapes returns every composition spelling both parsers have to resolve.
func CompositionShapes() ([]Shape, error) {
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("cannot locate the fixture directory")
	}
	path := filepath.Join(filepath.Dir(self), "composition_shapes.json")

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var shapes []Shape
	if err := json.Unmarshal(raw, &shapes); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(shapes) == 0 {
		return nil, fmt.Errorf("%s declares no shapes", path)
	}
	return shapes, nil
}

// Spec places the shape at a request body's root, one level down under a property, and
// as an array item, so what the three describe can be compared against each other.
func (s Shape) Spec() (string, error) {
	operation := func(schema any) map[string]any {
		return map[string]any{"post": map[string]any{
			"operationId": "op",
			"requestBody": map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": schema}}},
			"responses":   map[string]any{"200": map[string]any{"description": "ok"}},
		}}
	}
	wrap := func(value any) map[string]any {
		return map[string]any{"type": "object", "properties": map[string]any{"value": value}}
	}

	spec := map[string]any{
		"openapi": "3.1.0",
		"info":    map[string]any{"title": "Composition", "version": "1.0.0"},
		"servers": []any{map[string]any{"url": "http://api.test"}},
		"paths": map[string]any{
			"/root":   operation(s.Schema),
			"/nested": operation(wrap(s.Schema)),
			"/array":  operation(wrap(map[string]any{"type": "array", "items": s.Schema})),
		},
	}
	if len(s.Components) > 0 {
		spec["components"] = map[string]any{"schemas": s.Components}
	}

	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
