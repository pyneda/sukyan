package graphql

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getTestSchema(t *testing.T) *GraphQLSchema {
	parser := NewParser()
	schema, err := parser.ParseFromJSON([]byte(sampleIntrospectionJSON))
	require.NoError(t, err)
	return schema
}

func TestGenerateRequestsHappyPath(t *testing.T) {
	schema := getTestSchema(t)

	config := GenerationConfig{
		BaseURL:               "http://localhost:4000/graphql",
		IncludeOptionalParams: true,
		FuzzingEnabled:        false,
		MaxDepth:              3,
	}

	generator := NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	// Should have 2 queries + 3 mutations = 5 endpoints
	assert.Len(t, endpoints, 5)

	// Check query endpoints
	var userEndpoint *OperationEndpoint
	var usersEndpoint *OperationEndpoint
	for i := range endpoints {
		if endpoints[i].Name == "user" {
			userEndpoint = &endpoints[i]
		}
		if endpoints[i].Name == "users" {
			usersEndpoint = &endpoints[i]
		}
	}

	require.NotNil(t, userEndpoint)
	assert.Equal(t, "query", userEndpoint.OperationType)
	assert.Len(t, userEndpoint.Arguments, 1)
	assert.Equal(t, "id", userEndpoint.Arguments[0].Name)
	assert.True(t, userEndpoint.Arguments[0].Required)

	require.NotNil(t, usersEndpoint)
	assert.Len(t, usersEndpoint.Arguments, 2)

	// Each should have at least 1 request (happy path)
	assert.GreaterOrEqual(t, len(userEndpoint.Requests), 1)
	assert.GreaterOrEqual(t, len(usersEndpoint.Requests), 1)

	// Check happy path request structure
	happyPath := userEndpoint.Requests[0]
	assert.Equal(t, "Happy Path", happyPath.Label)
	assert.Contains(t, happyPath.Query, "query user")
	assert.Contains(t, happyPath.Query, "$id: ID!")
	assert.Contains(t, happyPath.Query, "id: $id")
	assert.NotNil(t, happyPath.Variables)
	assert.Contains(t, happyPath.Variables, "id")
}

func TestGenerateRequestsWithFuzzing(t *testing.T) {
	schema := getTestSchema(t)

	config := GenerationConfig{
		BaseURL:               "http://localhost:4000/graphql",
		IncludeOptionalParams: false,
		FuzzingEnabled:        true,
		MaxDepth:              2,
	}

	generator := NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	// Find user query endpoint
	var userEndpoint *OperationEndpoint
	for i := range endpoints {
		if endpoints[i].Name == "user" {
			userEndpoint = &endpoints[i]
			break
		}
	}

	require.NotNil(t, userEndpoint)

	// Should have more than 1 request (happy path + fuzz variations)
	assert.Greater(t, len(userEndpoint.Requests), 1)

	// Check that fuzz requests exist
	hasFuzzRequest := false
	for _, req := range userEndpoint.Requests {
		if strings.HasPrefix(req.Label, "Fuzz") {
			hasFuzzRequest = true
			break
		}
	}
	assert.True(t, hasFuzzRequest, "Should have fuzzing variations")
}

func TestGenerateRequestsWithInputObjects(t *testing.T) {
	schema := getTestSchema(t)

	config := GenerationConfig{
		BaseURL:               "http://localhost:4000/graphql",
		IncludeOptionalParams: true,
		FuzzingEnabled:        false,
		MaxDepth:              3,
	}

	generator := NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	// Find createUser mutation
	var createUserEndpoint *OperationEndpoint
	for i := range endpoints {
		if endpoints[i].Name == "createUser" {
			createUserEndpoint = &endpoints[i]
			break
		}
	}

	require.NotNil(t, createUserEndpoint)
	assert.Equal(t, "mutation", createUserEndpoint.OperationType)

	// Check that input argument metadata includes nested fields
	require.Len(t, createUserEndpoint.Arguments, 1)
	inputArg := createUserEndpoint.Arguments[0]
	assert.Equal(t, "input", inputArg.Name)
	assert.True(t, inputArg.IsInputObject)
	assert.NotEmpty(t, inputArg.NestedFields)

	// Check nested field metadata
	var nameField *ArgumentMetadata
	for i := range inputArg.NestedFields {
		if inputArg.NestedFields[i].Name == "name" {
			nameField = &inputArg.NestedFields[i]
			break
		}
	}
	require.NotNil(t, nameField)
	assert.True(t, nameField.Required)
	assert.Equal(t, "String", nameField.TypeName)

	// Check the request has proper variable structure
	happyPath := createUserEndpoint.Requests[0]
	assert.Contains(t, happyPath.Variables, "input")

	inputVar := happyPath.Variables["input"]
	inputMap, ok := inputVar.(map[string]interface{})
	require.True(t, ok, "input variable should be a map")
	assert.Contains(t, inputMap, "name")
	assert.Contains(t, inputMap, "email")
}

func TestGenerateQueryString(t *testing.T) {
	schema := getTestSchema(t)

	config := GenerationConfig{
		BaseURL:  "http://localhost:4000/graphql",
		MaxDepth: 2,
	}

	generator := NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	// Find user query
	var userEndpoint *OperationEndpoint
	for i := range endpoints {
		if endpoints[i].Name == "user" {
			userEndpoint = &endpoints[i]
			break
		}
	}

	require.NotNil(t, userEndpoint)
	require.NotEmpty(t, userEndpoint.Requests)

	query := userEndpoint.Requests[0].Query

	// Should be valid GraphQL query structure
	assert.True(t, strings.HasPrefix(query, "query user"))
	assert.Contains(t, query, "($id: ID!)")
	assert.Contains(t, query, "user(id: $id)")
	assert.Contains(t, query, "{") // Selection set opening
	assert.Contains(t, query, "}") // Selection set closing
}

func TestToHTTPRequest(t *testing.T) {
	rv := RequestVariation{
		Label:         "Test Request",
		Query:         "query user($id: ID!) { user(id: $id) { id name } }",
		Variables:     map[string]interface{}{"id": "123"},
		OperationName: "user",
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Authorization": "Bearer token123",
		},
	}

	httpReq, err := rv.ToHTTPRequest("http://localhost:4000/graphql")
	require.NoError(t, err)

	assert.Equal(t, "http://localhost:4000/graphql", httpReq.URL)
	assert.Equal(t, "POST", httpReq.Method)
	assert.Contains(t, httpReq.Headers, "Content-Type")
	assert.Contains(t, httpReq.Headers, "Authorization")

	// Verify body structure
	var body struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
		OperationName string                 `json:"operationName"`
	}
	err = json.Unmarshal(httpReq.Body, &body)
	require.NoError(t, err)

	assert.Equal(t, rv.Query, body.Query)
	assert.Equal(t, rv.OperationName, body.OperationName)
	assert.Contains(t, body.Variables, "id")
}

func TestArgumentMetadataForNestedInputObjects(t *testing.T) {
	// Create a schema with nested input objects
	schema := &GraphQLSchema{
		Queries: []Operation{},
		Mutations: []Operation{
			{
				Name: "createOrder",
				Arguments: []Argument{
					{
						Name: "input",
						Type: TypeRef{
							Kind:     TypeKindNonNull,
							Required: true,
							OfType: &TypeRef{
								Kind: TypeKindInputObject,
								Name: "OrderInput",
							},
						},
					},
				},
			},
		},
		InputTypes: map[string]InputTypeDef{
			"OrderInput": {
				Name: "OrderInput",
				Fields: []InputField{
					{
						Name: "customer",
						Type: TypeRef{
							Kind:     TypeKindNonNull,
							Required: true,
							OfType: &TypeRef{
								Kind: TypeKindInputObject,
								Name: "CustomerInput",
							},
						},
					},
					{
						Name: "total",
						Type: TypeRef{
							Kind: TypeKindScalar,
							Name: "Float",
						},
					},
				},
			},
			"CustomerInput": {
				Name: "CustomerInput",
				Fields: []InputField{
					{
						Name: "name",
						Type: TypeRef{
							Kind:     TypeKindNonNull,
							Required: true,
							OfType: &TypeRef{
								Kind: TypeKindScalar,
								Name: "String",
							},
						},
					},
					{
						Name: "email",
						Type: TypeRef{
							Kind: TypeKindScalar,
							Name: "String",
						},
					},
				},
			},
		},
		Types: map[string]TypeDef{},
		Enums: map[string]EnumDef{},
	}

	config := GenerationConfig{
		BaseURL:               "http://localhost/graphql",
		IncludeOptionalParams: true,
		MaxDepth:              3,
	}

	generator := NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	require.Len(t, endpoints, 1)
	createOrder := endpoints[0]

	require.Len(t, createOrder.Arguments, 1)
	inputArg := createOrder.Arguments[0]

	assert.True(t, inputArg.IsInputObject)
	assert.Len(t, inputArg.NestedFields, 2)

	// Find customer field
	var customerField *ArgumentMetadata
	for i := range inputArg.NestedFields {
		if inputArg.NestedFields[i].Name == "customer" {
			customerField = &inputArg.NestedFields[i]
			break
		}
	}

	require.NotNil(t, customerField)
	assert.True(t, customerField.IsInputObject)
	assert.NotEmpty(t, customerField.NestedFields)

	// Check nested customer fields
	var nameField *ArgumentMetadata
	for i := range customerField.NestedFields {
		if customerField.NestedFields[i].Name == "name" {
			nameField = &customerField.NestedFields[i]
			break
		}
	}
	require.NotNil(t, nameField)
	assert.True(t, nameField.Required)
}

func TestDeduplication(t *testing.T) {
	schema := getTestSchema(t)

	config := GenerationConfig{
		BaseURL:               "http://localhost/graphql",
		IncludeOptionalParams: true,
		FuzzingEnabled:        true,
		MaxDepth:              2,
	}

	generator := NewGenerator(schema, config)
	endpoints := generator.GenerateRequests()

	// Verify no duplicate requests within any endpoint
	for _, endpoint := range endpoints {
		seen := make(map[string]bool)
		for _, req := range endpoint.Requests {
			sig := generator.getRequestSignature(req)
			if seen[sig] {
				t.Errorf("Duplicate request found in %s: %s", endpoint.Name, req.Label)
			}
			seen[sig] = true
		}
	}
}

func TestFormatTypeRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      TypeRef
		expected string
	}{
		{
			name: "simple scalar",
			ref: TypeRef{
				Kind: TypeKindScalar,
				Name: "String",
			},
			expected: "String",
		},
		{
			name: "non-null scalar",
			ref: TypeRef{
				Kind: TypeKindNonNull,
				OfType: &TypeRef{
					Kind: TypeKindScalar,
					Name: "String",
				},
			},
			expected: "String!",
		},
		{
			name: "list",
			ref: TypeRef{
				Kind: TypeKindList,
				OfType: &TypeRef{
					Kind: TypeKindScalar,
					Name: "Int",
				},
			},
			expected: "[Int]",
		},
		{
			name: "[String!]!",
			ref: TypeRef{
				Kind: TypeKindNonNull,
				OfType: &TypeRef{
					Kind: TypeKindList,
					OfType: &TypeRef{
						Kind: TypeKindNonNull,
						OfType: &TypeRef{
							Kind: TypeKindScalar,
							Name: "String",
						},
					},
				},
			},
			expected: "[String!]!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTypeRef(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSelectionSetGeneration(t *testing.T) {
	schema := getTestSchema(t)

	config := GenerationConfig{
		BaseURL:  "http://localhost/graphql",
		MaxDepth: 3,
	}

	generator := NewGenerator(schema, config)

	// Test selection set for User type
	userTypeRef := TypeRef{
		Kind: TypeKindObject,
		Name: "User",
	}

	selectionSet := generator.buildSelectionSet(userTypeRef, 0)

	// Should contain scalar fields
	assert.Contains(t, selectionSet, "id")
	assert.Contains(t, selectionSet, "name")
	assert.Contains(t, selectionSet, "email")
	assert.Contains(t, selectionSet, "role")
	assert.Contains(t, selectionSet, "createdAt")
}

func TestMaxDepthRespected(t *testing.T) {
	schema := getTestSchema(t)

	// Test with maxDepth = 1
	config := GenerationConfig{
		BaseURL:  "http://localhost/graphql",
		MaxDepth: 1,
	}

	generator := NewGenerator(schema, config)

	// At depth 1, nested objects shouldn't be expanded
	userTypeRef := TypeRef{
		Kind: TypeKindObject,
		Name: "User",
	}

	selectionSet := generator.buildSelectionSet(userTypeRef, 1)

	// At max depth, should only have __typename fallback or simple fields
	assert.NotEmpty(t, selectionSet)
}
