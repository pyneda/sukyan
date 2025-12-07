package graphql

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"
)

// ValueStrategy generates values for GraphQL types
type ValueStrategy interface {
	// GenerateScalar generates values for a scalar type
	GenerateScalar(typeName string) []GeneratedValue
	// GenerateEnum generates values for an enum type
	GenerateEnum(enumDef EnumDef) []GeneratedValue
	// GenerateInputObject generates values for an input object type
	GenerateInputObject(inputDef InputTypeDef, schema *GraphQLSchema, depth int) []GeneratedValue
}

// GeneratedValue represents a generated value with description
type GeneratedValue struct {
	Value       interface{} `json:"value"`
	Description string      `json:"description"`
}

// DefaultValueStrategy generates single sensible default values
type DefaultValueStrategy struct {
	schema   *GraphQLSchema
	maxDepth int
}

// NewDefaultValueStrategy creates a new default value strategy
func NewDefaultValueStrategy(schema *GraphQLSchema) *DefaultValueStrategy {
	return &DefaultValueStrategy{
		schema:   schema,
		maxDepth: 3,
	}
}

// GenerateScalar generates a single default value for a scalar type
func (s *DefaultValueStrategy) GenerateScalar(typeName string) []GeneratedValue {
	var value interface{}
	desc := "Default value"

	switch strings.ToLower(typeName) {
	case "string":
		value = "test_string"
	case "int":
		value = 1
	case "float":
		value = 1.0
	case "boolean", "bool":
		value = true
	case "id":
		value = "1"
	// Common custom scalars
	case "date":
		value = time.Now().Format("2006-01-02")
		desc = "Current date"
	case "datetime", "timestamp":
		value = time.Now().Format(time.RFC3339)
		desc = "Current datetime"
	case "email":
		value = "test@example.com"
		desc = "Test email"
	case "url", "uri":
		value = "https://example.com"
		desc = "Test URL"
	case "json", "jsonobject":
		value = map[string]interface{}{"key": "value"}
		desc = "Test JSON object"
	case "uuid":
		value = "550e8400-e29b-41d4-a716-446655440000"
		desc = "Test UUID"
	case "phone", "phonenumber":
		value = "+1234567890"
		desc = "Test phone"
	case "bigint", "long":
		value = 9999999999
		desc = "Large integer"
	case "decimal", "money", "currency":
		value = 99.99
		desc = "Decimal value"
	case "positiveint":
		value = 1
		desc = "Positive integer"
	case "nonnegativeint":
		value = 0
		desc = "Non-negative integer"
	case "negativeint":
		value = -1
		desc = "Negative integer"
	case "upload", "file":
		value = nil
		desc = "File upload (null placeholder)"
	default:
		// Unknown scalar, use string
		value = "test_value"
		desc = fmt.Sprintf("Unknown scalar %s - using string", typeName)
	}

	return []GeneratedValue{{Value: value, Description: desc}}
}

// GenerateEnum generates a single enum value (the first one)
func (s *DefaultValueStrategy) GenerateEnum(enumDef EnumDef) []GeneratedValue {
	if len(enumDef.Values) == 0 {
		return []GeneratedValue{{Value: nil, Description: "Empty enum"}}
	}

	// Use first non-deprecated value
	for _, v := range enumDef.Values {
		if !v.IsDeprecated {
			return []GeneratedValue{{Value: v.Name, Description: fmt.Sprintf("Enum value %s", v.Name)}}
		}
	}

	// If all are deprecated, use the first one
	return []GeneratedValue{{Value: enumDef.Values[0].Name, Description: "First enum value (deprecated)"}}
}

// GenerateInputObject generates a single input object value
func (s *DefaultValueStrategy) GenerateInputObject(inputDef InputTypeDef, schema *GraphQLSchema, depth int) []GeneratedValue {
	if depth > s.maxDepth {
		return []GeneratedValue{{Value: nil, Description: "Max depth exceeded"}}
	}

	obj := make(map[string]interface{})

	for _, field := range inputDef.Fields {
		fieldValue := s.generateValueForType(field.Type, schema, depth+1)
		if fieldValue != nil || field.Type.Required {
			obj[field.Name] = fieldValue
		}
	}

	return []GeneratedValue{{
		Value:       obj,
		Description: fmt.Sprintf("Input object %s", inputDef.Name),
	}}
}

// generateValueForType generates a value for any type reference
func (s *DefaultValueStrategy) generateValueForType(typeRef TypeRef, schema *GraphQLSchema, depth int) interface{} {
	// Handle list types
	if typeRef.IsList {
		innerType := unwrapType(typeRef)
		innerValue := s.generateValueForType(innerType, schema, depth)
		if innerValue == nil {
			return []interface{}{}
		}
		return []interface{}{innerValue}
	}

	// Get base type name
	baseName := getBaseTypeNameFromRef(typeRef)

	// Check if it's an enum
	if enumDef, ok := schema.Enums[baseName]; ok {
		values := s.GenerateEnum(enumDef)
		if len(values) > 0 {
			return values[0].Value
		}
		return nil
	}

	// Check if it's an input object
	if inputDef, ok := schema.InputTypes[baseName]; ok {
		values := s.GenerateInputObject(inputDef, schema, depth)
		if len(values) > 0 {
			return values[0].Value
		}
		return nil
	}

	// Must be a scalar
	values := s.GenerateScalar(baseName)
	if len(values) > 0 {
		return values[0].Value
	}
	return nil
}

// InterestingValuesStrategy generates multiple interesting values for fuzzing
type InterestingValuesStrategy struct {
	schema   *GraphQLSchema
	maxDepth int
}

// NewInterestingValuesStrategy creates a new interesting values strategy
func NewInterestingValuesStrategy(schema *GraphQLSchema) *InterestingValuesStrategy {
	return &InterestingValuesStrategy{
		schema:   schema,
		maxDepth: 2,
	}
}

// GenerateScalar generates interesting values for a scalar type
func (s *InterestingValuesStrategy) GenerateScalar(typeName string) []GeneratedValue {
	var values []GeneratedValue

	switch strings.ToLower(typeName) {
	case "string":
		values = []GeneratedValue{
			{Value: "test_string", Description: "Default string"},
			{Value: "", Description: "Empty string"},
			{Value: strings.Repeat("A", 1000), Description: "Long string (1000 chars)"},
			{Value: "null", Description: "String 'null'"},
			{Value: "undefined", Description: "String 'undefined'"},
			{Value: "<script>alert(1)</script>", Description: "XSS payload"},
			{Value: "'; DROP TABLE users;--", Description: "SQL injection"},
			{Value: "{{7*7}}", Description: "SSTI payload"},
			{Value: "${7*7}", Description: "Expression injection"},
			{Value: "../../../etc/passwd", Description: "Path traversal"},
			{Value: "admin@test.com' OR '1'='1", Description: "Email injection"},
			{Value: "%00", Description: "Null byte"},
			{Value: "\n\r", Description: "CRLF injection"},
			{Value: "true", Description: "String 'true'"},
			{Value: "false", Description: "String 'false'"},
			{Value: "-1", Description: "String '-1'"},
			{Value: "0", Description: "String '0'"},
		}

	case "int":
		values = []GeneratedValue{
			{Value: 1, Description: "Default integer"},
			{Value: 0, Description: "Zero"},
			{Value: -1, Description: "Negative one"},
			{Value: math.MaxInt32, Description: "Max int32"},
			{Value: math.MinInt32, Description: "Min int32"},
			{Value: math.MaxInt64, Description: "Max int64"},
			{Value: math.MinInt64, Description: "Min int64"},
			{Value: 999999999, Description: "Large number"},
		}

	case "float":
		values = []GeneratedValue{
			{Value: 1.0, Description: "Default float"},
			{Value: 0.0, Description: "Zero"},
			{Value: -1.0, Description: "Negative one"},
			{Value: math.MaxFloat64, Description: "Max float64"},
			{Value: math.SmallestNonzeroFloat64, Description: "Smallest positive float64"},
			{Value: -math.MaxFloat64, Description: "Min float64"},
			{Value: 3.14159265358979, Description: "Pi"},
			{Value: 0.1 + 0.2, Description: "Floating point precision test"},
		}

	case "boolean", "bool":
		values = []GeneratedValue{
			{Value: true, Description: "True"},
			{Value: false, Description: "False"},
		}

	case "id":
		values = []GeneratedValue{
			{Value: "1", Description: "Default ID"},
			{Value: "0", Description: "Zero ID"},
			{Value: "-1", Description: "Negative ID"},
			{Value: "", Description: "Empty ID"},
			{Value: "nonexistent_id_12345", Description: "Non-existent ID"},
			{Value: strings.Repeat("9", 50), Description: "Long numeric ID"},
			{Value: "admin", Description: "Admin ID string"},
			{Value: "null", Description: "Null ID string"},
			{Value: "1 OR 1=1", Description: "SQL injection ID"},
		}

	case "email":
		values = []GeneratedValue{
			{Value: "test@example.com", Description: "Valid email"},
			{Value: "", Description: "Empty email"},
			{Value: "invalid-email", Description: "Invalid format"},
			{Value: "test@test@test.com", Description: "Double @"},
			{Value: strings.Repeat("a", 100) + "@example.com", Description: "Long local part"},
			{Value: "admin@localhost", Description: "Localhost email"},
			{Value: "test+injection@example.com", Description: "Plus sign email"},
		}

	case "url", "uri":
		values = []GeneratedValue{
			{Value: "https://example.com", Description: "Valid HTTPS URL"},
			{Value: "http://example.com", Description: "HTTP URL"},
			{Value: "http://localhost", Description: "Localhost"},
			{Value: "http://127.0.0.1", Description: "Loopback IP"},
			{Value: "http://169.254.169.254", Description: "Cloud metadata IP"},
			{Value: "file:///etc/passwd", Description: "File protocol"},
			{Value: "javascript:alert(1)", Description: "JavaScript protocol"},
			{Value: "//example.com", Description: "Protocol-relative URL"},
		}

	case "date":
		values = []GeneratedValue{
			{Value: time.Now().Format("2006-01-02"), Description: "Current date"},
			{Value: "1970-01-01", Description: "Unix epoch"},
			{Value: "9999-12-31", Description: "Far future"},
			{Value: "0000-01-01", Description: "Year zero"},
			{Value: "invalid-date", Description: "Invalid format"},
		}

	case "datetime", "timestamp":
		values = []GeneratedValue{
			{Value: time.Now().Format(time.RFC3339), Description: "Current datetime"},
			{Value: "1970-01-01T00:00:00Z", Description: "Unix epoch"},
			{Value: "9999-12-31T23:59:59Z", Description: "Far future"},
			{Value: "invalid-datetime", Description: "Invalid format"},
		}

	case "uuid":
		values = []GeneratedValue{
			{Value: "550e8400-e29b-41d4-a716-446655440000", Description: "Valid UUID"},
			{Value: "00000000-0000-0000-0000-000000000000", Description: "Nil UUID"},
			{Value: "invalid-uuid", Description: "Invalid UUID"},
			{Value: "", Description: "Empty UUID"},
		}

	case "json", "jsonobject":
		values = []GeneratedValue{
			{Value: map[string]interface{}{"key": "value"}, Description: "Simple JSON object"},
			{Value: map[string]interface{}{}, Description: "Empty object"},
			{Value: []interface{}{}, Description: "Empty array"},
			{Value: nil, Description: "Null"},
		}

	default:
		// Unknown scalar - return string-based interesting values
		values = []GeneratedValue{
			{Value: "test_value", Description: fmt.Sprintf("Default for %s", typeName)},
			{Value: "", Description: "Empty value"},
			{Value: "null", Description: "String null"},
		}
	}

	return values
}

// GenerateEnum generates all enum values for fuzzing
func (s *InterestingValuesStrategy) GenerateEnum(enumDef EnumDef) []GeneratedValue {
	values := make([]GeneratedValue, 0, len(enumDef.Values)+2)

	// Add all valid enum values
	for _, v := range enumDef.Values {
		desc := fmt.Sprintf("Enum value: %s", v.Name)
		if v.IsDeprecated {
			desc += " (deprecated)"
		}
		values = append(values, GeneratedValue{Value: v.Name, Description: desc})
	}

	// Add invalid enum values for fuzzing
	values = append(values, GeneratedValue{Value: "INVALID_ENUM_VALUE", Description: "Invalid enum value"})
	values = append(values, GeneratedValue{Value: "", Description: "Empty enum value"})

	return values
}

// GenerateInputObject generates interesting variations of an input object
func (s *InterestingValuesStrategy) GenerateInputObject(inputDef InputTypeDef, schema *GraphQLSchema, depth int) []GeneratedValue {
	if depth > s.maxDepth {
		return []GeneratedValue{{Value: nil, Description: "Max depth exceeded"}}
	}

	var values []GeneratedValue

	// Generate baseline with all required fields
	baseline := s.generateBaselineInputObject(inputDef, schema, depth)
	values = append(values, GeneratedValue{
		Value:       baseline,
		Description: fmt.Sprintf("Baseline %s with required fields", inputDef.Name),
	})

	// Generate with all fields
	complete := s.generateCompleteInputObject(inputDef, schema, depth)
	values = append(values, GeneratedValue{
		Value:       complete,
		Description: fmt.Sprintf("Complete %s with all fields", inputDef.Name),
	})

	// Generate empty object
	values = append(values, GeneratedValue{
		Value:       map[string]interface{}{},
		Description: fmt.Sprintf("Empty %s", inputDef.Name),
	})

	return values
}

func (s *InterestingValuesStrategy) generateBaselineInputObject(inputDef InputTypeDef, schema *GraphQLSchema, depth int) map[string]interface{} {
	obj := make(map[string]interface{})
	defaultStrategy := NewDefaultValueStrategy(schema)

	for _, field := range inputDef.Fields {
		if field.Type.Required {
			obj[field.Name] = defaultStrategy.generateValueForType(field.Type, schema, depth+1)
		}
	}

	return obj
}

func (s *InterestingValuesStrategy) generateCompleteInputObject(inputDef InputTypeDef, schema *GraphQLSchema, depth int) map[string]interface{} {
	obj := make(map[string]interface{})
	defaultStrategy := NewDefaultValueStrategy(schema)

	for _, field := range inputDef.Fields {
		obj[field.Name] = defaultStrategy.generateValueForType(field.Type, schema, depth+1)
	}

	return obj
}

// Helper functions

// unwrapType removes NON_NULL and LIST wrappers to get the inner type
func unwrapType(ref TypeRef) TypeRef {
	if ref.OfType == nil {
		return ref
	}
	if ref.Kind == TypeKindNonNull || ref.Kind == TypeKindList {
		return unwrapType(*ref.OfType)
	}
	return ref
}

// getBaseTypeNameFromRef extracts the base type name from a TypeRef
func getBaseTypeNameFromRef(ref TypeRef) string {
	if ref.Name != "" {
		return ref.Name
	}
	if ref.OfType != nil {
		return getBaseTypeNameFromRef(*ref.OfType)
	}
	return ""
}

// RandomString generates a random string of specified length
func RandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
