package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Constraints struct {
	MinLength    *int     `json:"min_length,omitempty"`
	MaxLength    *int     `json:"max_length,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`
	Minimum      *float64 `json:"minimum,omitempty"`
	Maximum      *float64 `json:"maximum,omitempty"`
	ExclusiveMin bool     `json:"exclusive_min,omitempty"`
	ExclusiveMax bool     `json:"exclusive_max,omitempty"`
	Enum         []any    `json:"enum,omitempty"`
	MinItems     *int     `json:"min_items,omitempty"`
	MaxItems     *int     `json:"max_items,omitempty"`
	Format       string   `json:"format,omitempty"`
}

func (c Constraints) IsEmpty() bool {
	return c.MinLength == nil &&
		c.MaxLength == nil &&
		c.Pattern == "" &&
		c.Minimum == nil &&
		c.Maximum == nil &&
		len(c.Enum) == 0 &&
		c.MinItems == nil &&
		c.MaxItems == nil &&
		c.Format == ""
}

func (c Constraints) HasStringConstraints() bool {
	return c.MinLength != nil || c.MaxLength != nil || c.Pattern != ""
}

func (c Constraints) HasNumericConstraints() bool {
	return c.Minimum != nil || c.Maximum != nil
}

func (c Constraints) HasEnumConstraints() bool {
	return len(c.Enum) > 0
}

func (c Constraints) HasArrayConstraints() bool {
	return c.MinItems != nil || c.MaxItems != nil
}

type BoundaryPayload struct {
	Value          any    `json:"value"`
	ViolationType  string `json:"violation_type"`
	ExpectedResult string `json:"expected_result"`
}

func GenerateBoundaryPayloads(param Parameter) []BoundaryPayload {
	var payloads []BoundaryPayload

	payloads = append(payloads, generateNumericBoundaryPayloads(param)...)
	payloads = append(payloads, generateStringBoundaryPayloads(param)...)
	payloads = append(payloads, generateEnumBoundaryPayloads(param)...)
	payloads = append(payloads, generateArrayBoundaryPayloads(param)...)
	payloads = append(payloads, generateTypeMismatchPayloads(param)...)

	return payloads
}

func generateNumericBoundaryPayloads(param Parameter) []BoundaryPayload {
	var payloads []BoundaryPayload

	if !param.DataType.IsNumeric() {
		return payloads
	}

	if param.Constraints.Maximum != nil {
		max := *param.Constraints.Maximum
		payloads = append(payloads, BoundaryPayload{
			Value:          max + 1,
			ViolationType:  "exceeds_maximum",
			ExpectedResult: fmt.Sprintf("value should be rejected: exceeds maximum of %v", max),
		})
		payloads = append(payloads, BoundaryPayload{
			Value:          max + 100,
			ViolationType:  "far_exceeds_maximum",
			ExpectedResult: fmt.Sprintf("value should be rejected: far exceeds maximum of %v", max),
		})
	}

	if param.Constraints.Minimum != nil {
		min := *param.Constraints.Minimum
		payloads = append(payloads, BoundaryPayload{
			Value:          min - 1,
			ViolationType:  "below_minimum",
			ExpectedResult: fmt.Sprintf("value should be rejected: below minimum of %v", min),
		})
		if min > 0 {
			payloads = append(payloads, BoundaryPayload{
				Value:          -1,
				ViolationType:  "negative_when_min_positive",
				ExpectedResult: "value should be rejected: negative value when minimum is positive",
			})
		}
	}

	if param.DataType == DataTypeInteger {
		payloads = append(payloads, BoundaryPayload{
			Value:          "9999999999999999999999999999999999999999",
			ViolationType:  "integer_overflow",
			ExpectedResult: "value should be rejected or handled: potential integer overflow",
		})
		payloads = append(payloads, BoundaryPayload{
			Value:          "-9999999999999999999999999999999999999999",
			ViolationType:  "integer_underflow",
			ExpectedResult: "value should be rejected or handled: potential integer underflow",
		})
	}

	return payloads
}

func generateStringBoundaryPayloads(param Parameter) []BoundaryPayload {
	var payloads []BoundaryPayload

	if param.DataType != DataTypeString {
		return payloads
	}

	if param.Constraints.MaxLength != nil {
		maxLen := *param.Constraints.MaxLength
		payload := strings.Repeat("a", maxLen+1)
		payloads = append(payloads, BoundaryPayload{
			Value:          payload,
			ViolationType:  "exceeds_max_length",
			ExpectedResult: fmt.Sprintf("value should be rejected: exceeds max length of %d", maxLen),
		})
		longPayload := strings.Repeat("a", maxLen*2)
		payloads = append(payloads, BoundaryPayload{
			Value:          longPayload,
			ViolationType:  "far_exceeds_max_length",
			ExpectedResult: fmt.Sprintf("value should be rejected: far exceeds max length of %d", maxLen),
		})
	}

	if param.Constraints.MinLength != nil {
		minLen := *param.Constraints.MinLength
		if minLen > 0 {
			payload := ""
			if minLen > 1 {
				payload = strings.Repeat("a", minLen-1)
			}
			payloads = append(payloads, BoundaryPayload{
				Value:          payload,
				ViolationType:  "below_min_length",
				ExpectedResult: fmt.Sprintf("value should be rejected: below min length of %d", minLen),
			})
		}
	}

	if param.Constraints.Pattern != "" {
		payloads = append(payloads, BoundaryPayload{
			Value:          "INVALID_PATTERN_VALUE_12345!@#$%",
			ViolationType:  "pattern_mismatch",
			ExpectedResult: "value should be rejected: does not match required pattern",
		})
	}

	switch param.Constraints.Format {
	case "email":
		payloads = append(payloads, BoundaryPayload{
			Value:          "not-an-email",
			ViolationType:  "invalid_email_format",
			ExpectedResult: "value should be rejected: invalid email format",
		})
	case "uri", "url":
		payloads = append(payloads, BoundaryPayload{
			Value:          "not-a-valid-url",
			ViolationType:  "invalid_uri_format",
			ExpectedResult: "value should be rejected: invalid URI format",
		})
	case "uuid":
		payloads = append(payloads, BoundaryPayload{
			Value:          "not-a-uuid",
			ViolationType:  "invalid_uuid_format",
			ExpectedResult: "value should be rejected: invalid UUID format",
		})
	case "date", "date-time":
		payloads = append(payloads, BoundaryPayload{
			Value:          "not-a-date",
			ViolationType:  "invalid_date_format",
			ExpectedResult: "value should be rejected: invalid date format",
		})
	case "ipv4":
		payloads = append(payloads, BoundaryPayload{
			Value:          "999.999.999.999",
			ViolationType:  "invalid_ipv4_format",
			ExpectedResult: "value should be rejected: invalid IPv4 format",
		})
	}

	return payloads
}

func generateEnumBoundaryPayloads(param Parameter) []BoundaryPayload {
	var payloads []BoundaryPayload

	if len(param.Constraints.Enum) == 0 {
		return payloads
	}

	payloads = append(payloads, BoundaryPayload{
		Value:          "INVALID_ENUM_VALUE_XYZ123",
		ViolationType:  "invalid_enum_value",
		ExpectedResult: "value should be rejected: not in allowed enum values",
	})

	for _, enumVal := range param.Constraints.Enum {
		strVal, ok := enumVal.(string)
		if ok && strVal != "" {
			payloads = append(payloads, BoundaryPayload{
				Value:          " " + strVal,
				ViolationType:  "enum_with_leading_space",
				ExpectedResult: "value should be rejected: enum value with leading whitespace",
			})
			payloads = append(payloads, BoundaryPayload{
				Value:          strings.ToUpper(strVal) + "_MODIFIED",
				ViolationType:  "modified_enum_value",
				ExpectedResult: "value should be rejected: modified enum value",
			})
			break
		}
	}

	return payloads
}

func generateArrayBoundaryPayloads(param Parameter) []BoundaryPayload {
	var payloads []BoundaryPayload

	if param.DataType != DataTypeArray {
		return payloads
	}

	if param.Constraints.MaxItems != nil {
		maxItems := *param.Constraints.MaxItems
		items := make([]string, maxItems+1)
		for i := range items {
			items[i] = "item"
		}
		jsonBytes, _ := json.Marshal(items)
		payloads = append(payloads, BoundaryPayload{
			Value:          string(jsonBytes),
			ViolationType:  "exceeds_max_items",
			ExpectedResult: fmt.Sprintf("value should be rejected: exceeds max items of %d", maxItems),
		})
	}

	if param.Constraints.MinItems != nil {
		minItems := *param.Constraints.MinItems
		if minItems > 0 {
			payloads = append(payloads, BoundaryPayload{
				Value:          "[]",
				ViolationType:  "below_min_items",
				ExpectedResult: fmt.Sprintf("value should be rejected: below min items of %d", minItems),
			})
		}
	}

	return payloads
}

func generateTypeMismatchPayloads(param Parameter) []BoundaryPayload {
	var payloads []BoundaryPayload

	switch param.DataType {
	case DataTypeInteger:
		payloads = append(payloads, BoundaryPayload{
			Value:          "not_a_number",
			ViolationType:  "type_mismatch_string_for_integer",
			ExpectedResult: "value should be rejected: string provided for integer field",
		})
		payloads = append(payloads, BoundaryPayload{
			Value:          3.14159,
			ViolationType:  "type_mismatch_float_for_integer",
			ExpectedResult: "value should be handled: float provided for integer field",
		})
	case DataTypeNumber:
		payloads = append(payloads, BoundaryPayload{
			Value:          "not_a_number",
			ViolationType:  "type_mismatch_string_for_number",
			ExpectedResult: "value should be rejected: string provided for number field",
		})
	case DataTypeBoolean:
		payloads = append(payloads, BoundaryPayload{
			Value:          "not_a_boolean",
			ViolationType:  "type_mismatch_string_for_boolean",
			ExpectedResult: "value should be rejected: string provided for boolean field",
		})
		payloads = append(payloads, BoundaryPayload{
			Value:          2,
			ViolationType:  "type_mismatch_integer_for_boolean",
			ExpectedResult: "value should be handled: integer provided for boolean field",
		})
	case DataTypeString:
		payloads = append(payloads, BoundaryPayload{
			Value:          12345,
			ViolationType:  "type_mismatch_integer_for_string",
			ExpectedResult: "value should be handled: integer provided for string field",
		})
	case DataTypeArray:
		payloads = append(payloads, BoundaryPayload{
			Value:          "not_an_array",
			ViolationType:  "type_mismatch_string_for_array",
			ExpectedResult: "value should be rejected: string provided for array field",
		})
	case DataTypeObject:
		payloads = append(payloads, BoundaryPayload{
			Value:          "not_an_object",
			ViolationType:  "type_mismatch_string_for_object",
			ExpectedResult: "value should be rejected: string provided for object field",
		})
	}

	return payloads
}
