package payloads

import (
	"strings"
	"testing"

	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(v int) *int         { return &v }
func floatPtr(v float64) *float64 { return &v }

func TestGenerateBoundaryPayloads_IntegerWithMinMax(t *testing.T) {
	param := core.Parameter{
		Name:     "age",
		DataType: core.DataTypeInteger,
		Constraints: core.Constraints{
			Minimum: floatPtr(0),
			Maximum: floatPtr(120),
		},
	}

	payloads := core.GenerateBoundaryPayloads(param)
	require.NotEmpty(t, payloads)

	violations := extractViolationTypes(payloads)
	assert.Contains(t, violations, "exceeds_maximum")
	assert.Contains(t, violations, "far_exceeds_maximum")
	assert.Contains(t, violations, "below_minimum")
	assert.Contains(t, violations, "integer_overflow")
	assert.Contains(t, violations, "integer_underflow")
	assert.Contains(t, violations, "type_mismatch_string_for_integer")
	assert.Contains(t, violations, "type_mismatch_float_for_integer")

	for _, p := range payloads {
		if p.ViolationType == "exceeds_maximum" {
			val, ok := p.Value.(float64)
			assert.True(t, ok, "exceeds_maximum value should be float64")
			assert.Greater(t, val, float64(120))
		}
		if p.ViolationType == "below_minimum" {
			val, ok := p.Value.(float64)
			assert.True(t, ok, "below_minimum value should be float64")
			assert.Less(t, val, float64(0))
		}
	}
}

func TestGenerateBoundaryPayloads_IntegerMinPositive(t *testing.T) {
	param := core.Parameter{
		Name:     "quantity",
		DataType: core.DataTypeInteger,
		Constraints: core.Constraints{
			Minimum: floatPtr(1),
		},
	}

	payloads := core.GenerateBoundaryPayloads(param)
	violations := extractViolationTypes(payloads)

	assert.Contains(t, violations, "below_minimum")
	assert.Contains(t, violations, "negative_when_min_positive")
}

func TestGenerateBoundaryPayloads_StringWithMinMaxLength(t *testing.T) {
	param := core.Parameter{
		Name:     "username",
		DataType: core.DataTypeString,
		Constraints: core.Constraints{
			MinLength: intPtr(3),
			MaxLength: intPtr(20),
		},
	}

	payloads := core.GenerateBoundaryPayloads(param)
	require.NotEmpty(t, payloads)

	violations := extractViolationTypes(payloads)
	assert.Contains(t, violations, "exceeds_max_length")
	assert.Contains(t, violations, "far_exceeds_max_length")
	assert.Contains(t, violations, "below_min_length")
	assert.Contains(t, violations, "type_mismatch_integer_for_string")

	for _, p := range payloads {
		if p.ViolationType == "exceeds_max_length" {
			strVal, ok := p.Value.(string)
			assert.True(t, ok)
			assert.Greater(t, len(strVal), 20)
		}
		if p.ViolationType == "below_min_length" {
			strVal, ok := p.Value.(string)
			assert.True(t, ok)
			assert.Less(t, len(strVal), 3)
		}
	}
}

func TestGenerateBoundaryPayloads_StringWithPattern(t *testing.T) {
	param := core.Parameter{
		Name:     "code",
		DataType: core.DataTypeString,
		Constraints: core.Constraints{
			Pattern: "^[A-Z]{3}$",
		},
	}

	payloads := core.GenerateBoundaryPayloads(param)
	violations := extractViolationTypes(payloads)

	assert.Contains(t, violations, "pattern_mismatch")
}

func TestGenerateBoundaryPayloads_StringWithFormat(t *testing.T) {
	tests := []struct {
		name            string
		format          string
		expectedType    string
	}{
		{"email format", "email", "invalid_email_format"},
		{"uri format", "uri", "invalid_uri_format"},
		{"url format", "url", "invalid_uri_format"},
		{"uuid format", "uuid", "invalid_uuid_format"},
		{"date format", "date", "invalid_date_format"},
		{"date-time format", "date-time", "invalid_date_format"},
		{"ipv4 format", "ipv4", "invalid_ipv4_format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := core.Parameter{
				Name:     "field",
				DataType: core.DataTypeString,
				Constraints: core.Constraints{
					Format: tt.format,
				},
			}

			payloads := core.GenerateBoundaryPayloads(param)
			violations := extractViolationTypes(payloads)
			assert.Contains(t, violations, tt.expectedType)
		})
	}
}

func TestGenerateBoundaryPayloads_EnumParameter(t *testing.T) {
	param := core.Parameter{
		Name:     "status",
		DataType: core.DataTypeString,
		Constraints: core.Constraints{
			Enum: []any{"active", "inactive", "pending"},
		},
	}

	payloads := core.GenerateBoundaryPayloads(param)
	require.NotEmpty(t, payloads)

	violations := extractViolationTypes(payloads)
	assert.Contains(t, violations, "invalid_enum_value")
	assert.Contains(t, violations, "enum_with_leading_space")
	assert.Contains(t, violations, "modified_enum_value")

	for _, p := range payloads {
		if p.ViolationType == "invalid_enum_value" {
			strVal, ok := p.Value.(string)
			assert.True(t, ok)
			assert.Equal(t, "INVALID_ENUM_VALUE_XYZ123", strVal)
		}
		if p.ViolationType == "enum_with_leading_space" {
			strVal, ok := p.Value.(string)
			assert.True(t, ok)
			assert.True(t, strings.HasPrefix(strVal, " "))
		}
	}
}

func TestGenerateBoundaryPayloads_TypeMismatch(t *testing.T) {
	tests := []struct {
		name             string
		dataType         core.DataType
		expectedViolations []string
	}{
		{
			name:     "integer type mismatch",
			dataType: core.DataTypeInteger,
			expectedViolations: []string{
				"type_mismatch_string_for_integer",
				"type_mismatch_float_for_integer",
			},
		},
		{
			name:     "number type mismatch",
			dataType: core.DataTypeNumber,
			expectedViolations: []string{
				"type_mismatch_string_for_number",
			},
		},
		{
			name:     "boolean type mismatch",
			dataType: core.DataTypeBoolean,
			expectedViolations: []string{
				"type_mismatch_string_for_boolean",
				"type_mismatch_integer_for_boolean",
			},
		},
		{
			name:     "string type mismatch",
			dataType: core.DataTypeString,
			expectedViolations: []string{
				"type_mismatch_integer_for_string",
			},
		},
		{
			name:     "array type mismatch",
			dataType: core.DataTypeArray,
			expectedViolations: []string{
				"type_mismatch_string_for_array",
			},
		},
		{
			name:     "object type mismatch",
			dataType: core.DataTypeObject,
			expectedViolations: []string{
				"type_mismatch_string_for_object",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := core.Parameter{
				Name:     "field",
				DataType: tt.dataType,
			}

			payloads := core.GenerateBoundaryPayloads(param)
			violations := extractViolationTypes(payloads)

			for _, expected := range tt.expectedViolations {
				assert.Contains(t, violations, expected)
			}
		})
	}
}

func TestGenerateBoundaryPayloads_NoConstraints(t *testing.T) {
	tests := []struct {
		name              string
		dataType          core.DataType
		minExpectedCount  int
	}{
		{"string without constraints", core.DataTypeString, 1},
		{"integer without constraints", core.DataTypeInteger, 4},
		{"number without constraints", core.DataTypeNumber, 1},
		{"boolean without constraints", core.DataTypeBoolean, 2},
		{"array without constraints", core.DataTypeArray, 1},
		{"object without constraints", core.DataTypeObject, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := core.Parameter{
				Name:     "field",
				DataType: tt.dataType,
			}

			payloads := core.GenerateBoundaryPayloads(param)
			assert.GreaterOrEqual(t, len(payloads), tt.minExpectedCount,
				"expected at least %d payloads for %s, got %d", tt.minExpectedCount, tt.dataType, len(payloads))
		})
	}
}

func TestGenerateBoundaryPayloads_ArrayConstraints(t *testing.T) {
	param := core.Parameter{
		Name:     "items",
		DataType: core.DataTypeArray,
		Constraints: core.Constraints{
			MinItems: intPtr(1),
			MaxItems: intPtr(5),
		},
	}

	payloads := core.GenerateBoundaryPayloads(param)
	violations := extractViolationTypes(payloads)

	assert.Contains(t, violations, "exceeds_max_items")
	assert.Contains(t, violations, "below_min_items")
}

func TestBoundaryPayloadGenerator_Generate(t *testing.T) {
	gen := NewBoundaryPayloadGenerator()
	param := core.Parameter{
		Name:     "count",
		DataType: core.DataTypeInteger,
		Constraints: core.Constraints{
			Minimum: floatPtr(0),
			Maximum: floatPtr(100),
		},
	}

	payloads := gen.Generate(param)
	assert.NotEmpty(t, payloads)
	assert.Greater(t, len(payloads), 4)
}

func TestBoundaryPayloadGenerator_GenerateForOperation(t *testing.T) {
	gen := NewBoundaryPayloadGenerator()
	op := core.Operation{
		Parameters: []core.Parameter{
			{
				Name:     "name",
				DataType: core.DataTypeString,
				Constraints: core.Constraints{
					MaxLength: intPtr(50),
				},
			},
			{
				Name:     "age",
				DataType: core.DataTypeInteger,
				Constraints: core.Constraints{
					Minimum: floatPtr(0),
					Maximum: floatPtr(150),
				},
			},
			{
				Name:     "unconstrained",
				DataType: core.DataTypeString,
			},
		},
	}

	result := gen.GenerateForOperation(op)

	assert.Contains(t, result, "name")
	assert.Contains(t, result, "age")
	assert.NotContains(t, result, "unconstrained")
	assert.NotEmpty(t, result["name"])
	assert.NotEmpty(t, result["age"])
}

func TestBoundaryPayloadGenerator_GenerateForConstrainedParams(t *testing.T) {
	gen := NewBoundaryPayloadGenerator()
	op := core.Operation{
		Parameters: []core.Parameter{
			{
				Name:     "email",
				DataType: core.DataTypeString,
				Constraints: core.Constraints{
					Format: "email",
				},
			},
			{
				Name:     "unconstrained",
				DataType: core.DataTypeString,
			},
		},
	}

	sets := gen.GenerateForConstrainedParams(op)

	assert.Len(t, sets, 1)
	assert.Equal(t, "email", sets[0].Parameter.Name)
	assert.NotEmpty(t, sets[0].Payloads)
}

func TestGenerateTypeConfusionPayloads(t *testing.T) {
	tests := []struct {
		name         string
		dataType     core.DataType
		minCount     int
		checkPayload func(t *testing.T, payloads []TypeConfusionPayload)
	}{
		{
			name:     "string field",
			dataType: core.DataTypeString,
			minCount: 4,
			checkPayload: func(t *testing.T, payloads []TypeConfusionPayload) {
				types := make([]string, 0, len(payloads))
				for _, p := range payloads {
					types = append(types, p.ActualType)
				}
				assert.Contains(t, types, "integer")
				assert.Contains(t, types, "boolean")
				assert.Contains(t, types, "array")
				assert.Contains(t, types, "object")
			},
		},
		{
			name:     "integer field",
			dataType: core.DataTypeInteger,
			minCount: 3,
			checkPayload: func(t *testing.T, payloads []TypeConfusionPayload) {
				types := make([]string, 0, len(payloads))
				for _, p := range payloads {
					types = append(types, p.ActualType)
				}
				assert.Contains(t, types, "string")
				assert.Contains(t, types, "float")
				assert.Contains(t, types, "string_overflow")
			},
		},
		{
			name:     "number field",
			dataType: core.DataTypeNumber,
			minCount: 3,
			checkPayload: func(t *testing.T, payloads []TypeConfusionPayload) {
				types := make([]string, 0, len(payloads))
				for _, p := range payloads {
					types = append(types, p.ActualType)
				}
				assert.Contains(t, types, "string")
				assert.Contains(t, types, "nan_string")
				assert.Contains(t, types, "infinity_string")
			},
		},
		{
			name:     "boolean field",
			dataType: core.DataTypeBoolean,
			minCount: 3,
			checkPayload: func(t *testing.T, payloads []TypeConfusionPayload) {
				types := make([]string, 0, len(payloads))
				for _, p := range payloads {
					types = append(types, p.ActualType)
				}
				assert.Contains(t, types, "string")
				assert.Contains(t, types, "integer")
				assert.Contains(t, types, "truthy_string")
			},
		},
		{
			name:     "array field",
			dataType: core.DataTypeArray,
			minCount: 2,
			checkPayload: func(t *testing.T, payloads []TypeConfusionPayload) {
				types := make([]string, 0, len(payloads))
				for _, p := range payloads {
					types = append(types, p.ActualType)
				}
				assert.Contains(t, types, "string")
				assert.Contains(t, types, "object")
			},
		},
		{
			name:     "object field",
			dataType: core.DataTypeObject,
			minCount: 2,
			checkPayload: func(t *testing.T, payloads []TypeConfusionPayload) {
				types := make([]string, 0, len(payloads))
				for _, p := range payloads {
					types = append(types, p.ActualType)
				}
				assert.Contains(t, types, "string")
				assert.Contains(t, types, "array")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			param := core.Parameter{
				Name:     "field",
				DataType: tt.dataType,
			}

			payloads := GenerateTypeConfusionPayloads(param)
			assert.GreaterOrEqual(t, len(payloads), tt.minCount)

			for _, p := range payloads {
				assert.Equal(t, string(tt.dataType), p.ExpectedType)
				assert.NotEmpty(t, p.Description)
				assert.NotEmpty(t, p.ActualType)
			}

			if tt.checkPayload != nil {
				tt.checkPayload(t, payloads)
			}
		})
	}
}

func TestGenerateFormatBypassPayloads(t *testing.T) {
	tests := []struct {
		format       string
		minPayloads  int
	}{
		{"email", 7},
		{"uri", 5},
		{"url", 5},
		{"uuid", 5},
		{"date", 5},
		{"date-time", 4},
		{"ipv4", 5},
		{"ipv6", 3},
		{"hostname", 3},
		{"unknown", 0},
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			payloads := GenerateFormatBypassPayloads(tt.format)
			assert.Len(t, payloads, tt.minPayloads, "format %q", tt.format)

			for _, p := range payloads {
				assert.NotEmpty(t, p.Description)
			}
		})
	}
}

func TestGenerateNullValuePayloads(t *testing.T) {
	payloads := GenerateNullValuePayloads()
	assert.Len(t, payloads, 6)

	hasNil := false
	for _, p := range payloads {
		if p == nil {
			hasNil = true
			break
		}
	}
	assert.True(t, hasNil, "null value payloads should include nil")
}

func TestGenerateSpecialCharacterPayloads(t *testing.T) {
	payloads := GenerateSpecialCharacterPayloads()
	assert.NotEmpty(t, payloads)
	assert.Contains(t, payloads, "\x00")
	assert.Contains(t, payloads, "\n")
	assert.Contains(t, payloads, "<")
	assert.Contains(t, payloads, ">")
	assert.Contains(t, payloads, "'")
}

func TestGenerateOverflowPayloads(t *testing.T) {
	payloads := GenerateOverflowPayloads()
	assert.NotEmpty(t, payloads)

	hasLongString := false
	for _, p := range payloads {
		if len(p) >= 1000 {
			hasLongString = true
			break
		}
	}
	assert.True(t, hasLongString, "overflow payloads should include long strings")
}

func TestBoundaryPayload_StructFields(t *testing.T) {
	payload := core.BoundaryPayload{
		Value:          "test-value",
		ViolationType:  "test_violation",
		ExpectedResult: "should be rejected",
	}

	assert.Equal(t, "test-value", payload.Value)
	assert.Equal(t, "test_violation", payload.ViolationType)
	assert.Equal(t, "should be rejected", payload.ExpectedResult)
}

func extractViolationTypes(payloads []core.BoundaryPayload) []string {
	violations := make([]string, 0, len(payloads))
	for _, p := range payloads {
		violations = append(violations, p.ViolationType)
	}
	return violations
}
