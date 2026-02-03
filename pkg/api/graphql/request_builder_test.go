package graphql

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/pkg/api/core"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeQueryOperation(name string, params []core.Parameter) core.Operation {
	return core.Operation{
		ID:      uuid.New(),
		APIType: core.APITypeGraphQL,
		Name:    name,
		Method:  "POST",
		Path:    "/graphql",
		BaseURL: "https://api.example.com/graphql",
		GraphQL: &core.GraphQLMetadata{
			OperationType: "query",
			ReturnType:    "User",
		},
		Parameters: params,
	}
}

func makeMutationOperation(name string, params []core.Parameter) core.Operation {
	return core.Operation{
		ID:      uuid.New(),
		APIType: core.APITypeGraphQL,
		Name:    name,
		Method:  "POST",
		Path:    "/graphql",
		BaseURL: "https://api.example.com/graphql",
		GraphQL: &core.GraphQLMetadata{
			OperationType: "mutation",
			ReturnType:    "User",
		},
		Parameters: params,
	}
}

func parseGQLBody(t *testing.T, body io.Reader) GraphQLRequest {
	t.Helper()
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var gqlReq GraphQLRequest
	require.NoError(t, json.Unmarshal(data, &gqlReq))
	return gqlReq
}

func TestBuild_BasicQuery(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("getUser", []core.Parameter{
		{Name: "id", Location: core.ParameterLocationBody, DataType: core.DataTypeString, Required: true},
	})

	req, err := builder.Build(context.Background(), op, map[string]any{"id": "user-123"})
	require.NoError(t, err)

	gqlReq := parseGQLBody(t, req.Body)

	assert.Contains(t, gqlReq.Query, "query getUser")
	assert.Contains(t, gqlReq.Query, "$id: String!")
	assert.Contains(t, gqlReq.Query, "id: $id")
	assert.Equal(t, "getUser", gqlReq.OperationName)
	assert.Equal(t, "user-123", gqlReq.Variables["id"])
}

func TestBuild_MutationOperation(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeMutationOperation("createUser", []core.Parameter{
		{Name: "name", Location: core.ParameterLocationBody, DataType: core.DataTypeString, Required: true},
		{Name: "email", Location: core.ParameterLocationBody, DataType: core.DataTypeString, Required: true},
	})

	params := map[string]any{"name": "Alice", "email": "alice@example.com"}
	req, err := builder.Build(context.Background(), op, params)
	require.NoError(t, err)

	gqlReq := parseGQLBody(t, req.Body)

	assert.Contains(t, gqlReq.Query, "mutation createUser")
	assert.Equal(t, "Alice", gqlReq.Variables["name"])
	assert.Equal(t, "alice@example.com", gqlReq.Variables["email"])
}

func TestBuild_VariableHandling(t *testing.T) {
	tests := []struct {
		name           string
		params         []core.Parameter
		paramValues    map[string]any
		expectedVars   map[string]any
	}{
		{
			name: "string variable",
			params: []core.Parameter{
				{Name: "query", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
			},
			paramValues:  map[string]any{"query": "search term"},
			expectedVars: map[string]any{"query": "search term"},
		},
		{
			name: "integer variable",
			params: []core.Parameter{
				{Name: "limit", Location: core.ParameterLocationBody, DataType: core.DataTypeInteger},
			},
			paramValues:  map[string]any{"limit": 10},
			expectedVars: map[string]any{"limit": 10},
		},
		{
			name: "boolean variable",
			params: []core.Parameter{
				{Name: "active", Location: core.ParameterLocationBody, DataType: core.DataTypeBoolean},
			},
			paramValues:  map[string]any{"active": true},
			expectedVars: map[string]any{"active": true},
		},
		{
			name: "uses effective value when param value not provided",
			params: []core.Parameter{
				{Name: "role", Location: core.ParameterLocationBody, DataType: core.DataTypeString, ExampleValue: "admin"},
			},
			paramValues:  map[string]any{},
			expectedVars: map[string]any{"role": "admin"},
		},
		{
			name: "multiple variables",
			params: []core.Parameter{
				{Name: "name", Location: core.ParameterLocationBody, DataType: core.DataTypeString, Required: true},
				{Name: "age", Location: core.ParameterLocationBody, DataType: core.DataTypeInteger, Required: true},
				{Name: "active", Location: core.ParameterLocationBody, DataType: core.DataTypeBoolean},
			},
			paramValues: map[string]any{"name": "Bob", "age": 25, "active": false},
			expectedVars: map[string]any{
				"name":   "Bob",
				"age":    25,
				"active": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewRequestBuilder()
			op := makeQueryOperation("testOp", tt.params)

			req, err := builder.Build(context.Background(), op, tt.paramValues)
			require.NoError(t, err)

			gqlReq := parseGQLBody(t, req.Body)

			for key, expected := range tt.expectedVars {
				actual, ok := gqlReq.Variables[key]
				assert.True(t, ok, "variable %q should be present", key)
				expectedJSON, _ := json.Marshal(expected)
				actualJSON, _ := json.Marshal(actual)
				assert.JSONEq(t, string(expectedJSON), string(actualJSON), "variable %q value mismatch", key)
			}
		})
	}
}

func TestBuild_NestedObjectParameter(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeMutationOperation("createUser", []core.Parameter{
		{
			Name:     "input",
			Location: core.ParameterLocationBody,
			DataType: core.DataTypeObject,
			Required: true,
			NestedParams: []core.Parameter{
				{Name: "name", DataType: core.DataTypeString, ExampleValue: "Alice"},
				{Name: "age", DataType: core.DataTypeInteger, ExampleValue: 30},
			},
		},
	})

	req, err := builder.Build(context.Background(), op, map[string]any{})
	require.NoError(t, err)

	gqlReq := parseGQLBody(t, req.Body)

	inputVar, ok := gqlReq.Variables["input"]
	require.True(t, ok, "variables should contain 'input'")

	inputMap, ok := inputVar.(map[string]any)
	require.True(t, ok, "input variable should be a map")
	assert.Equal(t, "Alice", inputMap["name"])

	age, _ := json.Marshal(inputMap["age"])
	assert.Equal(t, "30", string(age))
}

func TestBuildWithModifiedParam(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("getUser", []core.Parameter{
		{Name: "id", Location: core.ParameterLocationBody, DataType: core.DataTypeString, Required: true},
		{Name: "format", Location: core.ParameterLocationBody, DataType: core.DataTypeString},
	})

	original := map[string]any{"id": "user-1", "format": "json"}

	req, err := builder.BuildWithModifiedParam(context.Background(), op, "id", "user-999", original)
	require.NoError(t, err)

	gqlReq := parseGQLBody(t, req.Body)

	assert.Equal(t, "user-999", gqlReq.Variables["id"])
	assert.Equal(t, "json", gqlReq.Variables["format"])
	assert.Equal(t, "user-1", original["id"], "original map should not be mutated")
}

func TestBuild_ContentTypeHeader(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("ping", nil)

	req, err := builder.Build(context.Background(), op, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
}

func TestBuild_RequestMethodIsPOST(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("ping", nil)

	req, err := builder.Build(context.Background(), op, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "POST", req.Method)
}

func TestBuild_EmptyParameters(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("healthCheck", []core.Parameter{})

	req, err := builder.Build(context.Background(), op, map[string]any{})
	require.NoError(t, err)

	gqlReq := parseGQLBody(t, req.Body)

	assert.Contains(t, gqlReq.Query, "query healthCheck")
	assert.NotContains(t, gqlReq.Query, "(")
	assert.Equal(t, "healthCheck", gqlReq.OperationName)
	assert.Nil(t, gqlReq.Variables)
}

func TestBuild_NilGraphQLMetadataReturnsError(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Name:    "badOp",
		BaseURL: "https://api.example.com/graphql",
		GraphQL: nil,
	}

	_, err := builder.Build(context.Background(), op, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a GraphQL operation")
}

func TestBuild_DefaultHeaders(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("test", nil)

	req, err := builder.Build(context.Background(), op, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "application/json", req.Header.Get("Accept"))
	assert.NotEmpty(t, req.Header.Get("User-Agent"))
}

func TestBuild_WithBearerAuth(t *testing.T) {
	builder := NewRequestBuilder().WithAuth(&AuthConfig{
		BearerToken: "test-token-abc",
	})
	op := makeQueryOperation("secured", nil)

	req, err := builder.Build(context.Background(), op, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-token-abc", req.Header.Get("Authorization"))
}

func TestBuild_WithCustomHeaderAuth(t *testing.T) {
	builder := NewRequestBuilder().WithAuth(&AuthConfig{
		CustomHeaders: map[string]string{
			"X-API-Key": "my-key",
		},
	})
	op := makeQueryOperation("secured", nil)

	req, err := builder.Build(context.Background(), op, map[string]any{})
	require.NoError(t, err)

	assert.Equal(t, "my-key", req.Header.Get("X-API-Key"))
}

func TestBuild_WithMaxDepth(t *testing.T) {
	builder := NewRequestBuilder().WithMaxDepth(5)
	assert.Equal(t, 5, builder.MaxDepth)
}

func TestGetGraphQLTypeName(t *testing.T) {
	builder := NewRequestBuilder()

	tests := []struct {
		name     string
		param    core.Parameter
		expected string
	}{
		{
			name:     "required string",
			param:    core.Parameter{Name: "name", DataType: core.DataTypeString, Required: true},
			expected: "String!",
		},
		{
			name:     "optional string",
			param:    core.Parameter{Name: "name", DataType: core.DataTypeString, Required: false},
			expected: "String",
		},
		{
			name:     "required integer",
			param:    core.Parameter{Name: "count", DataType: core.DataTypeInteger, Required: true},
			expected: "Int!",
		},
		{
			name:     "optional integer",
			param:    core.Parameter{Name: "count", DataType: core.DataTypeInteger, Required: false},
			expected: "Int",
		},
		{
			name:     "number maps to Float",
			param:    core.Parameter{Name: "price", DataType: core.DataTypeNumber, Required: false},
			expected: "Float",
		},
		{
			name:     "boolean",
			param:    core.Parameter{Name: "active", DataType: core.DataTypeBoolean, Required: true},
			expected: "Boolean!",
		},
		{
			name: "string with id format",
			param: core.Parameter{
				Name:     "userId",
				DataType: core.DataTypeString,
				Required: true,
				Constraints: core.Constraints{
					Format: "id",
				},
			},
			expected: "ID!",
		},
		{
			name:     "object maps to JSONObject",
			param:    core.Parameter{Name: "meta", DataType: core.DataTypeObject, Required: false},
			expected: "JSONObject",
		},
		{
			name: "array of strings",
			param: core.Parameter{
				Name:     "tags",
				DataType: core.DataTypeArray,
				Required: false,
			},
			expected: "[String]",
		},
		{
			name: "array with nested integer",
			param: core.Parameter{
				Name:     "ids",
				DataType: core.DataTypeArray,
				Required: true,
				NestedParams: []core.Parameter{
					{Name: "item", DataType: core.DataTypeInteger},
				},
			},
			expected: "[Int]!",
		},
		{
			name: "enum parameter uses custom type name",
			param: core.Parameter{
				Name:     "status",
				DataType: core.DataTypeString,
				Required: false,
				Constraints: core.Constraints{
					Enum: []any{"ACTIVE", "INACTIVE"},
				},
			},
			expected: "statusEnum",
		},
		{
			name: "required enum parameter",
			param: core.Parameter{
				Name:     "role",
				DataType: core.DataTypeString,
				Required: true,
				Constraints: core.Constraints{
					Enum: []any{"ADMIN", "USER"},
				},
			},
			expected: "roleEnum!",
		},
		{
			name:     "unknown data type defaults to String",
			param:    core.Parameter{Name: "custom", DataType: core.DataType("custom"), Required: false},
			expected: "String",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := builder.getGraphQLTypeName(tt.param)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildQuery_QueryStructure(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("getUsers", []core.Parameter{
		{Name: "limit", Location: core.ParameterLocationBody, DataType: core.DataTypeInteger, Required: true},
		{Name: "offset", Location: core.ParameterLocationBody, DataType: core.DataTypeInteger},
	})

	query := builder.buildQuery(op, map[string]any{"limit": 10, "offset": 0})

	assert.Contains(t, query, "query getUsers($limit: Int!, $offset: Int)")
	assert.Contains(t, query, "getUsers(limit: $limit, offset: $offset)")
	assert.Contains(t, query, "__typename")
}

func TestBuildQuery_ReturnsEmptyForNonGraphQL(t *testing.T) {
	builder := NewRequestBuilder()
	op := core.Operation{
		Name:    "restOp",
		GraphQL: nil,
	}

	query := builder.buildQuery(op, nil)
	assert.Empty(t, query)
}

func TestBuildQuery_FieldSelectionFromSchema(t *testing.T) {
	schema := &pkgGraphql.GraphQLSchema{
		Types: map[string]pkgGraphql.TypeDef{
			"User": {
				Name: "User",
				Fields: []pkgGraphql.Field{
					{Name: "id", Type: pkgGraphql.TypeRef{Name: "ID", Kind: pkgGraphql.TypeKindScalar}},
					{Name: "name", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
					{Name: "email", Type: pkgGraphql.TypeRef{Name: "String", Kind: pkgGraphql.TypeKindScalar}},
				},
			},
		},
	}

	builder := NewRequestBuilder().WithSchema(schema)
	op := makeQueryOperation("getUser", []core.Parameter{
		{Name: "id", Location: core.ParameterLocationArgument, DataType: core.DataTypeString, Required: true},
	})

	query := builder.buildQuery(op, map[string]any{"id": "1"})

	assert.Contains(t, query, "id")
	assert.Contains(t, query, "name")
	assert.Contains(t, query, "email")
	assert.NotContains(t, query, "__typename")
}

func TestBuildQuery_FallbackToTypenameWithoutSchema(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("getUser", []core.Parameter{
		{Name: "id", Location: core.ParameterLocationArgument, DataType: core.DataTypeString, Required: true},
	})

	query := builder.buildQuery(op, map[string]any{"id": "1"})

	assert.Contains(t, query, "__typename")
}

func TestBuildIntrospectionRequest(t *testing.T) {
	req, err := BuildIntrospectionRequest(context.Background(), "https://api.example.com/graphql")
	require.NoError(t, err)

	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "application/json", req.Header.Get("Accept"))

	gqlReq := parseGQLBody(t, req.Body)
	assert.Contains(t, gqlReq.Query, "__schema")
	assert.Contains(t, gqlReq.Query, "IntrospectionQuery")
}

func TestBuildBatchRequest(t *testing.T) {
	queries := []GraphQLRequest{
		{Query: "query { user(id: 1) { name } }", OperationName: "GetUser"},
		{Query: "query { posts { title } }", OperationName: "GetPosts"},
	}

	req, err := BuildBatchRequest(context.Background(), "https://api.example.com/graphql", queries)
	require.NoError(t, err)

	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))

	data, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	var batch []GraphQLRequest
	require.NoError(t, json.Unmarshal(data, &batch))
	assert.Len(t, batch, 2)
	assert.Equal(t, "GetUser", batch[0].OperationName)
	assert.Equal(t, "GetPosts", batch[1].OperationName)
}

func TestBuildRequest_Convenience(t *testing.T) {
	op := makeQueryOperation("ping", []core.Parameter{
		{Name: "echo", Location: core.ParameterLocationBody, DataType: core.DataTypeString, Required: true},
	})

	req, err := BuildRequest(context.Background(), op, map[string]any{"echo": "hello"})
	require.NoError(t, err)

	gqlReq := parseGQLBody(t, req.Body)
	assert.Equal(t, "hello", gqlReq.Variables["echo"])
	assert.Equal(t, "POST", req.Method)
}

func TestGetDefaultParamValues(t *testing.T) {
	builder := NewRequestBuilder()
	op := makeQueryOperation("test", []core.Parameter{
		{Name: "name", DataType: core.DataTypeString, ExampleValue: "alice"},
		{Name: "count", DataType: core.DataTypeInteger, DefaultValue: 10},
		{Name: "active", DataType: core.DataTypeBoolean},
	})

	values := builder.GetDefaultParamValues(op)

	assert.Equal(t, "alice", values["name"])
	assert.Equal(t, 10, values["count"])
	assert.Equal(t, true, values["active"])
}
