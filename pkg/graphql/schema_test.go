package graphql

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A field returning an interface or a union is not a leaf. Leaving those types
// out of the schema makes them look like scalars to every selection set builder,
// and the resulting document is refused during validation, so the operation is
// never reached at all.
func TestSelectionSetForAbstractReturnTypes(t *testing.T) {
	const sdl = `
	interface Node { id: ID! }
	type User implements Node { id: ID!, name: String }
	type Post implements Node { id: ID!, title: String }
	union SearchResult = User | Post
	type Query {
	  node(id: ID!): Node
	  search(term: String!): [SearchResult!]!
	}
	`

	schema, oracle := parseSDL(t, sdl)

	t.Run("interface is a composite type", func(t *testing.T) {
		def, ok := schema.LookupType("Node")
		require.True(t, ok, "interfaces must be present in the type map")
		assert.Equal(t, TypeKindInterface, def.Kind)
		assert.ElementsMatch(t, []string{"User", "Post"}, def.PossibleTypes)
	})

	t.Run("union is a composite type", func(t *testing.T) {
		def, ok := schema.LookupType("SearchResult")
		require.True(t, ok, "unions must be present in the type map")
		assert.Equal(t, TypeKindUnion, def.Kind)
		assert.ElementsMatch(t, []string{"User", "Post"}, def.PossibleTypes)
		assert.Empty(t, def.Fields, "a union declares no fields of its own")
	})

	t.Run("interface selection uses the interface's own fields", func(t *testing.T) {
		selection := schema.BuildSelectionSetForTypeName("Node", DefaultSelectionOptions())
		require.NotEmpty(t, selection)
		assert.Contains(t, selection, "id")
		assertValidDocument(t, oracle, "query Q { node(id: \"1\") "+selection+" }")
	})

	t.Run("union selection uses inline fragments", func(t *testing.T) {
		selection := schema.BuildSelectionSetForTypeName("SearchResult", DefaultSelectionOptions())
		require.NotEmpty(t, selection)
		assert.Contains(t, selection, "__typename")
		assert.Contains(t, selection, "... on User")
		assert.Contains(t, selection, "... on Post")
		assertValidDocument(t, oracle, "query Q { search(term: \"x\") "+selection+" }")
	})
}

// A nested field with a required argument cannot be selected bare. Doing so is a
// validation error against the whole document, not just that field, so the
// argument is supplied inline.
func TestSelectionSetSuppliesRequiredFieldArguments(t *testing.T) {
	const sdl = `
	enum Order { ASC DESC }
	input Paging { limit: Int!, cursor: String }
	type Post { id: ID!, title: String }
	type User {
	  id: ID!
	  posts(first: Int!, order: Order!, paging: Paging!): [Post!]!
	  optional(after: String): [Post!]!
	}
	type Query { me: User }
	`

	schema, oracle := parseSDL(t, sdl)
	selection := schema.BuildSelectionSetForTypeName("User", DefaultSelectionOptions())

	assert.Contains(t, selection, "first: 1")
	assert.Contains(t, selection, "order: ASC", "enum arguments are written as bare names, not quoted")
	assert.Contains(t, selection, "paging: {limit: 1}", "only required input fields are supplied")
	assert.NotContains(t, selection, "after:", "optional arguments are left out")

	assertValidDocument(t, oracle, "query Q { me "+selection+" }")
}

// A field whose required argument cannot be satisfied is dropped. Emitting it
// anyway would invalidate every other field selected alongside it.
func TestSelectionSetDropsUnsatisfiableFields(t *testing.T) {
	const sdl = `
	input SelfRef { child: SelfRef!, name: String }
	type Thing { id: ID!, unreachable(filter: SelfRef!): String }
	type Query { thing: Thing }
	`

	schema, oracle := parseSDL(t, sdl)
	selection := schema.BuildSelectionSetForTypeName("Thing", DefaultSelectionOptions())

	assert.Contains(t, selection, "id")
	assert.NotContains(t, selection, "unreachable")
	assertValidDocument(t, oracle, "query Q { thing "+selection+" }")
}

func TestSelectionSetRespectsDepth(t *testing.T) {
	const sdl = `
	type D { value: String }
	type C { d: D }
	type B { c: C }
	type A { b: B }
	type Query { a: A }
	`

	schema, _ := parseSDL(t, sdl)

	// Each level of nesting below the root costs one unit of depth, so the
	// deepest field selected moves one step further down per increment.
	for depth, wantDeepest := range map[int]string{1: "b", 2: "c", 3: "value"} {
		selection := schema.BuildSelectionSetForTypeName("A", SelectionOptions{MaxDepth: depth})
		assert.Contains(t, selection, wantDeepest, "depth %d should reach %q", depth, wantDeepest)
	}

	shallow := schema.BuildSelectionSetForTypeName("A", SelectionOptions{MaxDepth: 1})
	assert.NotContains(t, shallow, "c", "depth 1 must not reach the third level")
}

// Selection sets fan out multiplicatively, so a wide schema has to be bounded or
// the generated document grows past what a server will accept.
func TestSelectionSetBoundsFieldsPerLevel(t *testing.T) {
	var fields strings.Builder
	for i := 0; i < 200; i++ {
		fields.WriteString("  f")
		fields.WriteString(string(rune('a' + i%26)))
		fields.WriteString(string(rune('a' + i/26)))
		fields.WriteString(": String\n")
	}
	sdl := "type Wide {\n" + fields.String() + "}\ntype Query { wide: Wide }"

	schema, oracle := parseSDL(t, sdl)
	selection := schema.BuildSelectionSetForTypeName("Wide", SelectionOptions{MaxFieldsPerLevel: 5})

	assert.Equal(t, 5, strings.Count(selection, "\n    "), "expected exactly the configured number of fields")
	assertValidDocument(t, oracle, "query Q { wide "+selection+" }")
}

// A type that reaches itself must terminate on the depth bound rather than
// recursing until the stack is exhausted.
func TestSelectionSetTerminatesOnRecursiveTypes(t *testing.T) {
	const sdl = `
	type Node { id: ID!, parent: Node, children: [Node!]! }
	type Query { root: Node }
	`

	schema, oracle := parseSDL(t, sdl)

	selection := schema.BuildSelectionSetForTypeName("Node", DefaultSelectionOptions())
	require.NotEmpty(t, selection)
	assertValidDocument(t, oracle, "query Q { root "+selection+" }")
}

// A type with nothing selectable still has to produce a valid selection set,
// because __typename is answerable on every composite type.
func TestSelectionSetFallsBackToTypename(t *testing.T) {
	const sdl = `
	input SelfRef { child: SelfRef! }
	type Empty { unreachable(filter: SelfRef!): String }
	type Query { empty: Empty }
	`

	schema, oracle := parseSDL(t, sdl)
	selection := schema.BuildSelectionSetForTypeName("Empty", DefaultSelectionOptions())

	assert.Contains(t, selection, "__typename")
	assert.NotContains(t, selection, "unreachable")
	assertValidDocument(t, oracle, "query Q { empty "+selection+" }")
}

func TestSelectionSetEmptyForLeafTypes(t *testing.T) {
	const sdl = `
	scalar JSON
	enum Role { ADMIN USER }
	type Query { blob: JSON, role: Role, name: String, count: Int }
	`

	schema, _ := parseSDL(t, sdl)

	for _, leaf := range []string{"JSON", "Role", "String", "Int", "Float", "Boolean", "ID", "[String!]!"} {
		assert.Empty(t, schema.BuildSelectionSetForTypeName(leaf, DefaultSelectionOptions()),
			"%s takes no selection set", leaf)
	}
}

func TestIsLeafType(t *testing.T) {
	const sdl = `
	scalar DateTime
	enum Role { ADMIN }
	type User { id: ID! }
	interface Node { id: ID! }
	union Any = User
	type Query { me: User }
	`

	schema, _ := parseSDL(t, sdl)

	for _, name := range []string{"String", "Int", "Float", "Boolean", "ID", "DateTime", "Role"} {
		assert.True(t, schema.IsLeafType(name), "%s is a leaf", name)
	}
	for _, name := range []string{"User", "Node", "Any", "Query", "Unknown"} {
		assert.False(t, schema.IsLeafType(name), "%s is not a leaf", name)
	}
}

func TestStripTypeModifiers(t *testing.T) {
	for signature, want := range map[string]string{
		"String":     "String",
		"String!":    "String",
		"[String]":   "String",
		"[String!]!": "String",
		"[[Int!]!]!": "Int",
		"":           "",
	} {
		assert.Equal(t, want, StripTypeModifiers(signature), "signature %q", signature)
	}
}

func TestDefaultLiteral(t *testing.T) {
	const sdl = `
	enum Status { ACTIVE ARCHIVED }
	enum AllDeprecated { OLD @deprecated(reason: "gone") }
	input Nested { id: ID! }
	input Filter { term: String!, nested: Nested!, optional: Int }
	type Query { search(filter: Filter!): String }
	`

	schema, _ := parseSDL(t, sdl)

	tests := []struct {
		name string
		ref  TypeRef
		want string
	}{
		{"Int", named("Int", TypeKindScalar), "1"},
		{"Float", named("Float", TypeKindScalar), "1.0"},
		{"Boolean", named("Boolean", TypeKindScalar), "true"},
		{"String", named("String", TypeKindScalar), `"1"`},
		{"non-null String", nonNull(named("String", TypeKindScalar)), `"1"`},
		{"list of Int", list(named("Int", TypeKindScalar)), "[1]"},
		{"enum picks a live value", named("Status", TypeKindEnum), "ACTIVE"},
		{"enum falls back to a deprecated value", named("AllDeprecated", TypeKindEnum), "OLD"},
		{"input object supplies only required fields", named("Filter", TypeKindInputObject), `{term: "1", nested: {id: "1"}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			literal, ok := schema.DefaultLiteral(tt.ref)
			require.True(t, ok)
			assert.Equal(t, tt.want, literal)
		})
	}
}

func TestDefaultLiteralRefusesMalformedTypes(t *testing.T) {
	schema := &GraphQLSchema{Types: map[string]TypeDef{}, Enums: map[string]EnumDef{}, InputTypes: map[string]InputTypeDef{}}

	unrenderable := []TypeRef{
		{Kind: TypeKindNonNull},                              // wrapper with no inner type
		{Kind: TypeKindList},                                 // wrapper with no inner type
		{Kind: TypeKindScalar, Name: ""},                     // nameless
		{Kind: TypeKindScalar, Name: "not a valid name"},     // unusable as a literal
		{Kind: TypeKindScalar, Name: "evil\") { hacked } #"}, // injection attempt
	}

	for _, ref := range unrenderable {
		_, ok := schema.DefaultLiteral(ref)
		assert.False(t, ok, "%+v should not render", ref)
	}
}

// Names come from an endpoint the scanner does not control. One that is not a
// legal GraphQL name must never be written into a document.
func TestSelectionSetSkipsInvalidNames(t *testing.T) {
	schema := &GraphQLSchema{
		Types: map[string]TypeDef{
			"Thing": {
				Name: "Thing",
				Kind: TypeKindObject,
				Fields: []Field{
					{Name: "ok", Type: named("String", TypeKindScalar)},
					{Name: "id } evil { x", Type: named("String", TypeKindScalar)},
					{Name: "", Type: named("String", TypeKindScalar)},
				},
			},
		},
		Enums:      map[string]EnumDef{},
		InputTypes: map[string]InputTypeDef{},
	}

	selection := schema.BuildSelectionSetForTypeName("Thing", DefaultSelectionOptions())

	assert.Contains(t, selection, "ok")
	assert.NotContains(t, selection, "evil")
}

func TestSelectionSetOnNilSchema(t *testing.T) {
	var schema *GraphQLSchema

	assert.NotPanics(t, func() {
		assert.Empty(t, schema.BuildSelectionSetForTypeName("User", DefaultSelectionOptions()))
		assert.Empty(t, schema.BuildSelectionSet(named("User", TypeKindObject), DefaultSelectionOptions()))
		assert.True(t, schema.IsLeafType("String"))
		assert.False(t, schema.IsLeafType("User"))
	})
}

func named(name string, kind TypeKind) TypeRef {
	return TypeRef{Name: name, Kind: kind}
}

func nonNull(inner TypeRef) TypeRef {
	return TypeRef{Kind: TypeKindNonNull, OfType: &inner, Required: true, IsList: inner.IsList}
}

func list(inner TypeRef) TypeRef {
	return TypeRef{Kind: TypeKindList, OfType: &inner, IsList: true}
}
