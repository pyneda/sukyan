package payloads

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/pkg/api/core"
)

type BoundaryPayloadGenerator struct{}

func NewBoundaryPayloadGenerator() *BoundaryPayloadGenerator {
	return &BoundaryPayloadGenerator{}
}

func (g *BoundaryPayloadGenerator) Generate(param core.Parameter) []core.BoundaryPayload {
	return core.GenerateBoundaryPayloads(param)
}

func (g *BoundaryPayloadGenerator) GenerateForOperation(op core.Operation) map[string][]core.BoundaryPayload {
	payloads := make(map[string][]core.BoundaryPayload)

	for _, param := range op.Parameters {
		if param.HasConstraints() {
			payloads[param.Name] = g.Generate(param)
		}
	}

	return payloads
}

func (g *BoundaryPayloadGenerator) GenerateForConstrainedParams(op core.Operation) []ParameterPayloadSet {
	var sets []ParameterPayloadSet

	for _, param := range op.Parameters {
		if !param.HasConstraints() {
			continue
		}

		payloads := g.Generate(param)
		if len(payloads) > 0 {
			sets = append(sets, ParameterPayloadSet{
				Parameter: param,
				Payloads:  payloads,
			})
		}
	}

	return sets
}

type ParameterPayloadSet struct {
	Parameter core.Parameter
	Payloads  []core.BoundaryPayload
}

func GenerateTypeConfusionPayloads(param core.Parameter) []TypeConfusionPayload {
	var payloads []TypeConfusionPayload

	switch param.DataType {
	case core.DataTypeString:
		payloads = append(payloads, TypeConfusionPayload{
			Value:        12345,
			ExpectedType: "string",
			ActualType:   "integer",
			Description:  "integer provided for string field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        true,
			ExpectedType: "string",
			ActualType:   "boolean",
			Description:  "boolean provided for string field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        []string{"array", "value"},
			ExpectedType: "string",
			ActualType:   "array",
			Description:  "array provided for string field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        map[string]any{"key": "value"},
			ExpectedType: "string",
			ActualType:   "object",
			Description:  "object provided for string field",
		})

	case core.DataTypeInteger:
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "not_a_number",
			ExpectedType: "integer",
			ActualType:   "string",
			Description:  "string provided for integer field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        3.14159,
			ExpectedType: "integer",
			ActualType:   "float",
			Description:  "float provided for integer field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "9999999999999999999999999999",
			ExpectedType: "integer",
			ActualType:   "string_overflow",
			Description:  "overflow string provided for integer field",
		})

	case core.DataTypeNumber:
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "not_a_number",
			ExpectedType: "number",
			ActualType:   "string",
			Description:  "string provided for number field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "NaN",
			ExpectedType: "number",
			ActualType:   "nan_string",
			Description:  "NaN string provided for number field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "Infinity",
			ExpectedType: "number",
			ActualType:   "infinity_string",
			Description:  "Infinity string provided for number field",
		})

	case core.DataTypeBoolean:
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "not_a_boolean",
			ExpectedType: "boolean",
			ActualType:   "string",
			Description:  "string provided for boolean field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        2,
			ExpectedType: "boolean",
			ActualType:   "integer",
			Description:  "non-binary integer provided for boolean field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "yes",
			ExpectedType: "boolean",
			ActualType:   "truthy_string",
			Description:  "truthy string provided for boolean field",
		})

	case core.DataTypeArray:
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "not_an_array",
			ExpectedType: "array",
			ActualType:   "string",
			Description:  "string provided for array field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        map[string]any{"key": "value"},
			ExpectedType: "array",
			ActualType:   "object",
			Description:  "object provided for array field",
		})

	case core.DataTypeObject:
		payloads = append(payloads, TypeConfusionPayload{
			Value:        "not_an_object",
			ExpectedType: "object",
			ActualType:   "string",
			Description:  "string provided for object field",
		})
		payloads = append(payloads, TypeConfusionPayload{
			Value:        []string{"array", "value"},
			ExpectedType: "object",
			ActualType:   "array",
			Description:  "array provided for object field",
		})
	}

	return payloads
}

type TypeConfusionPayload struct {
	Value        any    `json:"value"`
	ExpectedType string `json:"expected_type"`
	ActualType   string `json:"actual_type"`
	Description  string `json:"description"`
}

func GenerateFormatBypassPayloads(format string) []FormatBypassPayload {
	var payloads []FormatBypassPayload

	switch format {
	case "email":
		payloads = append(payloads,
			FormatBypassPayload{Value: "test", Description: "plain string without @"},
			FormatBypassPayload{Value: "test@", Description: "email without domain"},
			FormatBypassPayload{Value: "@test.com", Description: "email without local part"},
			FormatBypassPayload{Value: "test@@test.com", Description: "double @ symbol"},
			FormatBypassPayload{Value: "test@test", Description: "email without TLD"},
			FormatBypassPayload{Value: "test\x00@test.com", Description: "null byte injection"},
			FormatBypassPayload{Value: "test\n@test.com", Description: "newline injection"},
		)

	case "uri", "url":
		payloads = append(payloads,
			FormatBypassPayload{Value: "not-a-url", Description: "plain string without protocol"},
			FormatBypassPayload{Value: "://missing-protocol", Description: "missing protocol"},
			FormatBypassPayload{Value: "javascript:alert(1)", Description: "javascript protocol"},
			FormatBypassPayload{Value: "file:///etc/passwd", Description: "file protocol"},
			FormatBypassPayload{Value: "//protocol-relative", Description: "protocol-relative URL"},
		)

	case "uuid":
		payloads = append(payloads,
			FormatBypassPayload{Value: "not-a-uuid", Description: "plain string"},
			FormatBypassPayload{Value: "123456", Description: "short numeric string"},
			FormatBypassPayload{Value: "00000000-0000-0000-0000-00000000000", Description: "UUID with missing character"},
			FormatBypassPayload{Value: "00000000-0000-0000-0000-0000000000000", Description: "UUID with extra character"},
			FormatBypassPayload{Value: "ZZZZZZZZ-ZZZZ-ZZZZ-ZZZZ-ZZZZZZZZZZZZ", Description: "UUID with invalid hex characters"},
		)

	case "date":
		payloads = append(payloads,
			FormatBypassPayload{Value: "not-a-date", Description: "plain string"},
			FormatBypassPayload{Value: "0000-00-00", Description: "zero date"},
			FormatBypassPayload{Value: "9999-99-99", Description: "invalid month/day"},
			FormatBypassPayload{Value: "2024-13-01", Description: "invalid month 13"},
			FormatBypassPayload{Value: "2024-02-30", Description: "invalid day for February"},
		)

	case "date-time":
		payloads = append(payloads,
			FormatBypassPayload{Value: "not-a-datetime", Description: "plain string"},
			FormatBypassPayload{Value: "2024-01-01", Description: "date without time"},
			FormatBypassPayload{Value: "2024-01-01T25:00:00Z", Description: "invalid hour 25"},
			FormatBypassPayload{Value: "2024-01-01T00:60:00Z", Description: "invalid minute 60"},
		)

	case "ipv4":
		payloads = append(payloads,
			FormatBypassPayload{Value: "not-an-ip", Description: "plain string"},
			FormatBypassPayload{Value: "999.999.999.999", Description: "octets > 255"},
			FormatBypassPayload{Value: "1.2.3", Description: "only 3 octets"},
			FormatBypassPayload{Value: "1.2.3.4.5", Description: "5 octets"},
			FormatBypassPayload{Value: "1.2.3.4/24", Description: "IP with CIDR"},
		)

	case "ipv6":
		payloads = append(payloads,
			FormatBypassPayload{Value: "not-an-ip", Description: "plain string"},
			FormatBypassPayload{Value: ":::", Description: "triple colon"},
			FormatBypassPayload{Value: "GGGG::", Description: "invalid hex"},
		)

	case "hostname":
		payloads = append(payloads,
			FormatBypassPayload{Value: "-invalid", Description: "starts with hyphen"},
			FormatBypassPayload{Value: "invalid-", Description: "ends with hyphen"},
			FormatBypassPayload{Value: strings.Repeat("a", 64) + ".com", Description: "label > 63 chars"},
		)
	}

	return payloads
}

type FormatBypassPayload struct {
	Value       string `json:"value"`
	Description string `json:"description"`
}

func GenerateNullValuePayloads() []any {
	return []any{
		nil,
		"null",
		"NULL",
		"Null",
		"",
		0,
	}
}

func GenerateSpecialCharacterPayloads() []string {
	return []string{
		"\x00",
		"\n",
		"\r",
		"\r\n",
		"\t",
		"\\",
		"/",
		"\"",
		"'",
		"`",
		"<",
		">",
		"&",
		"%00",
		"%0a",
		"%0d",
		"%09",
	}
}

func GenerateOverflowPayloads() []string {
	return []string{
		strings.Repeat("a", 1000),
		strings.Repeat("a", 10000),
		strings.Repeat("a", 100000),
		fmt.Sprintf("%d", 1<<31),
		fmt.Sprintf("%d", 1<<63-1),
		fmt.Sprintf("%d", -(1 << 63)),
		"9999999999999999999999999999999999999999",
		"-9999999999999999999999999999999999999999",
	}
}
