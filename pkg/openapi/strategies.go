package openapi

import (
	"math"
	"strings"
)

// DefaultValueStrategy generates a single valid value based on the schema
type DefaultValueStrategy struct{}

func (s *DefaultValueStrategy) Generate(schema map[string]interface{}) []GeneratedValue {
	val := generateDefaultValue(schema)
	return []GeneratedValue{{Value: val, Description: "Default value"}}
}

// InterestingValuesStrategy generates multiple values including boundaries and common fuzzing payloads
type InterestingValuesStrategy struct{}

func (s *InterestingValuesStrategy) Generate(schema map[string]interface{}) []GeneratedValue {
	var values []GeneratedValue

	// Always include the default value as a baseline
	values = append(values, GeneratedValue{Value: generateDefaultValue(schema), Description: "Baseline valid value"})

	typ, _ := schema["type"].(string)
	format, _ := schema["format"].(string)

	switch typ {
	case "integer":
		values = append(values,
			GeneratedValue{Value: 0, Description: "Integer: 0"},
			GeneratedValue{Value: -1, Description: "Integer: -1"},
			GeneratedValue{Value: 1, Description: "Integer: 1"},
			GeneratedValue{Value: math.MaxInt32, Description: "Integer: MaxInt32"},
			GeneratedValue{Value: math.MinInt32, Description: "Integer: MinInt32"},
		)
		if format == "int64" {
			values = append(values,
				GeneratedValue{Value: math.MaxInt64, Description: "Integer: MaxInt64"},
				GeneratedValue{Value: math.MinInt64, Description: "Integer: MinInt64"},
			)
		}
	case "number":
		values = append(values,
			GeneratedValue{Value: 0.0, Description: "Number: 0.0"},
			GeneratedValue{Value: -1.5, Description: "Number: -1.5"},
			GeneratedValue{Value: 3.14, Description: "Number: 3.14"},
			GeneratedValue{Value: math.MaxFloat64, Description: "Number: MaxFloat64"},
			GeneratedValue{Value: math.SmallestNonzeroFloat64, Description: "Number: SmallestNonzeroFloat64"},
		)
	case "string":
		values = append(values,
			GeneratedValue{Value: "", Description: "String: Empty"},
			GeneratedValue{Value: "test", Description: "String: Simple"},
			GeneratedValue{Value: strings.Repeat("A", 1000), Description: "String: Long (1000 chars)"},
			GeneratedValue{Value: "null", Description: "String: null value"},
			GeneratedValue{Value: "undefined", Description: "String: undefined value"},
		)
		// Add format specific strings if needed (email, uuid, etc)
	case "boolean":
		values = append(values,
			GeneratedValue{Value: true, Description: "Boolean: true"},
			GeneratedValue{Value: false, Description: "Boolean: false"},
		)
	}

	return values
}

func generateDefaultValue(schema map[string]interface{}) interface{} {
	if schema == nil {
		return "test" // Fallback for nil schema
	}
	if example, ok := schema["example"]; ok && example != nil {
		return example
	}
	if def, ok := schema["default"]; ok && def != nil {
		return def
	}

	typ, _ := schema["type"].(string)
	switch typ {
	case "string":
		return "string_value"
	case "integer":
		return 1
	case "number":
		return 1.1
	case "boolean":
		return true
	case "array":
		if items, ok := schema["items"].(map[string]interface{}); ok {
			// Generate a single item for the array to show structure
			return []interface{}{generateDefaultValue(items)}
		}
		return []interface{}{}
	case "object":
		res := make(map[string]interface{})
		if props, ok := schema["properties"].(map[string]interface{}); ok {
			for k, v := range props {
				if propSchema, ok := v.(map[string]interface{}); ok {
					res[k] = generateDefaultValue(propSchema)
				}
			}
		}
		return res
	default:
		// Unknown or missing type - default to string
		return "test"
	}
}
