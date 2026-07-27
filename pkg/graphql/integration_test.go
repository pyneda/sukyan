package graphql

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The whole point of this package is to produce requests a server will actually
// execute. These schemas cover the shapes real APIs are built from, and every
// request generated from them is held to what a server enforces before any
// resolver runs. A regression here means the scanner is testing nothing.

var schemaLibrary = map[string]string{
	"relay connections": `
	interface Node { id: ID! }
	type PageInfo { hasNextPage: Boolean!, endCursor: String }
	type PostEdge { cursor: String!, node: Post! }
	type PostConnection { edges: [PostEdge!]!, pageInfo: PageInfo!, totalCount: Int! }
	type Post implements Node { id: ID!, title: String!, author: User! }
	type User implements Node {
	  id: ID!
	  name: String
	  posts(first: Int!, after: String): PostConnection!
	}
	type Query {
	  node(id: ID!): Node
	  viewer: User
	  users(first: Int = 10): [User!]!
	}
	`,

	"unions and interfaces": `
	interface Entity { id: ID! }
	type User implements Entity { id: ID!, email: String! }
	type Team implements Entity { id: ID!, slug: String! }
	type NotFound { message: String! }
	union SearchResult = User | Team
	union LookupResult = User | Team | NotFound
	type Query {
	  search(term: String!): [SearchResult!]!
	  lookup(id: ID!): LookupResult!
	  entity(id: ID!): Entity
	}
	`,

	"nested input objects": `
	enum SortDirection { ASC DESC }
	input Sort { field: String!, direction: SortDirection! = ASC }
	input Range { min: Int, max: Int }
	input Filter { term: String!, range: Range, sorts: [Sort!], negate: Boolean = false }
	input SearchInput { filter: Filter!, page: Int = 1 }
	type Result { id: ID!, score: Float! }
	type Query { search(input: SearchInput!): [Result!]! }
	`,

	"mutations and subscriptions": `
	scalar DateTime
	enum Role { ADMIN EDITOR VIEWER }
	input CreateUser { email: String!, name: String, role: Role! = VIEWER }
	type User { id: ID!, email: String!, role: Role!, createdAt: DateTime! }
	type Query { me: User }
	type Mutation {
	  createUser(input: CreateUser!): User!
	  deleteUser(id: ID!): Boolean!
	  touch: DateTime!
	}
	type Subscription { userCreated(role: Role): User! }
	`,

	"custom scalars as leaves": `
	scalar JSON
	scalar Upload
	scalar DateTime
	type Query {
	  config: JSON!
	  configs: [JSON!]!
	  now: DateTime
	}
	type Mutation { upload(file: Upload!): Boolean! }
	`,

	"recursive types": `
	input TreeFilter { name: String, child: TreeFilter }
	type Tree { id: ID!, name: String, parent: Tree, children: [Tree!]! }
	type Query { tree(filter: TreeFilter): Tree, forest: [Tree!]! }
	`,

	"root type exposed as a field": `
	type Query { viewer: Query, hello: String, nested: Query! }
	`,

	"required arguments on nested fields": `
	enum Unit { METRIC IMPERIAL }
	input Precision { digits: Int! }
	type Reading { value: Float!, formatted(unit: Unit!, precision: Precision!): String! }
	type Sensor { id: ID!, reading(at: String!): Reading! }
	type Query { sensor(id: ID!): Sensor }
	`,

	"deprecated members": `
	enum Status { LIVE, OLD @deprecated(reason: "gone") }
	type User { id: ID!, legacyName: String @deprecated(reason: "use name"), name: String }
	type Query { me: User @deprecated(reason: "use viewer"), viewer: User }
	`,

	"defaults on every argument": `
	enum Mode { FAST SLOW }
	input Opts { retries: Int! = 3, label: String = "none" }
	type Query {
	  run(
	    count: Int = 0
	    ratio: Float = 0.5
	    label: String = "default"
	    enabled: Boolean = false
	    mode: Mode = FAST
	    tags: [String!] = ["a"]
	    matrix: [[Int!]!] = [[1]]
	    opts: Opts = {retries: 1}
	  ): String
	}
	`,

	"nested list types": `
	type Query {
	  grid(cells: [[Int!]!]!): Boolean!
	  cube(values: [[[String]]]): Boolean!
	  sparse(rows: [[Int]]): Boolean!
	}
	`,
}

func TestGeneratedRequestsAreAcceptedByAServer(t *testing.T) {
	for name, sdl := range schemaLibrary {
		t.Run(name, func(t *testing.T) {
			endpoints := assertGeneratedRequestsAreValid(t, sdl, DefaultGenerationConfig())
			require.NotEmpty(t, endpoints, "schema produced no operations")

			for _, endpoint := range endpoints {
				assert.NotEmpty(t, endpoint.Requests, "operation %q produced no request", endpoint.Name)
			}
		})
	}
}

// Fuzz values are deliberately out of range, but the document carrying them still
// has to be one the server will parse and validate, or the payload never lands.
func TestFuzzRequestsProduceValidDocuments(t *testing.T) {
	config := DefaultGenerationConfig()
	config.FuzzingEnabled = true

	for name, sdl := range schemaLibrary {
		t.Run(name, func(t *testing.T) {
			assertGeneratedRequestsAreValid(t, sdl, config)
		})
	}
}

func TestEveryOperationYieldsAnEndpoint(t *testing.T) {
	const sdl = `
	type User { id: ID! }
	type Query { a: String, b: String }
	type Mutation { c: String }
	type Subscription { d: String }
	`

	schema, _ := parseSDL(t, sdl)
	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()

	byType := map[string][]string{}
	for _, endpoint := range endpoints {
		byType[endpoint.OperationType] = append(byType[endpoint.OperationType], endpoint.Name)
	}

	assert.Equal(t, []string{"a", "b"}, byType["query"])
	assert.Equal(t, []string{"c"}, byType["mutation"])
	assert.Equal(t, []string{"d"}, byType["subscription"])
}

// A scan has to be reproducible, so the same schema must queue the same requests
// in the same order every run.
func TestGenerationIsDeterministic(t *testing.T) {
	config := DefaultGenerationConfig()
	config.FuzzingEnabled = true

	schema, _ := parseSDL(t, schemaLibrary["nested input objects"])

	first := NewGenerator(schema, config).GenerateRequests()
	for i := 0; i < 5; i++ {
		again := NewGenerator(schema, config).GenerateRequests()
		require.Len(t, again, len(first))

		for j := range first {
			assert.Equal(t, first[j].Name, again[j].Name)
			require.Len(t, again[j].Requests, len(first[j].Requests))
			for k := range first[j].Requests {
				assert.Equal(t, first[j].Requests[k].Query, again[j].Requests[k].Query)
				assert.Equal(t, first[j].Requests[k].Label, again[j].Requests[k].Label)
			}
		}
	}
}

// Nested lists must keep their shape. Collapsing [[Int!]!] to a flat list sends a
// value the server refuses to coerce, and the operation is never exercised.
func TestNestedListVariablesKeepTheirShape(t *testing.T) {
	schema, oracle := parseSDL(t, schemaLibrary["nested list types"])
	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()

	byName := map[string]RequestVariation{}
	for _, endpoint := range endpoints {
		byName[endpoint.Name] = endpoint.Requests[0]
	}

	grid := byName["grid"]
	cells, ok := grid.Variables["cells"].([]interface{})
	require.True(t, ok, "cells should be a list, got %T", grid.Variables["cells"])
	require.Len(t, cells, 1)
	assert.IsType(t, []interface{}{}, cells[0], "[[Int!]!]! must produce a list of lists")

	cube := byName["cube"]
	outer, ok := cube.Variables["values"].([]interface{})
	require.True(t, ok)
	require.Len(t, outer, 1)
	middle, ok := outer[0].([]interface{})
	require.True(t, ok, "[[[String]]] must nest three levels")
	require.Len(t, middle, 1)
	assert.IsType(t, []interface{}{}, middle[0])

	for _, req := range byName {
		assertValidVariables(t, oracle, req.Query, req.Variables)
	}
}

// A defaulted argument is where the literal-versus-JSON confusion shows up, so
// the baseline request is checked against the server's own coercion rules.
func TestDefaultedArgumentsCoerceCorrectly(t *testing.T) {
	schema, oracle := parseSDL(t, schemaLibrary["defaults on every argument"])
	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()

	require.Len(t, endpoints, 1)
	request := endpoints[0].Requests[0]

	assert.Equal(t, int64(0), request.Variables["count"])
	assert.Equal(t, 0.5, request.Variables["ratio"])
	assert.Equal(t, "default", request.Variables["label"])
	assert.Equal(t, false, request.Variables["enabled"])
	assert.Equal(t, "FAST", request.Variables["mode"])

	assertValidVariables(t, oracle, request.Query, request.Variables)
}

// Custom scalars are leaves. Appending a selection set to one is a validation
// error, and they are only recognisable through the schema's scalar list.
func TestCustomScalarsGetNoSelectionSet(t *testing.T) {
	schema, oracle := parseSDL(t, schemaLibrary["custom scalars as leaves"])
	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()

	for _, endpoint := range endpoints {
		for _, req := range endpoint.Requests {
			if endpoint.Name == "config" || endpoint.Name == "configs" || endpoint.Name == "now" {
				assert.NotContains(t, req.Query, "__typename",
					"a custom scalar must not be given a selection set")
			}
			assertValidDocument(t, oracle, req.Query)
		}
	}
}

// Variations multiply with argument count, so without a cap a few wide
// operations consume a scan's entire request budget.
func TestRequestsPerOperationAreCapped(t *testing.T) {
	const sdl = `type Query { wide(a: String, b: String, c: String, d: String): String }`

	schema, _ := parseSDL(t, sdl)

	uncapped := DefaultGenerationConfig()
	uncapped.FuzzingEnabled = true
	uncapped.MaxRequestsPerOperation = 0
	full := NewGenerator(schema, uncapped).GenerateRequests()
	require.Len(t, full, 1)
	require.Greater(t, len(full[0].Requests), 10, "this schema should fan out")

	capped := uncapped
	capped.MaxRequestsPerOperation = 5
	limited := NewGenerator(schema, capped).GenerateRequests()
	require.Len(t, limited, 1)
	assert.Len(t, limited[0].Requests, 5)
	assert.Equal(t, labelHappyPath, limited[0].Requests[0].Label, "the baseline request is never dropped")
}

// The names in a schema come from the target. One that is not a legal GraphQL
// name would otherwise be spliced straight into every document built from it.
func TestHostileSchemaCannotInjectIntoDocuments(t *testing.T) {
	schema := &GraphQLSchema{
		Queries: []Operation{
			{
				Name:       "evil { hacked } query x",
				ReturnType: named("String", TypeKindScalar),
			},
			{
				Name:       "legit",
				ReturnType: named("String", TypeKindScalar),
				Arguments: []Argument{
					{Name: "ok", Type: named("String", TypeKindScalar)},
					{Name: "bad) { hacked } (", Type: named("String", TypeKindScalar)},
				},
			},
		},
		Types:      map[string]TypeDef{},
		Enums:      map[string]EnumDef{},
		InputTypes: map[string]InputTypeDef{},
	}

	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()

	require.Len(t, endpoints, 1, "the operation with an illegal name is dropped entirely")
	assert.Equal(t, "legit", endpoints[0].Name)

	request := endpoints[0].Requests[0]
	assert.NotContains(t, request.Query, "hacked")
	assert.Contains(t, request.Query, "$ok")
	assert.NotContains(t, request.Variables, "bad) { hacked } (",
		"a dropped argument must also be dropped from the variables it would be declared for")
}

func TestToHTTPRequestShape(t *testing.T) {
	schema, _ := parseSDL(t, `type Query { hello(name: String): String }`)
	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()
	require.Len(t, endpoints, 1)

	req, err := endpoints[0].Requests[0].ToHTTPRequest("https://example.com/graphql")
	require.NoError(t, err)

	assert.Equal(t, "POST", req.Method)
	assert.Equal(t, "https://example.com/graphql", req.URL)
	assert.Equal(t, "application/json", req.Headers["Content-Type"])

	body := string(req.Body)
	assert.Contains(t, body, `"query"`)
	assert.Contains(t, body, `"variables"`)
	assert.Contains(t, body, `"operationName":"hello"`)
}

// A schema large enough to be realistic must not turn into a document no server
// will accept, and must not take an unreasonable amount of work to build.
func TestWideSchemaStaysBounded(t *testing.T) {
	var sdl strings.Builder
	sdl.WriteString("type Query {\n")
	for i := 0; i < 60; i++ {
		sdl.WriteString("  op")
		sdl.WriteString(string(rune('a' + i%26)))
		sdl.WriteString(string(rune('a' + i/26)))
		sdl.WriteString("(arg: String): Wide\n")
	}
	sdl.WriteString("}\ntype Wide {\n")
	for i := 0; i < 60; i++ {
		sdl.WriteString("  f")
		sdl.WriteString(string(rune('a' + i%26)))
		sdl.WriteString(string(rune('a' + i/26)))
		sdl.WriteString(": Wide\n")
	}
	sdl.WriteString("  leaf: String\n}\n")

	schema, oracle := parseSDL(t, sdl.String())
	endpoints := NewGenerator(schema, DefaultGenerationConfig()).GenerateRequests()
	require.Len(t, endpoints, 60)

	for _, endpoint := range endpoints {
		query := endpoint.Requests[0].Query
		assert.Less(t, len(query), 256*1024, "selection sets must stay bounded on a wide schema")
		assertValidDocument(t, oracle, query)
	}
}

func TestGeneratorHandlesEmptyAndNilSchemas(t *testing.T) {
	assert.NotPanics(t, func() {
		assert.Empty(t, NewGenerator(nil, DefaultGenerationConfig()).GenerateRequests())
		assert.Empty(t, NewGenerator(&GraphQLSchema{}, GenerationConfig{}).GenerateRequests())
	})
}
