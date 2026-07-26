package openapi

import (
	"encoding/json"
	"math"
	"testing"
)

func TestGenerateDefaultValue(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]interface{}
		expected interface{}
	}{
		{
			name:     "Nil schema",
			schema:   nil,
			expected: "test",
		},
		{
			name:     "Unknown type",
			schema:   map[string]interface{}{"type": "widget"},
			expected: "test",
		},
		{
			name:     "Example wins over type",
			schema:   map[string]interface{}{"type": "string", "example": "from-example"},
			expected: "from-example",
		},
		{
			name:     "Default wins over type",
			schema:   map[string]interface{}{"type": "string", "default": "from-default"},
			expected: "from-default",
		},
		{
			name:     "Enum is a known good value",
			schema:   map[string]interface{}{"type": "string", "enum": []interface{}{"alpha", "beta"}},
			expected: "alpha",
		},
		{
			name:     "Array without items",
			schema:   map[string]interface{}{"type": "array"},
			expected: []interface{}{},
		},
		{
			name: "Simple String",
			schema: map[string]interface{}{
				"type": "string",
			},
			expected: "string_value",
		},
		{
			name: "Simple Integer",
			schema: map[string]interface{}{
				"type": "integer",
			},
			expected: 1,
		},
		{
			name: "Object with Properties",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{"type": "string"},
					"age":  map[string]interface{}{"type": "integer"},
				},
			},
			expected: map[string]interface{}{
				"name": "string_value",
				"age":  1,
			},
		},
		{
			name: "Array of Strings",
			schema: map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			expected: []interface{}{"string_value"},
		},
		{
			name: "Nested Object",
			schema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"user": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{"type": "integer"},
						},
					},
				},
			},
			expected: map[string]interface{}{
				"user": map[string]interface{}{
					"id": 1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateDefaultValue(tt.schema)

			// Use JSON marshaling for comparison to handle map/slice types easily
			gotJSON, _ := json.Marshal(got)
			expJSON, _ := json.Marshal(tt.expected)

			if string(gotJSON) != string(expJSON) {
				t.Errorf("generateDefaultValue() = %v, want %v", string(gotJSON), string(expJSON))
			}
		})
	}
}

func TestGenerateDefaultValueTerminatesOnDeepSchema(t *testing.T) {
	// A schema map deeper than the walk limit must return rather than recurse until
	// the stack is exhausted.
	deepest := map[string]interface{}{"type": "string"}
	current := deepest
	for i := 0; i < maxSchemaDepth*4; i++ {
		current = map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"next": current},
		}
	}

	if got := generateDefaultValue(current); got == nil {
		t.Fatal("generateDefaultValue returned nil for a deeply nested schema")
	}
}

func TestInterestingValuesStrategy(t *testing.T) {
	tests := []struct {
		name         string
		schema       map[string]interface{}
		wantContains []interface{}
	}{
		{
			name:         "string boundaries",
			schema:       map[string]interface{}{"type": "string"},
			wantContains: []interface{}{"", "test", "null", "undefined"},
		},
		{
			name:         "integer boundaries",
			schema:       map[string]interface{}{"type": "integer"},
			wantContains: []interface{}{0, -1, 1, math.MaxInt32, math.MinInt32},
		},
		{
			name:         "int64 boundaries are typed",
			schema:       map[string]interface{}{"type": "integer", "format": "int64"},
			wantContains: []interface{}{int64(math.MaxInt64), int64(math.MinInt64)},
		},
		{
			name:         "boolean values",
			schema:       map[string]interface{}{"type": "boolean"},
			wantContains: []interface{}{true, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values := (&InterestingValuesStrategy{}).Generate(tt.schema)
			if len(values) == 0 {
				t.Fatal("no values generated")
			}
			if values[0].Description != "Baseline valid value" {
				t.Errorf("first value should be the baseline, got %q", values[0].Description)
			}

			seen := make(map[interface{}]bool, len(values))
			for _, value := range values {
				seen[value.Value] = true
			}
			for _, want := range tt.wantContains {
				if !seen[want] {
					t.Errorf("missing expected fuzz value %#v", want)
				}
			}
		})
	}
}

func TestInterestingValuesStrategyOnUntypedSchema(t *testing.T) {
	values := (&InterestingValuesStrategy{}).Generate(nil)
	if len(values) != 1 {
		t.Fatalf("values = %d, want just the baseline", len(values))
	}
}
