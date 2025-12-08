package graphql

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Sample introspection response for testing
var sampleIntrospectionJSON = `{
	"data": {
		"__schema": {
			"queryType": {"name": "Query"},
			"mutationType": {"name": "Mutation"},
			"subscriptionType": null,
			"types": [
				{
					"kind": "OBJECT",
					"name": "Query",
					"description": "Root query type",
					"fields": [
						{
							"name": "user",
							"description": "Get a user by ID",
							"args": [
								{
									"name": "id",
									"description": "User ID",
									"type": {
										"kind": "NON_NULL",
										"name": null,
										"ofType": {"kind": "SCALAR", "name": "ID", "ofType": null}
									},
									"defaultValue": null
								}
							],
							"type": {"kind": "OBJECT", "name": "User", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "users",
							"description": "Get all users",
							"args": [
								{
									"name": "limit",
									"description": "Max results",
									"type": {"kind": "SCALAR", "name": "Int", "ofType": null},
									"defaultValue": "10"
								},
								{
									"name": "filter",
									"description": "Filter input",
									"type": {"kind": "INPUT_OBJECT", "name": "UserFilter", "ofType": null},
									"defaultValue": null
								}
							],
							"type": {
								"kind": "NON_NULL",
								"name": null,
								"ofType": {
									"kind": "LIST",
									"name": null,
									"ofType": {"kind": "OBJECT", "name": "User", "ofType": null}
								}
							},
							"isDeprecated": false,
							"deprecationReason": null
						}
					],
					"inputFields": null,
					"interfaces": [],
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "OBJECT",
					"name": "Mutation",
					"description": "Root mutation type",
					"fields": [
						{
							"name": "createUser",
							"description": "Create a new user",
							"args": [
								{
									"name": "input",
									"description": "User input",
									"type": {
										"kind": "NON_NULL",
										"name": null,
										"ofType": {"kind": "INPUT_OBJECT", "name": "CreateUserInput", "ofType": null}
									},
									"defaultValue": null
								}
							],
							"type": {"kind": "OBJECT", "name": "User", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "updateUser",
							"description": "Update an existing user",
							"args": [
								{
									"name": "id",
									"description": "User ID",
									"type": {
										"kind": "NON_NULL",
										"name": null,
										"ofType": {"kind": "SCALAR", "name": "ID", "ofType": null}
									},
									"defaultValue": null
								},
								{
									"name": "input",
									"description": "Update input",
									"type": {
										"kind": "NON_NULL",
										"name": null,
										"ofType": {"kind": "INPUT_OBJECT", "name": "UpdateUserInput", "ofType": null}
									},
									"defaultValue": null
								}
							],
							"type": {"kind": "OBJECT", "name": "User", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "deleteUser",
							"description": "Delete a user (deprecated)",
							"args": [
								{
									"name": "id",
									"description": "User ID",
									"type": {
										"kind": "NON_NULL",
										"name": null,
										"ofType": {"kind": "SCALAR", "name": "ID", "ofType": null}
									},
									"defaultValue": null
								}
							],
							"type": {"kind": "SCALAR", "name": "Boolean", "ofType": null},
							"isDeprecated": true,
							"deprecationReason": "Use deactivateUser instead"
						}
					],
					"inputFields": null,
					"interfaces": [],
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "OBJECT",
					"name": "User",
					"description": "A user in the system",
					"fields": [
						{
							"name": "id",
							"description": "User ID",
							"args": [],
							"type": {
								"kind": "NON_NULL",
								"name": null,
								"ofType": {"kind": "SCALAR", "name": "ID", "ofType": null}
							},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "name",
							"description": "User name",
							"args": [],
							"type": {"kind": "SCALAR", "name": "String", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "email",
							"description": "User email",
							"args": [],
							"type": {
								"kind": "NON_NULL",
								"name": null,
								"ofType": {"kind": "SCALAR", "name": "String", "ofType": null}
							},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "role",
							"description": "User role",
							"args": [],
							"type": {"kind": "ENUM", "name": "UserRole", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "createdAt",
							"description": "Creation timestamp",
							"args": [],
							"type": {"kind": "SCALAR", "name": "DateTime", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						}
					],
					"inputFields": null,
					"interfaces": [],
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "INPUT_OBJECT",
					"name": "CreateUserInput",
					"description": "Input for creating a user",
					"fields": null,
					"inputFields": [
						{
							"name": "name",
							"description": "User name",
							"type": {
								"kind": "NON_NULL",
								"name": null,
								"ofType": {"kind": "SCALAR", "name": "String", "ofType": null}
							},
							"defaultValue": null
						},
						{
							"name": "email",
							"description": "User email",
							"type": {
								"kind": "NON_NULL",
								"name": null,
								"ofType": {"kind": "SCALAR", "name": "String", "ofType": null}
							},
							"defaultValue": null
						},
						{
							"name": "role",
							"description": "User role",
							"type": {"kind": "ENUM", "name": "UserRole", "ofType": null},
							"defaultValue": "USER"
						}
					],
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "INPUT_OBJECT",
					"name": "UpdateUserInput",
					"description": "Input for updating a user",
					"fields": null,
					"inputFields": [
						{
							"name": "name",
							"description": "User name",
							"type": {"kind": "SCALAR", "name": "String", "ofType": null},
							"defaultValue": null
						},
						{
							"name": "email",
							"description": "User email",
							"type": {"kind": "SCALAR", "name": "String", "ofType": null},
							"defaultValue": null
						},
						{
							"name": "role",
							"description": "User role",
							"type": {"kind": "ENUM", "name": "UserRole", "ofType": null},
							"defaultValue": null
						}
					],
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "INPUT_OBJECT",
					"name": "UserFilter",
					"description": "Filter for user queries",
					"fields": null,
					"inputFields": [
						{
							"name": "role",
							"description": "Filter by role",
							"type": {"kind": "ENUM", "name": "UserRole", "ofType": null},
							"defaultValue": null
						},
						{
							"name": "search",
							"description": "Search term",
							"type": {"kind": "SCALAR", "name": "String", "ofType": null},
							"defaultValue": null
						}
					],
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "ENUM",
					"name": "UserRole",
					"description": "User roles",
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"enumValues": [
						{"name": "ADMIN", "description": "Administrator", "isDeprecated": false, "deprecationReason": null},
						{"name": "USER", "description": "Regular user", "isDeprecated": false, "deprecationReason": null},
						{"name": "GUEST", "description": "Guest user", "isDeprecated": true, "deprecationReason": "Use USER instead"}
					],
					"possibleTypes": null
				},
				{
					"kind": "SCALAR",
					"name": "ID",
					"description": "Built-in ID scalar",
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "SCALAR",
					"name": "String",
					"description": "Built-in String scalar",
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "SCALAR",
					"name": "Int",
					"description": "Built-in Int scalar",
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "SCALAR",
					"name": "Boolean",
					"description": "Built-in Boolean scalar",
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "SCALAR",
					"name": "DateTime",
					"description": "Custom DateTime scalar",
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				}
			],
			"directives": [
				{
					"name": "deprecated",
					"description": "Marks an element as deprecated",
					"locations": ["FIELD_DEFINITION", "ENUM_VALUE"],
					"args": [
						{
							"name": "reason",
							"description": "Deprecation reason",
							"type": {"kind": "SCALAR", "name": "String", "ofType": null},
							"defaultValue": "\"No longer supported\""
						}
					]
				}
			]
		}
	}
}`

func TestParseFromJSON(t *testing.T) {
	parser := NewParser()
	schema, err := parser.ParseFromJSON([]byte(sampleIntrospectionJSON))

	require.NoError(t, err)
	require.NotNil(t, schema)

	// Check queries
	assert.Len(t, schema.Queries, 2)

	// Check user query
	userQuery := findOperation(schema.Queries, "user")
	require.NotNil(t, userQuery)
	assert.Equal(t, "Get a user by ID", userQuery.Description)
	assert.Len(t, userQuery.Arguments, 1)
	assert.Equal(t, "id", userQuery.Arguments[0].Name)
	assert.True(t, userQuery.Arguments[0].Type.Required)

	// Check users query with optional args
	usersQuery := findOperation(schema.Queries, "users")
	require.NotNil(t, usersQuery)
	assert.Len(t, usersQuery.Arguments, 2)
	limitArg := findArgument(usersQuery.Arguments, "limit")
	require.NotNil(t, limitArg)
	assert.False(t, limitArg.Type.Required)

	// Check mutations
	assert.Len(t, schema.Mutations, 3)

	// Check createUser mutation
	createUserMutation := findOperation(schema.Mutations, "createUser")
	require.NotNil(t, createUserMutation)
	assert.Len(t, createUserMutation.Arguments, 1)
	assert.Equal(t, "input", createUserMutation.Arguments[0].Name)
	assert.True(t, createUserMutation.Arguments[0].Type.Required)

	// Check deprecated mutation
	deleteUserMutation := findOperation(schema.Mutations, "deleteUser")
	require.NotNil(t, deleteUserMutation)
	assert.True(t, deleteUserMutation.IsDeprecated)
	assert.Equal(t, "Use deactivateUser instead", deleteUserMutation.Deprecation)

	// Check types
	assert.Contains(t, schema.Types, "User")
	userType := schema.Types["User"]
	assert.Len(t, userType.Fields, 5)

	// Check enums
	assert.Contains(t, schema.Enums, "UserRole")
	userRole := schema.Enums["UserRole"]
	assert.Len(t, userRole.Values, 3)

	// Check input types
	assert.Contains(t, schema.InputTypes, "CreateUserInput")
	createUserInput := schema.InputTypes["CreateUserInput"]
	assert.Len(t, createUserInput.Fields, 3)

	// Check scalars
	assert.Contains(t, schema.Scalars, "DateTime")
}

func TestConvertTypeRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      IntrospectionTypeRef
		expected TypeRef
	}{
		{
			name: "simple scalar",
			ref: IntrospectionTypeRef{
				Kind: "SCALAR",
				Name: "String",
			},
			expected: TypeRef{
				Kind:     TypeKindScalar,
				Name:     "String",
				Required: false,
				IsList:   false,
			},
		},
		{
			name: "non-null scalar",
			ref: IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "SCALAR",
					Name: "String",
				},
			},
			expected: TypeRef{
				Kind:     TypeKindNonNull,
				Required: true,
				IsList:   false,
			},
		},
		{
			name: "list of scalars",
			ref: IntrospectionTypeRef{
				Kind: "LIST",
				OfType: &IntrospectionTypeRef{
					Kind: "SCALAR",
					Name: "String",
				},
			},
			expected: TypeRef{
				Kind:     TypeKindList,
				Required: false,
				IsList:   true,
			},
		},
		{
			name: "non-null list of non-null scalars",
			ref: IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "LIST",
					OfType: &IntrospectionTypeRef{
						Kind: "NON_NULL",
						OfType: &IntrospectionTypeRef{
							Kind: "SCALAR",
							Name: "String",
						},
					},
				},
			},
			expected: TypeRef{
				Kind:     TypeKindNonNull,
				Required: true,
				IsList:   true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTypeRef(tt.ref)
			assert.Equal(t, tt.expected.Kind, result.Kind)
			assert.Equal(t, tt.expected.Required, result.Required)
			assert.Equal(t, tt.expected.IsList, result.IsList)
		})
	}
}

func TestFormatTypeSignature(t *testing.T) {
	tests := []struct {
		name     string
		ref      IntrospectionTypeRef
		expected string
	}{
		{
			name: "simple String",
			ref: IntrospectionTypeRef{
				Kind: "SCALAR",
				Name: "String",
			},
			expected: "String",
		},
		{
			name: "non-null String",
			ref: IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "SCALAR",
					Name: "String",
				},
			},
			expected: "String!",
		},
		{
			name: "list of strings",
			ref: IntrospectionTypeRef{
				Kind: "LIST",
				OfType: &IntrospectionTypeRef{
					Kind: "SCALAR",
					Name: "String",
				},
			},
			expected: "[String]",
		},
		{
			name: "[String!]!",
			ref: IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "LIST",
					OfType: &IntrospectionTypeRef{
						Kind: "NON_NULL",
						OfType: &IntrospectionTypeRef{
							Kind: "SCALAR",
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
			result := formatTypeSignature(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetBaseTypeName(t *testing.T) {
	tests := []struct {
		name     string
		ref      IntrospectionTypeRef
		expected string
	}{
		{
			name: "simple scalar",
			ref: IntrospectionTypeRef{
				Kind: "SCALAR",
				Name: "String",
			},
			expected: "String",
		},
		{
			name: "wrapped in non-null",
			ref: IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "SCALAR",
					Name: "Int",
				},
			},
			expected: "Int",
		},
		{
			name: "deeply nested",
			ref: IntrospectionTypeRef{
				Kind: "NON_NULL",
				OfType: &IntrospectionTypeRef{
					Kind: "LIST",
					OfType: &IntrospectionTypeRef{
						Kind: "NON_NULL",
						OfType: &IntrospectionTypeRef{
							Kind: "OBJECT",
							Name: "User",
						},
					},
				},
			},
			expected: "User",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getBaseTypeName(tt.ref)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseFromJSONDataOnly(t *testing.T) {
	// Test parsing when JSON contains only the data portion without wrapper
	dataOnlyJSON := `{
		"__schema": {
			"queryType": {"name": "Query"},
			"mutationType": null,
			"subscriptionType": null,
			"types": [
				{
					"kind": "OBJECT",
					"name": "Query",
					"description": null,
					"fields": [
						{
							"name": "hello",
							"description": null,
							"args": [],
							"type": {"kind": "SCALAR", "name": "String", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						}
					],
					"inputFields": null,
					"interfaces": [],
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "SCALAR",
					"name": "String",
					"description": null,
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				}
			],
			"directives": []
		}
	}`

	parser := NewParser()
	schema, err := parser.ParseFromJSON([]byte(dataOnlyJSON))

	require.NoError(t, err)
	require.NotNil(t, schema)
	assert.Len(t, schema.Queries, 1)
	assert.Equal(t, "hello", schema.Queries[0].Name)
}

func TestParseFromJSONErrors(t *testing.T) {
	parser := NewParser()

	// Test invalid JSON
	_, err := parser.ParseFromJSON([]byte("not json"))
	assert.Error(t, err)

	// Test valid JSON but missing schema
	_, err = parser.ParseFromJSON([]byte(`{"data": null}`))
	assert.Error(t, err)
}

// Helper functions

func findOperation(ops []Operation, name string) *Operation {
	for i := range ops {
		if ops[i].Name == name {
			return &ops[i]
		}
	}
	return nil
}

func findArgument(args []Argument, name string) *Argument {
	for i := range args {
		if args[i].Name == name {
			return &args[i]
		}
	}
	return nil
}

func TestIntrospectionQueryIsValid(t *testing.T) {
	// Basic sanity check that the introspection query is well-formed
	assert.Contains(t, IntrospectionQuery, "__schema")
	assert.Contains(t, IntrospectionQuery, "queryType")
	assert.Contains(t, IntrospectionQuery, "mutationType")
	assert.Contains(t, IntrospectionQuery, "subscriptionType")
	assert.Contains(t, IntrospectionQuery, "types")
	assert.Contains(t, IntrospectionQuery, "fields")
}

func TestSchemaJSONSerialization(t *testing.T) {
	parser := NewParser()
	schema, err := parser.ParseFromJSON([]byte(sampleIntrospectionJSON))
	require.NoError(t, err)

	// Test that schema can be serialized to JSON
	jsonData, err := json.Marshal(schema)
	require.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Test that serialized JSON can be unmarshaled
	var unmarshaled GraphQLSchema
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, len(schema.Queries), len(unmarshaled.Queries))
	assert.Equal(t, len(schema.Mutations), len(unmarshaled.Mutations))
}
