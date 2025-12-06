package openapi

import (
	"encoding/json"
	"testing"
)

func TestGenerateDefaultValue(t *testing.T) {
	tests := []struct {
		name     string
		schema   map[string]interface{}
		expected interface{}
	}{
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
