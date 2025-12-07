package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultValueStrategyScalars(t *testing.T) {
	schema := &GraphQLSchema{
		Enums:      make(map[string]EnumDef),
		InputTypes: make(map[string]InputTypeDef),
	}
	strategy := NewDefaultValueStrategy(schema)

	tests := []struct {
		typeName string
		notNil   bool
	}{
		{"String", true},
		{"Int", true},
		{"Float", true},
		{"Boolean", true},
		{"ID", true},
		{"Date", true},
		{"DateTime", true},
		{"Email", true},
		{"URL", true},
		{"UUID", true},
		{"JSON", true},
		{"UnknownScalar", true},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			values := strategy.GenerateScalar(tt.typeName)
			require.Len(t, values, 1)
			if tt.notNil {
				assert.NotNil(t, values[0].Value)
			}
			assert.NotEmpty(t, values[0].Description)
		})
	}
}

func TestDefaultValueStrategyEnum(t *testing.T) {
	schema := &GraphQLSchema{
		Enums: make(map[string]EnumDef),
	}
	strategy := NewDefaultValueStrategy(schema)

	enumDef := EnumDef{
		Name: "Status",
		Values: []EnumValue{
			{Name: "ACTIVE", IsDeprecated: false},
			{Name: "INACTIVE", IsDeprecated: false},
			{Name: "PENDING", IsDeprecated: true},
		},
	}

	values := strategy.GenerateEnum(enumDef)
	require.Len(t, values, 1)
	assert.Equal(t, "ACTIVE", values[0].Value) // First non-deprecated value
}

func TestDefaultValueStrategyEnumAllDeprecated(t *testing.T) {
	schema := &GraphQLSchema{
		Enums: make(map[string]EnumDef),
	}
	strategy := NewDefaultValueStrategy(schema)

	enumDef := EnumDef{
		Name: "OldStatus",
		Values: []EnumValue{
			{Name: "OLD1", IsDeprecated: true},
			{Name: "OLD2", IsDeprecated: true},
		},
	}

	values := strategy.GenerateEnum(enumDef)
	require.Len(t, values, 1)
	assert.Equal(t, "OLD1", values[0].Value) // Falls back to first
}

func TestDefaultValueStrategyInputObject(t *testing.T) {
	schema := &GraphQLSchema{
		Enums: map[string]EnumDef{
			"Role": {
				Name: "Role",
				Values: []EnumValue{
					{Name: "ADMIN"},
					{Name: "USER"},
				},
			},
		},
		InputTypes: map[string]InputTypeDef{
			"UserInput": {
				Name: "UserInput",
				Fields: []InputField{
					{
						Name: "name",
						Type: TypeRef{Kind: TypeKindScalar, Name: "String", Required: true},
					},
					{
						Name: "email",
						Type: TypeRef{Kind: TypeKindScalar, Name: "String", Required: true},
					},
					{
						Name: "role",
						Type: TypeRef{Kind: TypeKindEnum, Name: "Role", Required: false},
					},
				},
			},
		},
	}
	strategy := NewDefaultValueStrategy(schema)

	inputDef := schema.InputTypes["UserInput"]
	values := strategy.GenerateInputObject(inputDef, schema, 0)

	require.Len(t, values, 1)
	obj, ok := values[0].Value.(map[string]interface{})
	require.True(t, ok)

	assert.Contains(t, obj, "name")
	assert.Contains(t, obj, "email")
	assert.Contains(t, obj, "role")
}

func TestInterestingValuesStrategyScalars(t *testing.T) {
	schema := &GraphQLSchema{
		Enums:      make(map[string]EnumDef),
		InputTypes: make(map[string]InputTypeDef),
	}
	strategy := NewInterestingValuesStrategy(schema)

	tests := []struct {
		typeName string
		minCount int
	}{
		{"String", 5},  // Should have multiple string variations
		{"Int", 5},     // Should have boundary values
		{"Float", 5},   // Should have boundary values
		{"Boolean", 2}, // true and false
		{"ID", 5},      // Should have injection payloads
		{"Email", 3},   // Should have valid/invalid variations
		{"URL", 5},     // Should have SSRF payloads
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			values := strategy.GenerateScalar(tt.typeName)
			assert.GreaterOrEqual(t, len(values), tt.minCount,
				"Expected at least %d values for %s, got %d", tt.minCount, tt.typeName, len(values))

			// All values should have descriptions
			for _, v := range values {
				assert.NotEmpty(t, v.Description)
			}
		})
	}
}

func TestInterestingValuesStrategyStringPayloads(t *testing.T) {
	schema := &GraphQLSchema{}
	strategy := NewInterestingValuesStrategy(schema)

	values := strategy.GenerateScalar("String")

	// Check for specific security-focused payloads
	hasXSS := false
	hasSQLi := false
	hasSSTI := false
	hasPathTraversal := false

	for _, v := range values {
		str, ok := v.Value.(string)
		if !ok {
			continue
		}
		if str == "<script>alert(1)</script>" {
			hasXSS = true
		}
		if str == "'; DROP TABLE users;--" {
			hasSQLi = true
		}
		if str == "{{7*7}}" {
			hasSSTI = true
		}
		if str == "../../../etc/passwd" {
			hasPathTraversal = true
		}
	}

	assert.True(t, hasXSS, "Should include XSS payload")
	assert.True(t, hasSQLi, "Should include SQLi payload")
	assert.True(t, hasSSTI, "Should include SSTI payload")
	assert.True(t, hasPathTraversal, "Should include path traversal payload")
}

func TestInterestingValuesStrategyIntBoundaries(t *testing.T) {
	schema := &GraphQLSchema{}
	strategy := NewInterestingValuesStrategy(schema)

	values := strategy.GenerateScalar("Int")

	hasZero := false
	hasNegative := false
	hasMaxInt := false

	for _, v := range values {
		switch v.Value {
		case 0:
			hasZero = true
		case -1:
			hasNegative = true
		case int(2147483647): // MaxInt32
			hasMaxInt = true
		}
	}

	assert.True(t, hasZero, "Should include zero")
	assert.True(t, hasNegative, "Should include negative value")
	assert.True(t, hasMaxInt, "Should include max int boundary")
}

func TestInterestingValuesStrategyEnum(t *testing.T) {
	schema := &GraphQLSchema{}
	strategy := NewInterestingValuesStrategy(schema)

	enumDef := EnumDef{
		Name: "Status",
		Values: []EnumValue{
			{Name: "ACTIVE"},
			{Name: "INACTIVE"},
		},
	}

	values := strategy.GenerateEnum(enumDef)

	// Should have all valid values + invalid value + empty
	assert.GreaterOrEqual(t, len(values), 4)

	// Check for invalid enum value
	hasInvalid := false
	for _, v := range values {
		if v.Value == "INVALID_ENUM_VALUE" {
			hasInvalid = true
			break
		}
	}
	assert.True(t, hasInvalid, "Should include invalid enum value")
}

func TestInterestingValuesStrategyInputObject(t *testing.T) {
	schema := &GraphQLSchema{
		InputTypes: map[string]InputTypeDef{
			"TestInput": {
				Name: "TestInput",
				Fields: []InputField{
					{Name: "required", Type: TypeRef{Kind: TypeKindScalar, Name: "String", Required: true}},
					{Name: "optional", Type: TypeRef{Kind: TypeKindScalar, Name: "String", Required: false}},
				},
			},
		},
		Enums: make(map[string]EnumDef),
	}
	strategy := NewInterestingValuesStrategy(schema)

	inputDef := schema.InputTypes["TestInput"]
	values := strategy.GenerateInputObject(inputDef, schema, 0)

	// Should have baseline, complete, and empty variations
	assert.GreaterOrEqual(t, len(values), 3)

	// Check for empty object
	hasEmpty := false
	for _, v := range values {
		obj, ok := v.Value.(map[string]interface{})
		if ok && len(obj) == 0 {
			hasEmpty = true
			break
		}
	}
	assert.True(t, hasEmpty, "Should include empty object variation")
}

func TestUnwrapType(t *testing.T) {
	tests := []struct {
		name     string
		ref      TypeRef
		expected string
	}{
		{
			name:     "simple scalar",
			ref:      TypeRef{Kind: TypeKindScalar, Name: "String"},
			expected: "String",
		},
		{
			name: "non-null wrapper",
			ref: TypeRef{
				Kind:   TypeKindNonNull,
				OfType: &TypeRef{Kind: TypeKindScalar, Name: "Int"},
			},
			expected: "Int",
		},
		{
			name: "list wrapper",
			ref: TypeRef{
				Kind:   TypeKindList,
				OfType: &TypeRef{Kind: TypeKindScalar, Name: "Float"},
			},
			expected: "Float",
		},
		{
			name: "deeply nested",
			ref: TypeRef{
				Kind: TypeKindNonNull,
				OfType: &TypeRef{
					Kind: TypeKindList,
					OfType: &TypeRef{
						Kind:   TypeKindNonNull,
						OfType: &TypeRef{Kind: TypeKindScalar, Name: "Boolean"},
					},
				},
			},
			expected: "Boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unwrapType(tt.ref)
			assert.Equal(t, tt.expected, result.Name)
		})
	}
}

func TestGetBaseTypeNameFromRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      TypeRef
		expected string
	}{
		{
			name:     "direct name",
			ref:      TypeRef{Name: "User"},
			expected: "User",
		},
		{
			name: "wrapped",
			ref: TypeRef{
				Kind:   TypeKindNonNull,
				OfType: &TypeRef{Name: "User"},
			},
			expected: "User",
		},
		{
			name: "deeply nested",
			ref: TypeRef{
				Kind: TypeKindList,
				OfType: &TypeRef{
					Kind:   TypeKindNonNull,
					OfType: &TypeRef{Name: "Post"},
				},
			},
			expected: "Post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBaseTypeNameFromRef(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaxDepthInInputObject(t *testing.T) {
	schema := &GraphQLSchema{
		InputTypes: map[string]InputTypeDef{
			"NestedInput": {
				Name: "NestedInput",
				Fields: []InputField{
					{
						Name: "nested",
						Type: TypeRef{Kind: TypeKindInputObject, Name: "NestedInput"},
					},
				},
			},
		},
		Enums: make(map[string]EnumDef),
	}
	strategy := NewDefaultValueStrategy(schema)
	strategy.maxDepth = 2

	inputDef := schema.InputTypes["NestedInput"]
	values := strategy.GenerateInputObject(inputDef, schema, 0)

	require.Len(t, values, 1)
	// Should not cause infinite recursion
	assert.NotNil(t, values[0].Value)
}

func TestURLPayloads(t *testing.T) {
	schema := &GraphQLSchema{}
	strategy := NewInterestingValuesStrategy(schema)

	values := strategy.GenerateScalar("URL")

	hasSSRF := false
	hasFileProtocol := false
	hasJavascript := false

	for _, v := range values {
		str, ok := v.Value.(string)
		if !ok {
			continue
		}
		if str == "http://169.254.169.254" {
			hasSSRF = true
		}
		if str == "file:///etc/passwd" {
			hasFileProtocol = true
		}
		if str == "javascript:alert(1)" {
			hasJavascript = true
		}
	}

	assert.True(t, hasSSRF, "Should include cloud metadata SSRF payload")
	assert.True(t, hasFileProtocol, "Should include file protocol payload")
	assert.True(t, hasJavascript, "Should include javascript protocol payload")
}

func TestRandomString(t *testing.T) {
	// Test that RandomString generates correct length
	for _, length := range []int{5, 10, 20, 100} {
		result := RandomString(length)
		assert.Len(t, result, length)
	}

	// Test that results are different (not deterministic)
	s1 := RandomString(20)
	s2 := RandomString(20)
	// There's a tiny chance these could be equal, but highly unlikely
	assert.NotEqual(t, s1, s2, "Random strings should be different")
}
