package graphql

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalSDL = `type Query { hello: String }`

const richSDL = `
"""A user of the system."""
type User implements Node {
  id: ID!
  name: String!
  role: Role!
  posts(first: Int = 10, filter: PostFilter): [Post!]!
  legacyName: String @deprecated(reason: "use name")
}

interface Node { id: ID! }

type Post implements Node {
  id: ID!
  title: String!
}

union SearchResult = User | Post

enum Role { ADMIN USER }

input PostFilter {
  search: String
  limit: Int = 25
}

type Query {
  user(id: ID!): User
  search(term: String!): [SearchResult!]
}

type Mutation {
  createPost(input: PostFilter!): Post!
}

type Subscription {
  postAdded: Post!
}
`

func TestParseFromSDLReadsAMinimalSchema(t *testing.T) {
	schema, err := NewParser().ParseFromSDL([]byte(minimalSDL))
	require.NoError(t, err)
	require.Len(t, schema.Queries, 1)

	assert.Equal(t, "hello", schema.Queries[0].Name)
	assert.Equal(t, "String", schema.Queries[0].ReturnType.Signature())
}

func TestParseFromSDLReadsOperationsTypesAndDefaults(t *testing.T) {
	schema, err := NewParser().ParseFromSDL([]byte(richSDL))
	require.NoError(t, err)

	queryNames := operationNames(schema.Queries)
	assert.ElementsMatch(t, []string{"user", "search"}, queryNames)
	assert.ElementsMatch(t, []string{"createPost"}, operationNames(schema.Mutations))
	assert.ElementsMatch(t, []string{"postAdded"}, operationNames(schema.Subscriptions))

	user := findOperation(schema.Queries, "user")
	require.NotNil(t, user)
	require.Len(t, user.Arguments, 1)
	assert.Equal(t, "ID!", user.Arguments[0].Type.Signature())
	assert.True(t, user.Arguments[0].Type.Required)

	userType, ok := schema.Types["User"]
	require.True(t, ok, "object types must be reachable so selection sets can be built")
	assert.Equal(t, TypeKindObject, userType.Kind)
	assert.Contains(t, userType.Interfaces, "Node")

	// A field default has to arrive decoded, not as the literal text: sent as
	// "10" the server rejects it as a string where an Int was declared.
	posts := findField(t, userType, "posts")
	require.Len(t, posts.Arguments, 2)
	first := posts.Arguments[0]
	assert.Equal(t, "first", first.Name)
	assert.Equal(t, "10", first.DefaultLiteral)
	assert.EqualValues(t, 10, first.DefaultValue)
	assert.Equal(t, "[Post!]!", posts.Type.Signature())

	legacy := findField(t, userType, "legacyName")
	assert.True(t, legacy.IsDeprecated)
	assert.Equal(t, "use name", legacy.Deprecation)

	role, ok := schema.Enums["Role"]
	require.True(t, ok)
	assert.Len(t, role.Values, 2)

	filter, ok := schema.InputTypes["PostFilter"]
	require.True(t, ok)
	require.Len(t, filter.Fields, 2)
	assert.Equal(t, "limit", filter.Fields[1].Name)
	assert.EqualValues(t, 25, filter.Fields[1].DefaultValue)

	union, ok := schema.Types["SearchResult"]
	require.True(t, ok)
	assert.Equal(t, TypeKindUnion, union.Kind)
	assert.ElementsMatch(t, []string{"User", "Post"}, union.PossibleTypes)

	node, ok := schema.Types["Node"]
	require.True(t, ok)
	assert.Equal(t, TypeKindInterface, node.Kind)
	assert.ElementsMatch(t, []string{"User", "Post"}, node.PossibleTypes)
}

// The pasted-schema import and the stored raw definition both go through
// ParseSchema, which has to accept either shape without being told which it has.
func TestParseSchemaAcceptsSDLAndIntrospection(t *testing.T) {
	parser := NewParser()

	fromSDL, err := parser.ParseSchema([]byte(minimalSDL))
	require.NoError(t, err)
	require.Len(t, fromSDL.Queries, 1)
	assert.Equal(t, "hello", fromSDL.Queries[0].Name)

	fromJSON, err := parser.ParseSchema([]byte(`{"data":{"__schema":{
		"queryType":{"name":"Query"},
		"types":[{"kind":"OBJECT","name":"Query","fields":[
			{"name":"hello","type":{"kind":"SCALAR","name":"String"},"args":[]}
		]}]
	}}}`))
	require.NoError(t, err)
	require.Len(t, fromJSON.Queries, 1)
	assert.Equal(t, "hello", fromJSON.Queries[0].Name)
}

func TestParseSchemaRejectsContentThatIsNeitherShape(t *testing.T) {
	parser := NewParser()

	_, err := parser.ParseSchema([]byte("this is not a schema"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SDL")

	// JSON is only ever read as introspection: reporting an SDL syntax error for a
	// JSON body would point the user at the wrong problem.
	_, err = parser.ParseSchema([]byte(`{"nothing":"useful"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "introspection")

	_, err = parser.ParseSchema([]byte("   "))
	require.Error(t, err)
}

func TestParseFromSDLRejectsAnInvalidSchema(t *testing.T) {
	_, err := NewParser().ParseFromSDL([]byte("type Query { hello: Missing }"))
	require.Error(t, err, "a reference to an undefined type must not import as a valid schema")
}

func operationNames(operations []Operation) []string {
	names := make([]string, 0, len(operations))
	for _, operation := range operations {
		names = append(names, operation.Name)
	}
	return names
}

func findField(t *testing.T, typeDef TypeDef, name string) Field {
	t.Helper()
	for _, field := range typeDef.Fields {
		if field.Name == name {
			return field
		}
	}
	t.Fatalf("field %q not found on %s", name, typeDef.Name)
	return Field{}
}
