package generation

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/pkg/scan/options"
)

type SmartAPIPayloads struct{}

func NewSmartAPIPayloads() *SmartAPIPayloads {
	return &SmartAPIPayloads{}
}

func (s *SmartAPIPayloads) GenerateForParameter(param options.APIParameter) []string {
	var payloads []string

	payloads = append(payloads, s.generateTypePayloads(param)...)
	payloads = append(payloads, s.generateFormatPayloads(param)...)
	payloads = append(payloads, s.generateConstraintBypass(param)...)

	if len(param.Enum) > 0 {
		payloads = append(payloads, s.generateEnumBypass(param)...)
	}

	if param.Pattern != "" {
		payloads = append(payloads, s.generatePatternBypass(param)...)
	}

	return payloads
}

func (s *SmartAPIPayloads) generateTypePayloads(param options.APIParameter) []string {
	switch param.Type {
	case "integer", "number":
		return []string{
			"0",
			"-1",
			"999999999999999999",
			"-999999999999999999",
			"1.5",
			"1e308",
			"NaN",
			"Infinity",
			"-Infinity",
			"0x1",
			"0b1",
			"1' OR '1'='1",
			"1; DROP TABLE users",
		}
	case "boolean":
		return []string{
			"true",
			"false",
			"1",
			"0",
			"yes",
			"no",
			"null",
			"undefined",
			"True",
			"TRUE",
			"\"true\"",
		}
	case "array":
		return []string{
			"[]",
			"[null]",
			"[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]",
			"[[[[[[]]]]]]",
			"[\"__proto__\"]",
		}
	case "object":
		return []string{
			"{}",
			"{\"__proto__\":{\"admin\":true}}",
			"{\"constructor\":{\"prototype\":{\"admin\":true}}}",
			"null",
		}
	default:
		return []string{}
	}
}

func (s *SmartAPIPayloads) generateFormatPayloads(param options.APIParameter) []string {
	switch param.Format {
	case "email":
		return []string{
			"test@test.com",
			"\"test\"@test.com",
			"test+tag@test.com",
			"test@test.com\n",
			"test@test.com%00",
			"<script>@test.com",
			"test@test.com'--",
			"test@[127.0.0.1]",
			".test@test.com",
			"test..test@test.com",
		}
	case "uuid":
		return []string{
			"00000000-0000-0000-0000-000000000000",
			"ffffffff-ffff-ffff-ffff-ffffffffffff",
			"not-a-uuid",
			"' OR '1'='1",
			"../../../etc/passwd",
		}
	case "date", "date-time":
		return []string{
			"2024-01-01",
			"0000-00-00",
			"9999-99-99",
			"2024-01-01T00:00:00Z",
			"2024-01-01'; DROP TABLE--",
			"../../etc/passwd",
		}
	case "uri", "url":
		return []string{
			"https://example.com",
			"javascript:alert(1)",
			"data:text/html,<script>alert(1)</script>",
			"file:///etc/passwd",
			"//attacker.com",
			"https://attacker.com\\@legitimate.com",
		}
	case "hostname":
		return []string{
			"localhost",
			"127.0.0.1",
			"0.0.0.0",
			"internal.service",
			"169.254.169.254",
			"[::1]",
		}
	case "ipv4":
		return []string{
			"127.0.0.1",
			"0.0.0.0",
			"169.254.169.254",
			"192.168.1.1",
			"10.0.0.1",
			"127.0.0.1#attacker.com",
		}
	case "int32", "int64":
		return []string{
			"2147483647",
			"-2147483648",
			"2147483648",
			"-2147483649",
			"9223372036854775807",
			"-9223372036854775808",
		}
	default:
		return []string{}
	}
}

func (s *SmartAPIPayloads) generateConstraintBypass(param options.APIParameter) []string {
	var payloads []string

	if param.Minimum != nil {
		payloads = append(payloads, fmt.Sprintf("%v", *param.Minimum-1))
		payloads = append(payloads, fmt.Sprintf("%v", *param.Minimum-0.1))
	}
	if param.Maximum != nil {
		payloads = append(payloads, fmt.Sprintf("%v", *param.Maximum+1))
		payloads = append(payloads, fmt.Sprintf("%v", *param.Maximum+0.1))
	}

	if param.MinLength != nil && *param.MinLength > 0 {
		payloads = append(payloads, strings.Repeat("a", *param.MinLength-1))
		payloads = append(payloads, "")
	}
	if param.MaxLength != nil {
		payloads = append(payloads, strings.Repeat("a", *param.MaxLength+1))
		payloads = append(payloads, strings.Repeat("a", *param.MaxLength+50))
	}

	return payloads
}

func (s *SmartAPIPayloads) generateEnumBypass(param options.APIParameter) []string {
	var payloads []string

	for _, v := range param.Enum {
		payloads = append(payloads, strings.ToUpper(v))
		payloads = append(payloads, strings.ToLower(v))
		payloads = append(payloads, v+" ")
		payloads = append(payloads, " "+v)
		payloads = append(payloads, v+"\x00")
	}

	payloads = append(payloads, "INVALID_ENUM_VALUE")
	payloads = append(payloads, "null")
	payloads = append(payloads, "")

	return payloads
}

func (s *SmartAPIPayloads) generatePatternBypass(param options.APIParameter) []string {
	return []string{
		"",
		"\x00",
		"a\nb",
		"a\rb",
		"%00",
		"\u0000",
	}
}

func (s *SmartAPIPayloads) GetPayloadsForAPIContext(apiContext *options.APIContext, paramName string) []string {
	if apiContext == nil {
		return nil
	}

	for _, param := range apiContext.Parameters {
		if param.Name == paramName {
			return s.GenerateForParameter(param)
		}
	}

	return nil
}

func GetSmartPayloadsForParameter(param options.APIParameter) []string {
	generator := NewSmartAPIPayloads()
	return generator.GenerateForParameter(param)
}

func GetSmartPayloadsForAPIContext(apiContext *options.APIContext, paramName string) []string {
	if apiContext == nil {
		return nil
	}

	generator := NewSmartAPIPayloads()
	return generator.GetPayloadsForAPIContext(apiContext, paramName)
}
