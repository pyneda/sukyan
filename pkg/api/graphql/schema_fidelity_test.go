package graphql

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A schema exercising the shapes that a DataType-based type reconstruction gets
// wrong: a custom scalar argument, a list argument, an enum argument, an input
// object argument, and fields returning custom scalars.
const fidelitySchemaJSON = `{"data":{"__schema":{
  "queryType":{"name":"Query"},
  "mutationType":{"name":"Mutation"},
  "subscriptionType":null,
  "types":[
    {"kind":"OBJECT","name":"Query","fields":[
      {"name":"serverTime","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"DateTime","ofType":null}}},
      {"name":"debugDump","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"JSON","ofType":null}}},
      {"name":"advancedSearch","args":[
        {"name":"filter","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"JSON","ofType":null}}}
      ],"type":{"kind":"OBJECT","name":"Post","ofType":null}}
    ],"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null},
    {"kind":"OBJECT","name":"Mutation","fields":[
      {"name":"createProduct","args":[
        {"name":"tags","type":{"kind":"LIST","name":null,"ofType":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}}}},
        {"name":"visibility","type":{"kind":"ENUM","name":"Visibility","ofType":null}},
        {"name":"input","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"INPUT_OBJECT","name":"ProductInput","ofType":null}}}
      ],"type":{"kind":"OBJECT","name":"Post","ofType":null}}
    ],"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null},
    {"kind":"OBJECT","name":"Post","fields":[
      {"name":"id","args":[],"type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"ID","ofType":null}}},
      {"name":"title","args":[],"type":{"kind":"SCALAR","name":"String","ofType":null}}
    ],"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null},
    {"kind":"INPUT_OBJECT","name":"ProductInput","fields":null,"inputFields":[
      {"name":"name","type":{"kind":"NON_NULL","name":null,"ofType":{"kind":"SCALAR","name":"String","ofType":null}}},
      {"name":"price","type":{"kind":"SCALAR","name":"Float","ofType":null}}
    ],"interfaces":[],"enumValues":null,"possibleTypes":null},
    {"kind":"ENUM","name":"Visibility","fields":null,"inputFields":null,"interfaces":[],"enumValues":[
      {"name":"PUBLIC"},{"name":"PRIVATE"}
    ],"possibleTypes":null},
    {"kind":"SCALAR","name":"JSON","fields":null,"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null},
    {"kind":"SCALAR","name":"DateTime","fields":null,"inputFields":null,"interfaces":[],"enumValues":null,"possibleTypes":null}
  ],
  "directives":[]
}}}`

func parseFidelitySchema(t *testing.T) ([]core.Operation, *pkgGraphql.GraphQLSchema) {
	t.Helper()
	def := &db.APIDefinition{
		Type:          db.APIDefinitionTypeGraphQL,
		SourceURL:     "https://example.com/graphql",
		BaseURL:       "https://example.com",
		RawDefinition: []byte(fidelitySchemaJSON),
	}
	ops, schema, err := NewParser().Parse(def)
	require.NoError(t, err)
	return ops, schema
}

func findOp(t *testing.T, ops []core.Operation, name string) core.Operation {
	t.Helper()
	for i := range ops {
		if ops[i].Name == name {
			return ops[i]
		}
	}
	t.Fatalf("operation %q not found", name)
	return core.Operation{}
}

func buildQueryFor(t *testing.T, op core.Operation, schema *pkgGraphql.GraphQLSchema) string {
	t.Helper()
	b := NewRequestBuilder().WithSchema(schema)
	values := map[string]any{}
	for _, p := range op.Parameters {
		values[p.Name] = p.GetEffectiveValue()
	}
	req, err := b.Build(t.Context(), op, values)
	require.NoError(t, err)

	buf := make([]byte, req.ContentLength)
	n, _ := req.Body.Read(buf)
	var payload struct {
		Query string `json:"query"`
	}
	require.NoError(t, json.Unmarshal(buf[:n], &payload))
	return payload.Query
}

// Variable declarations must reuse the schema's own type names. Reconstructing
// them from DataType yields "JSONObject" and "<arg>Enum", which no schema
// defines, so the server rejects the operation before the resolver runs.
func TestBuildQuery_DeclaresSchemaTypeNames(t *testing.T) {
	ops, schema := parseFidelitySchema(t)

	query := buildQueryFor(t, findOp(t, ops, "createProduct"), schema)

	assert.Contains(t, query, "$input: ProductInput!")
	assert.Contains(t, query, "$tags: [String!]")
	assert.Contains(t, query, "$visibility: Visibility")
	assert.NotContains(t, query, "JSONObject")
	assert.NotContains(t, query, "Enum")

	search := buildQueryFor(t, findOp(t, ops, "advancedSearch"), schema)
	assert.Contains(t, search, "$filter: JSON!")
}

// A field whose return type is a leaf (built-in or custom scalar, enum) must
// carry no selection set; appending `{ __typename }` is a validation error.
func TestBuildQuery_NoSelectionSetForLeafReturnTypes(t *testing.T) {
	ops, schema := parseFidelitySchema(t)

	for _, name := range []string{"serverTime", "debugDump"} {
		query := buildQueryFor(t, findOp(t, ops, name), schema)
		assert.NotContains(t, query, "__typename", "%s returns a custom scalar and takes no selection set: %s", name, query)
		assert.Equal(t, 1, strings.Count(query, "{"), "only the operation body brace expected in %s", query)
	}
}

func TestBuildQuery_KeepsSelectionSetForObjectReturnTypes(t *testing.T) {
	ops, schema := parseFidelitySchema(t)

	query := buildQueryFor(t, findOp(t, ops, "advancedSearch"), schema)
	assert.Contains(t, query, "id")
	assert.Contains(t, query, "title")
}

// Nested input-object fields are the injection points for mass assignment and
// injection through mutation inputs; a NON_NULL wrapper must not hide them.
func TestParse_NonNullInputObjectExposesNestedFields(t *testing.T) {
	ops, _ := parseFidelitySchema(t)

	op := findOp(t, ops, "createProduct")
	var input *core.Parameter
	for i := range op.Parameters {
		if op.Parameters[i].Name == "input" {
			input = &op.Parameters[i]
		}
	}
	require.NotNil(t, input)
	assert.Equal(t, "ProductInput!", input.TypeSignature)
	require.NotEmpty(t, input.NestedParams)

	names := make([]string, 0, len(input.NestedParams))
	for _, p := range input.NestedParams {
		names = append(names, p.Name)
	}
	assert.ElementsMatch(t, []string{"name", "price"}, names)
}

func TestTypeRefSignature(t *testing.T) {
	tests := []struct {
		name string
		ref  pkgGraphql.TypeRef
		want string
	}{
		{
			name: "plain scalar",
			ref:  pkgGraphql.TypeRef{Kind: pkgGraphql.TypeKindScalar, Name: "String"},
			want: "String",
		},
		{
			name: "non null scalar",
			ref: pkgGraphql.TypeRef{Kind: pkgGraphql.TypeKindNonNull, OfType: &pkgGraphql.TypeRef{
				Kind: pkgGraphql.TypeKindScalar, Name: "ID",
			}},
			want: "ID!",
		},
		{
			name: "list of non null strings",
			ref: pkgGraphql.TypeRef{Kind: pkgGraphql.TypeKindList, OfType: &pkgGraphql.TypeRef{
				Kind: pkgGraphql.TypeKindNonNull, OfType: &pkgGraphql.TypeRef{
					Kind: pkgGraphql.TypeKindScalar, Name: "String",
				},
			}},
			want: "[String!]",
		},
		{
			name: "non null list of non null input objects",
			ref: pkgGraphql.TypeRef{Kind: pkgGraphql.TypeKindNonNull, OfType: &pkgGraphql.TypeRef{
				Kind: pkgGraphql.TypeKindList, OfType: &pkgGraphql.TypeRef{
					Kind: pkgGraphql.TypeKindNonNull, OfType: &pkgGraphql.TypeRef{
						Kind: pkgGraphql.TypeKindInputObject, Name: "ItemInput",
					},
				},
			}},
			want: "[ItemInput!]!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.ref.Signature())
		})
	}
}

func TestTypeRefBaseKind(t *testing.T) {
	ref := pkgGraphql.TypeRef{Kind: pkgGraphql.TypeKindNonNull, OfType: &pkgGraphql.TypeRef{
		Kind: pkgGraphql.TypeKindInputObject, Name: "UserInput",
	}}
	assert.Equal(t, pkgGraphql.TypeKindInputObject, ref.BaseKind())
}
