package graphql

import (
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var minimalIntrospectionJSON = `{
	"data": {
		"__schema": {
			"queryType": {"name": "Query"},
			"mutationType": {"name": "Mutation"},
			"subscriptionType": {"name": "Subscription"},
			"types": [
				{
					"kind": "OBJECT",
					"name": "Query",
					"fields": [
						{
							"name": "user",
							"description": "Fetch a user",
							"args": [
								{
									"name": "id",
									"description": "User ID",
									"type": {"kind": "NON_NULL", "name": null, "ofType": {"kind": "SCALAR", "name": "ID", "ofType": null}},
									"defaultValue": null
								}
							],
							"type": {"kind": "OBJECT", "name": "User", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "users",
							"description": "List users",
							"args": [
								{
									"name": "limit",
									"type": {"kind": "SCALAR", "name": "Int", "ofType": null},
									"defaultValue": "10"
								},
								{
									"name": "offset",
									"type": {"kind": "SCALAR", "name": "Int", "ofType": null},
									"defaultValue": null
								},
								{
									"name": "role",
									"type": {"kind": "ENUM", "name": "Role", "ofType": null},
									"defaultValue": null
								},
								{
									"name": "scores",
									"type": {"kind": "LIST", "name": null, "ofType": {"kind": "SCALAR", "name": "Float", "ofType": null}},
									"defaultValue": null
								},
								{
									"name": "active",
									"type": {"kind": "SCALAR", "name": "Boolean", "ofType": null},
									"defaultValue": null
								},
								{
									"name": "filter",
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
					"fields": [
						{
							"name": "createUser",
							"description": "Create user",
							"args": [
								{
									"name": "input",
									"type": {"kind": "NON_NULL", "name": null, "ofType": {"kind": "INPUT_OBJECT", "name": "CreateUserInput", "ofType": null}},
									"defaultValue": null
								}
							],
							"type": {"kind": "OBJECT", "name": "User", "ofType": null},
							"isDeprecated": false,
							"deprecationReason": null
						},
						{
							"name": "deleteUser",
							"description": "Delete user (deprecated)",
							"args": [
								{
									"name": "id",
									"type": {"kind": "NON_NULL", "name": null, "ofType": {"kind": "SCALAR", "name": "ID", "ofType": null}},
									"defaultValue": null
								}
							],
							"type": {"kind": "SCALAR", "name": "Boolean", "ofType": null},
							"isDeprecated": true,
							"deprecationReason": "Use removeUser instead"
						}
					],
					"inputFields": null,
					"interfaces": [],
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "OBJECT",
					"name": "Subscription",
					"fields": [
						{
							"name": "userCreated",
							"description": "Subscribe to new users",
							"args": [],
							"type": {"kind": "OBJECT", "name": "User", "ofType": null},
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
					"name": "User",
					"fields": [
						{"name": "id", "args": [], "type": {"kind": "NON_NULL", "name": null, "ofType": {"kind": "SCALAR", "name": "ID", "ofType": null}}, "isDeprecated": false, "deprecationReason": null},
						{"name": "name", "args": [], "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "isDeprecated": false, "deprecationReason": null}
					],
					"inputFields": null,
					"interfaces": [],
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "INPUT_OBJECT",
					"name": "CreateUserInput",
					"fields": null,
					"inputFields": [
						{"name": "name", "type": {"kind": "NON_NULL", "name": null, "ofType": {"kind": "SCALAR", "name": "String", "ofType": null}}, "defaultValue": null},
						{"name": "email", "type": {"kind": "NON_NULL", "name": null, "ofType": {"kind": "SCALAR", "name": "String", "ofType": null}}, "defaultValue": null},
						{"name": "age", "type": {"kind": "SCALAR", "name": "Int", "ofType": null}, "defaultValue": "25"},
						{"name": "role", "type": {"kind": "ENUM", "name": "Role", "ofType": null}, "defaultValue": "USER"},
						{"name": "address", "type": {"kind": "INPUT_OBJECT", "name": "AddressInput", "ofType": null}, "defaultValue": null}
					],
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "INPUT_OBJECT",
					"name": "AddressInput",
					"fields": null,
					"inputFields": [
						{"name": "street", "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "defaultValue": null},
						{"name": "city", "type": {"kind": "NON_NULL", "name": null, "ofType": {"kind": "SCALAR", "name": "String", "ofType": null}}, "defaultValue": null}
					],
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "INPUT_OBJECT",
					"name": "UserFilter",
					"fields": null,
					"inputFields": [
						{"name": "search", "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "defaultValue": null},
						{"name": "role", "type": {"kind": "ENUM", "name": "Role", "ofType": null}, "defaultValue": null}
					],
					"interfaces": null,
					"enumValues": null,
					"possibleTypes": null
				},
				{
					"kind": "ENUM",
					"name": "Role",
					"enumValues": [
						{"name": "ADMIN", "isDeprecated": false, "deprecationReason": null},
						{"name": "USER", "isDeprecated": false, "deprecationReason": null}
					],
					"fields": null,
					"inputFields": null,
					"interfaces": null,
					"possibleTypes": null
				},
				{"kind": "SCALAR", "name": "ID", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null},
				{"kind": "SCALAR", "name": "String", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null},
				{"kind": "SCALAR", "name": "Int", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null},
				{"kind": "SCALAR", "name": "Float", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null},
				{"kind": "SCALAR", "name": "Boolean", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null}
			],
			"directives": []
		}
	}
}`

func newTestDefinition(rawJSON string, baseURL string) *db.APIDefinition {
	return &db.APIDefinition{
		Type:          db.APIDefinitionTypeGraphQL,
		RawDefinition: []byte(rawJSON),
		BaseURL:       baseURL,
	}
}

func findCoreParam(params []core.Parameter, name string) *core.Parameter {
	for i := range params {
		if params[i].Name == name {
			return &params[i]
		}
	}
	return nil
}

func TestParse_ValidIntrospection(t *testing.T) {
	parser := NewParser()
	definition := newTestDefinition(minimalIntrospectionJSON, "https://api.example.com/graphql")

	ops, err := parser.Parse(definition)
	require.NoError(t, err)
	require.NotEmpty(t, ops)

	var queries, mutations, subscriptions []core.Operation
	for _, op := range ops {
		require.NotNil(t, op.GraphQL)
		switch op.GraphQL.OperationType {
		case "query":
			queries = append(queries, op)
		case "mutation":
			mutations = append(mutations, op)
		case "subscription":
			subscriptions = append(subscriptions, op)
		}
	}

	assert.Len(t, queries, 2)
	assert.Len(t, mutations, 2)
	assert.Len(t, subscriptions, 1)
	assert.Equal(t, 5, len(ops))
}

func TestParse_OperationTypes(t *testing.T) {
	tests := []struct {
		name              string
		json              string
		expectedQueries   int
		expectedMutations int
		expectedSubs      int
	}{
		{
			name:              "all three operation types",
			json:              minimalIntrospectionJSON,
			expectedQueries:   2,
			expectedMutations: 2,
			expectedSubs:      1,
		},
		{
			name: "query only",
			json: `{"data":{"__schema":{
				"queryType":{"name":"Query"},
				"mutationType":null,
				"subscriptionType":null,
				"types":[
					{"kind":"OBJECT","name":"Query","fields":[
						{"name":"ping","args":[],"type":{"kind":"SCALAR","name":"String","ofType":null},"isDeprecated":false,"deprecationReason":null}
					],"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null},
					{"kind":"SCALAR","name":"String","fields":null,"inputFields":null,"interfaces":null,"enumValues":null,"possibleTypes":null}
				],
				"directives":[]
			}}}`,
			expectedQueries:   1,
			expectedMutations: 0,
			expectedSubs:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			def := newTestDefinition(tt.json, "https://example.com/graphql")
			ops, err := parser.Parse(def)
			require.NoError(t, err)

			var q, m, s int
			for _, op := range ops {
				require.NotNil(t, op.GraphQL)
				switch op.GraphQL.OperationType {
				case "query":
					q++
				case "mutation":
					m++
				case "subscription":
					s++
				}
			}
			assert.Equal(t, tt.expectedQueries, q)
			assert.Equal(t, tt.expectedMutations, m)
			assert.Equal(t, tt.expectedSubs, s)
		})
	}
}

func TestParse_OperationMetadata(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://api.test.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	tests := []struct {
		name            string
		opName          string
		opType          string
		method          string
		apiType         core.APIType
		baseURL         string
		deprecated      bool
		hasDescription  bool
		expectedParamCt int
	}{
		{
			name:            "query user",
			opName:          "user",
			opType:          "query",
			method:          "POST",
			apiType:         core.APITypeGraphQL,
			baseURL:         "https://api.test.com/graphql",
			deprecated:      false,
			hasDescription:  true,
			expectedParamCt: 1,
		},
		{
			name:            "query users with multiple args",
			opName:          "users",
			opType:          "query",
			method:          "POST",
			apiType:         core.APITypeGraphQL,
			baseURL:         "https://api.test.com/graphql",
			deprecated:      false,
			hasDescription:  true,
			expectedParamCt: 6,
		},
		{
			name:            "mutation createUser",
			opName:          "createUser",
			opType:          "mutation",
			method:          "POST",
			apiType:         core.APITypeGraphQL,
			baseURL:         "https://api.test.com/graphql",
			deprecated:      false,
			hasDescription:  true,
			expectedParamCt: 1,
		},
		{
			name:            "deprecated mutation deleteUser",
			opName:          "deleteUser",
			opType:          "mutation",
			method:          "POST",
			apiType:         core.APITypeGraphQL,
			baseURL:         "https://api.test.com/graphql",
			deprecated:      true,
			hasDescription:  true,
			expectedParamCt: 1,
		},
		{
			name:            "subscription userCreated",
			opName:          "userCreated",
			opType:          "subscription",
			method:          "POST",
			apiType:         core.APITypeGraphQL,
			baseURL:         "https://api.test.com/graphql",
			deprecated:      false,
			hasDescription:  true,
			expectedParamCt: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var found *core.Operation
			for i := range ops {
				if ops[i].Name == tt.opName && ops[i].GraphQL.OperationType == tt.opType {
					found = &ops[i]
					break
				}
			}
			require.NotNilf(t, found, "operation %s (%s) not found", tt.opName, tt.opType)

			assert.Equal(t, tt.method, found.Method)
			assert.Equal(t, tt.apiType, found.APIType)
			assert.Equal(t, tt.baseURL, found.BaseURL)
			assert.Equal(t, tt.deprecated, found.Deprecated)
			assert.Equal(t, tt.deprecated, found.GraphQL.IsDeprecated)
			if tt.hasDescription {
				assert.NotEmpty(t, found.Description)
			}
			assert.Len(t, found.Parameters, tt.expectedParamCt)
		})
	}
}

func TestParse_ParameterTypeMapping(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var usersOp *core.Operation
	for i := range ops {
		if ops[i].Name == "users" {
			usersOp = &ops[i]
			break
		}
	}
	require.NotNil(t, usersOp)

	tests := []struct {
		paramName    string
		expectedType core.DataType
		required     bool
		location     core.ParameterLocation
	}{
		{"limit", core.DataTypeInteger, false, core.ParameterLocationArgument},
		{"offset", core.DataTypeInteger, false, core.ParameterLocationArgument},
		{"role", core.DataTypeString, false, core.ParameterLocationArgument},
		{"scores", core.DataTypeNumber, false, core.ParameterLocationArgument},
		{"active", core.DataTypeBoolean, false, core.ParameterLocationArgument},
		{"filter", core.DataTypeObject, false, core.ParameterLocationArgument},
	}

	for _, tt := range tests {
		t.Run(tt.paramName, func(t *testing.T) {
			param := findCoreParam(usersOp.Parameters, tt.paramName)
			require.NotNilf(t, param, "parameter %s not found", tt.paramName)
			assert.Equal(t, tt.expectedType, param.DataType)
			assert.Equal(t, tt.required, param.Required)
			assert.Equal(t, tt.location, param.Location)
		})
	}
}

func TestParse_ScalarTypeMapping(t *testing.T) {
	tests := []struct {
		name         string
		scalarName   string
		expectedType core.DataType
		format       string
	}{
		{"String maps to string", "String", core.DataTypeString, "string"},
		{"ID maps to string with id format", "ID", core.DataTypeString, "id"},
		{"Int maps to integer", "Int", core.DataTypeInteger, "int32"},
		{"Float maps to number", "Float", core.DataTypeNumber, "double"},
		{"Boolean maps to boolean", "Boolean", core.DataTypeBoolean, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			json := buildSingleArgIntrospection(tt.scalarName)
			parser := NewParser()
			def := newTestDefinition(json, "https://example.com/graphql")
			ops, err := parser.Parse(def)
			require.NoError(t, err)
			require.Len(t, ops, 1)
			require.Len(t, ops[0].Parameters, 1)

			param := ops[0].Parameters[0]
			assert.Equal(t, tt.expectedType, param.DataType)
			if tt.format != "" {
				assert.Equal(t, tt.format, param.Constraints.Format)
			}
		})
	}
}

func TestParse_RequiredArgument(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var userOp *core.Operation
	for i := range ops {
		if ops[i].Name == "user" {
			userOp = &ops[i]
			break
		}
	}
	require.NotNil(t, userOp)
	require.Len(t, userOp.Parameters, 1)

	idParam := userOp.Parameters[0]
	assert.Equal(t, "id", idParam.Name)
	assert.True(t, idParam.Required)
	assert.Equal(t, core.DataTypeString, idParam.DataType)
	assert.Equal(t, "id", idParam.Constraints.Format)
}

func TestParse_EnumConstraints(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var usersOp *core.Operation
	for i := range ops {
		if ops[i].Name == "users" {
			usersOp = &ops[i]
			break
		}
	}
	require.NotNil(t, usersOp)

	roleParam := findCoreParam(usersOp.Parameters, "role")
	require.NotNil(t, roleParam)
	assert.Equal(t, core.DataTypeString, roleParam.DataType)
	require.Len(t, roleParam.Constraints.Enum, 2)
	assert.Contains(t, roleParam.Constraints.Enum, "ADMIN")
	assert.Contains(t, roleParam.Constraints.Enum, "USER")
}

func TestParse_DefaultValuePreserved(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var usersOp *core.Operation
	for i := range ops {
		if ops[i].Name == "users" {
			usersOp = &ops[i]
			break
		}
	}
	require.NotNil(t, usersOp)

	limitParam := findCoreParam(usersOp.Parameters, "limit")
	require.NotNil(t, limitParam)
	assert.NotNil(t, limitParam.DefaultValue)
}

func TestParse_InputObjectNestedParams_NonNullWrapped(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var createUserOp *core.Operation
	for i := range ops {
		if ops[i].Name == "createUser" {
			createUserOp = &ops[i]
			break
		}
	}
	require.NotNil(t, createUserOp)
	require.Len(t, createUserOp.Parameters, 1)

	inputParam := createUserOp.Parameters[0]
	assert.Equal(t, "input", inputParam.Name)
	assert.True(t, inputParam.Required)
	assert.Equal(t, core.DataTypeObject, inputParam.DataType)
	assert.Empty(t, inputParam.NestedParams)
}

func TestParse_InputObjectNestedParams_DirectKind(t *testing.T) {
	directInputJSON := `{"data":{"__schema":{
		"queryType":{"name":"Query"},
		"mutationType":{"name":"Mutation"},
		"subscriptionType":null,
		"types":[
			{"kind":"OBJECT","name":"Query","fields":[
				{"name":"ping","args":[],"type":{"kind":"SCALAR","name":"String","ofType":null},"isDeprecated":false,"deprecationReason":null}
			],"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null},
			{"kind":"OBJECT","name":"Mutation","fields":[
				{
					"name":"createUser",
					"args":[
						{
							"name":"input",
							"type":{"kind":"INPUT_OBJECT","name":"CreateUserInput","ofType":null},
							"defaultValue":null
						}
					],
					"type":{"kind":"SCALAR","name":"String","ofType":null},
					"isDeprecated":false,
					"deprecationReason":null
				}
			],"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null},
			{
				"kind":"INPUT_OBJECT","name":"CreateUserInput","fields":null,
				"inputFields":[
					{"name":"name","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"defaultValue":null},
					{"name":"email","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"defaultValue":null},
					{"name":"age","type":{"kind":"SCALAR","name":"Int","ofType":null},"defaultValue":"25"},
					{"name":"role","type":{"kind":"ENUM","name":"Role","ofType":null},"defaultValue":"USER"},
					{"name":"address","type":{"kind":"INPUT_OBJECT","name":"AddressInput","ofType":null},"defaultValue":null}
				],
				"interfaces":null,"enumValues":null,"possibleTypes":null
			},
			{
				"kind":"INPUT_OBJECT","name":"AddressInput","fields":null,
				"inputFields":[
					{"name":"street","type":{"kind":"SCALAR","name":"String","ofType":null},"defaultValue":null},
					{"name":"city","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}},"defaultValue":null}
				],
				"interfaces":null,"enumValues":null,"possibleTypes":null
			},
			{"kind":"ENUM","name":"Role","enumValues":[
				{"name":"ADMIN","isDeprecated":false,"deprecationReason":null},
				{"name":"USER","isDeprecated":false,"deprecationReason":null}
			],"fields":null,"inputFields":null,"interfaces":null,"possibleTypes":null},
			{"kind":"SCALAR","name":"String","fields":null,"inputFields":null,"interfaces":null,"enumValues":null,"possibleTypes":null},
			{"kind":"SCALAR","name":"Int","fields":null,"inputFields":null,"interfaces":null,"enumValues":null,"possibleTypes":null}
		],
		"directives":[]
	}}}`

	parser := NewParser()
	def := newTestDefinition(directInputJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var createUserOp *core.Operation
	for i := range ops {
		if ops[i].Name == "createUser" {
			createUserOp = &ops[i]
			break
		}
	}
	require.NotNil(t, createUserOp)
	require.Len(t, createUserOp.Parameters, 1)

	inputParam := createUserOp.Parameters[0]
	assert.Equal(t, "input", inputParam.Name)
	assert.False(t, inputParam.Required)
	assert.Equal(t, core.DataTypeObject, inputParam.DataType)
	require.NotEmpty(t, inputParam.NestedParams)
	assert.Len(t, inputParam.NestedParams, 5)

	nameField := findCoreParam(inputParam.NestedParams, "name")
	require.NotNil(t, nameField)
	assert.Equal(t, core.DataTypeString, nameField.DataType)
	assert.True(t, nameField.Required)
	assert.Equal(t, core.ParameterLocationBody, nameField.Location)

	emailField := findCoreParam(inputParam.NestedParams, "email")
	require.NotNil(t, emailField)
	assert.True(t, emailField.Required)

	ageField := findCoreParam(inputParam.NestedParams, "age")
	require.NotNil(t, ageField)
	assert.Equal(t, core.DataTypeInteger, ageField.DataType)
	assert.False(t, ageField.Required)
	assert.NotNil(t, ageField.DefaultValue)

	roleField := findCoreParam(inputParam.NestedParams, "role")
	require.NotNil(t, roleField)
	assert.Equal(t, core.DataTypeString, roleField.DataType)
	assert.NotNil(t, roleField.DefaultValue)

	addressField := findCoreParam(inputParam.NestedParams, "address")
	require.NotNil(t, addressField)
	assert.Equal(t, core.DataTypeObject, addressField.DataType)
	require.NotEmpty(t, addressField.NestedParams)
	assert.Len(t, addressField.NestedParams, 2)

	streetField := findCoreParam(addressField.NestedParams, "street")
	require.NotNil(t, streetField)
	assert.Equal(t, core.DataTypeString, streetField.DataType)
	assert.False(t, streetField.Required)

	cityField := findCoreParam(addressField.NestedParams, "city")
	require.NotNil(t, cityField)
	assert.True(t, cityField.Required)
}

func TestParse_InputObjectFilterNestedParams(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var usersOp *core.Operation
	for i := range ops {
		if ops[i].Name == "users" {
			usersOp = &ops[i]
			break
		}
	}
	require.NotNil(t, usersOp)

	filterParam := findCoreParam(usersOp.Parameters, "filter")
	require.NotNil(t, filterParam)
	assert.Equal(t, core.DataTypeObject, filterParam.DataType)
	require.Len(t, filterParam.NestedParams, 2)

	searchField := findCoreParam(filterParam.NestedParams, "search")
	require.NotNil(t, searchField)
	assert.Equal(t, core.DataTypeString, searchField.DataType)

	roleField := findCoreParam(filterParam.NestedParams, "role")
	require.NotNil(t, roleField)
	assert.Equal(t, core.DataTypeString, roleField.DataType)
	assert.Len(t, roleField.Constraints.Enum, 2)
}

func TestParse_CircularInputTypes(t *testing.T) {
	circularJSON := `{"data":{"__schema":{
		"queryType":{"name":"Query"},
		"mutationType":null,
		"subscriptionType":null,
		"types":[
			{
				"kind": "OBJECT",
				"name": "Query",
				"fields": [
					{
						"name": "search",
						"args": [
							{
								"name": "filter",
								"type": {"kind": "INPUT_OBJECT", "name": "FilterInput", "ofType": null},
								"defaultValue": null
							}
						],
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
				"kind": "INPUT_OBJECT",
				"name": "FilterInput",
				"fields": null,
				"inputFields": [
					{"name": "value", "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "defaultValue": null},
					{"name": "and", "type": {"kind": "INPUT_OBJECT", "name": "FilterInput", "ofType": null}, "defaultValue": null},
					{"name": "or", "type": {"kind": "INPUT_OBJECT", "name": "FilterInput", "ofType": null}, "defaultValue": null}
				],
				"interfaces": null,
				"enumValues": null,
				"possibleTypes": null
			},
			{"kind": "SCALAR", "name": "String", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null}
		],
		"directives":[]
	}}}`

	parser := NewParser()
	def := newTestDefinition(circularJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	filterParam := findCoreParam(ops[0].Parameters, "filter")
	require.NotNil(t, filterParam)
	assert.Equal(t, core.DataTypeObject, filterParam.DataType)
	require.NotEmpty(t, filterParam.NestedParams)

	valueField := findCoreParam(filterParam.NestedParams, "value")
	require.NotNil(t, valueField)
	assert.Equal(t, core.DataTypeString, valueField.DataType)

	andField := findCoreParam(filterParam.NestedParams, "and")
	require.NotNil(t, andField)
	assert.Equal(t, core.DataTypeObject, andField.DataType)
	assert.Empty(t, andField.NestedParams)

	orField := findCoreParam(filterParam.NestedParams, "or")
	require.NotNil(t, orField)
	assert.Equal(t, core.DataTypeObject, orField.DataType)
	assert.Empty(t, orField.NestedParams)
}

func TestParse_MutuallyCircularInputTypes(t *testing.T) {
	mutualCircularJSON := `{"data":{"__schema":{
		"queryType":{"name":"Query"},
		"mutationType":null,
		"subscriptionType":null,
		"types":[
			{
				"kind": "OBJECT",
				"name": "Query",
				"fields": [
					{
						"name": "find",
						"args": [
							{"name": "nodeA", "type": {"kind": "INPUT_OBJECT", "name": "NodeA", "ofType": null}, "defaultValue": null}
						],
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
				"kind": "INPUT_OBJECT",
				"name": "NodeA",
				"fields": null,
				"inputFields": [
					{"name": "label", "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "defaultValue": null},
					{"name": "next", "type": {"kind": "INPUT_OBJECT", "name": "NodeB", "ofType": null}, "defaultValue": null}
				],
				"interfaces": null,
				"enumValues": null,
				"possibleTypes": null
			},
			{
				"kind": "INPUT_OBJECT",
				"name": "NodeB",
				"fields": null,
				"inputFields": [
					{"name": "label", "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "defaultValue": null},
					{"name": "back", "type": {"kind": "INPUT_OBJECT", "name": "NodeA", "ofType": null}, "defaultValue": null}
				],
				"interfaces": null,
				"enumValues": null,
				"possibleTypes": null
			},
			{"kind": "SCALAR", "name": "String", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null}
		],
		"directives":[]
	}}}`

	parser := NewParser()
	def := newTestDefinition(mutualCircularJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	nodeAParam := findCoreParam(ops[0].Parameters, "nodeA")
	require.NotNil(t, nodeAParam)
	assert.Equal(t, core.DataTypeObject, nodeAParam.DataType)
	require.NotEmpty(t, nodeAParam.NestedParams)

	nextField := findCoreParam(nodeAParam.NestedParams, "next")
	require.NotNil(t, nextField)
	assert.Equal(t, core.DataTypeObject, nextField.DataType)
	require.NotEmpty(t, nextField.NestedParams)

	backField := findCoreParam(nextField.NestedParams, "back")
	require.NotNil(t, backField)
	assert.Equal(t, core.DataTypeObject, backField.DataType)
	assert.Empty(t, backField.NestedParams)
}

func TestParse_DeeplyNestedInputTypes(t *testing.T) {
	deepJSON := `{"data":{"__schema":{
		"queryType":{"name":"Query"},
		"mutationType":null,
		"subscriptionType":null,
		"types":[
			{
				"kind": "OBJECT",
				"name": "Query",
				"fields": [
					{
						"name": "deep",
						"args": [{"name": "input", "type": {"kind": "INPUT_OBJECT", "name": "Level0", "ofType": null}, "defaultValue": null}],
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
			{"kind": "INPUT_OBJECT", "name": "Level0", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level1", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level1", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level2", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level2", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level3", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level3", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level4", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level4", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level5", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level5", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level6", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level6", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level7", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level7", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level8", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level8", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level9", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level9", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level10", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level10", "fields": null, "inputFields": [{"name": "child", "type": {"kind": "INPUT_OBJECT", "name": "Level11", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "INPUT_OBJECT", "name": "Level11", "fields": null, "inputFields": [{"name": "leaf", "type": {"kind": "SCALAR", "name": "String", "ofType": null}, "defaultValue": null}], "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "SCALAR", "name": "String", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null}
		],
		"directives":[]
	}}}`

	parser := NewParser()
	def := newTestDefinition(deepJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)
	require.Len(t, ops, 1)

	param := findCoreParam(ops[0].Parameters, "input")
	require.NotNil(t, param)

	depth := 0
	current := param
	for current != nil && len(current.NestedParams) > 0 {
		depth++
		child := findCoreParam(current.NestedParams, "child")
		if child == nil {
			break
		}
		current = child
	}

	assert.LessOrEqual(t, depth, maxGraphQLDepth+1)
}

func TestParse_ReturnTypeFormatting(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	tests := []struct {
		opName          string
		expectedContain string
	}{
		{"user", "User"},
		{"users", "User"},
		{"deleteUser", "Boolean"},
		{"userCreated", "User"},
	}

	for _, tt := range tests {
		t.Run(tt.opName, func(t *testing.T) {
			var found *core.Operation
			for i := range ops {
				if ops[i].Name == tt.opName {
					found = &ops[i]
					break
				}
			}
			require.NotNilf(t, found, "operation %s not found", tt.opName)
			require.NotNil(t, found.GraphQL)
			assert.Contains(t, found.GraphQL.ReturnType, tt.expectedContain)
		})
	}
}

func TestParse_DefinitionIDPropagated(t *testing.T) {
	defID := uuid.New()
	def := &db.APIDefinition{
		Type:          db.APIDefinitionTypeGraphQL,
		RawDefinition: []byte(minimalIntrospectionJSON),
		BaseURL:       "https://example.com/graphql",
	}
	def.ID = defID

	parser := NewParser()
	ops, err := parser.Parse(def)
	require.NoError(t, err)
	require.NotEmpty(t, ops)

	for _, op := range ops {
		assert.Equal(t, defID, op.DefinitionID)
	}
}

func TestParse_BaseURLFallback(t *testing.T) {
	tests := []struct {
		name        string
		baseURL     string
		sourceURL   string
		expectedURL string
	}{
		{
			name:        "uses base URL when set",
			baseURL:     "https://api.example.com/graphql",
			sourceURL:   "https://discovery.example.com/graphql",
			expectedURL: "https://api.example.com/graphql",
		},
		{
			name:        "falls back to source URL when base URL is empty",
			baseURL:     "",
			sourceURL:   "https://discovery.example.com/graphql",
			expectedURL: "https://discovery.example.com/graphql",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := &db.APIDefinition{
				Type:          db.APIDefinitionTypeGraphQL,
				RawDefinition: []byte(minimalIntrospectionJSON),
				BaseURL:       tt.baseURL,
				SourceURL:     tt.sourceURL,
			}

			parser := NewParser()
			ops, err := parser.Parse(def)
			require.NoError(t, err)
			require.NotEmpty(t, ops)

			for _, op := range ops {
				assert.Equal(t, tt.expectedURL, op.BaseURL)
			}
		})
	}
}

func TestParse_ErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		def     *db.APIDefinition
		wantErr string
	}{
		{
			name: "wrong definition type",
			def: &db.APIDefinition{
				Type:          db.APIDefinitionTypeOpenAPI,
				RawDefinition: []byte(minimalIntrospectionJSON),
			},
			wantErr: "expected GraphQL definition",
		},
		{
			name: "empty raw definition",
			def: &db.APIDefinition{
				Type:          db.APIDefinitionTypeGraphQL,
				RawDefinition: nil,
			},
			wantErr: "empty raw definition",
		},
		{
			name: "invalid JSON",
			def: &db.APIDefinition{
				Type:          db.APIDefinitionTypeGraphQL,
				RawDefinition: []byte("not json"),
			},
			wantErr: "failed to parse GraphQL schema",
		},
		{
			name: "valid JSON but no schema",
			def: &db.APIDefinition{
				Type:          db.APIDefinitionTypeGraphQL,
				RawDefinition: []byte(`{"data": null}`),
			},
			wantErr: "failed to parse GraphQL schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser()
			_, err := parser.Parse(tt.def)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseFromRawDefinition(t *testing.T) {
	ops, err := ParseFromRawDefinition([]byte(minimalIntrospectionJSON), "https://example.com/graphql")
	require.NoError(t, err)
	require.NotEmpty(t, ops)

	for _, op := range ops {
		assert.Equal(t, "https://example.com/graphql", op.BaseURL)
		assert.Equal(t, core.APITypeGraphQL, op.APIType)
		assert.NotNil(t, op.GraphQL)
	}
}

func TestParse_FormatTypeRef(t *testing.T) {
	parser := NewParser()
	def := newTestDefinition(minimalIntrospectionJSON, "https://example.com/graphql")
	ops, err := parser.Parse(def)
	require.NoError(t, err)

	var usersOp *core.Operation
	for i := range ops {
		if ops[i].Name == "users" {
			usersOp = &ops[i]
			break
		}
	}
	require.NotNil(t, usersOp)
	require.NotNil(t, usersOp.GraphQL)
	assert.NotEmpty(t, usersOp.GraphQL.ReturnType)
	assert.Contains(t, usersOp.GraphQL.ReturnType, "[")
	assert.Contains(t, usersOp.GraphQL.ReturnType, "]")
	assert.Contains(t, usersOp.GraphQL.ReturnType, "!")
}

func buildSingleArgIntrospection(scalarName string) string {
	return `{"data":{"__schema":{
		"queryType":{"name":"Query"},
		"mutationType":null,
		"subscriptionType":null,
		"types":[
			{
				"kind": "OBJECT",
				"name": "Query",
				"fields": [
					{
						"name": "test",
						"args": [
							{"name": "arg", "type": {"kind": "SCALAR", "name": "` + scalarName + `", "ofType": null}, "defaultValue": null}
						],
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
			{"kind": "SCALAR", "name": "` + scalarName + `", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null},
			{"kind": "SCALAR", "name": "String", "fields": null, "inputFields": null, "interfaces": null, "enumValues": null, "possibleTypes": null}
		],
		"directives":[]
	}}}`
}
