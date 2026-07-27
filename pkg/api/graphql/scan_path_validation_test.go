package graphql

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

// This is the path the scanner actually takes: a stored introspection document
// becomes core.Operations, and the request builder turns each one into an HTTP
// request. The checks here hold those requests to what a server enforces before
// any resolver runs, because a document rejected during validation exercises
// nothing no matter what payload it carries.

// scanPathSchema covers the shapes that decide whether a generated document is
// accepted: abstract return types, nested fields with required arguments,
// defaults of every kind, custom scalars, and a root type used as a field.
const scanPathSchema = `
scalar DateTime
scalar JSON

interface Node { id: ID! }

enum Role { ADMIN EDITOR VIEWER }

input Paging { limit: Int!, cursor: String }
input CreateUser { email: String!, role: Role! = VIEWER, meta: JSON }

type Post implements Node { id: ID!, title: String! }
type User implements Node {
  id: ID!
  email: String!
  role: Role!
  createdAt: DateTime!
  posts(first: Int!, paging: Paging!): [Post!]!
}

union SearchResult = User | Post

type Query {
  node(id: ID!): Node
  search(term: String!): [SearchResult!]!
  users(limit: Int = 10, role: Role = ADMIN, active: Boolean = true, ratio: Float = 0.5, name: String = "anon"): [User!]!
  config: JSON!
  viewer: Query
}

type Mutation {
  createUser(input: CreateUser!): User!
  deleteUser(id: ID!): Boolean!
}
`

func loadScanPathSchema(t *testing.T) ([]core.Operation, *pkgGraphql.GraphQLSchema, *ast.Schema) {
	t.Helper()

	oracle, err := gqlparser.LoadSchema(&ast.Source{Name: "scan_path.graphql", Input: scanPathSchema})
	require.NoError(t, err)

	definition := &db.APIDefinition{
		Type:          db.APIDefinitionTypeGraphQL,
		SourceURL:     "https://example.com/graphql",
		BaseURL:       "https://example.com",
		RawDefinition: renderIntrospectionJSON(t, oracle),
	}

	operations, schema, err := NewParser().Parse(definition)
	require.NoError(t, err)
	require.NotEmpty(t, operations)

	return operations, schema, oracle
}

// Every request the scanner would send for this schema has to be one the server
// will accept, both as a document and as a set of variable values.
func TestScanPathRequestsAreAcceptedByAServer(t *testing.T) {
	operations, schema, oracle := loadScanPathSchema(t)
	builder := NewRequestBuilder().WithSchema(schema)

	for _, op := range operations {
		t.Run(op.Name, func(t *testing.T) {
			values := builder.GetDefaultParamValues(op)

			req, err := builder.Build(context.Background(), op, values)
			require.NoError(t, err)

			query, variables := decodeGraphQLBody(t, req)

			doc, errs := gqlparser.LoadQueryWithRules(oracle, query, rules.NewDefaultRules())
			if len(errs) > 0 {
				t.Fatalf("scanner would send a document the server rejects: %v\n--- query ---\n%s", errs, query)
			}

			if _, err := validator.VariableValues(oracle, doc.Operations[0], variables); err != nil {
				encoded, _ := json.Marshal(variables)
				t.Fatalf("scanner would send variables the server rejects: %v\n--- query ---\n%s\n--- variables ---\n%s",
					err, query, encoded)
			}
		})
	}
}

// Injecting a payload must not turn a valid document into an invalid one, or the
// payload is discarded during validation and the finding is silently lost.
func TestScanPathRequestsStayValidWithInjectedPayloads(t *testing.T) {
	operations, schema, oracle := loadScanPathSchema(t)
	builder := NewRequestBuilder().WithSchema(schema)

	payloads := []any{
		"' OR '1'='1",
		"<script>alert(1)</script>",
		"{{7*7}}",
		"../../../etc/passwd",
		strings.Repeat("A", 512),
		"",
	}

	for _, op := range operations {
		for _, param := range op.Parameters {
			for _, payload := range payloads {
				values := builder.GetDefaultParamValues(op)

				req, err := builder.BuildWithModifiedParam(context.Background(), op, param.Name, payload, values)
				require.NoError(t, err)

				query, _ := decodeGraphQLBody(t, req)
				if errs := documentErrors(oracle, query); errs != nil {
					t.Fatalf("payload on %s.%s produced an invalid document: %v\n--- query ---\n%s",
						op.Name, param.Name, errs, query)
				}
			}
		}
	}
}

// A field returning an interface or a union needs a selection set. Without one
// the operation is rejected outright, so it would never be exercised at all.
func TestScanPathSelectsAbstractReturnTypes(t *testing.T) {
	operations, schema, _ := loadScanPathSchema(t)
	builder := NewRequestBuilder().WithSchema(schema)

	for _, name := range []string{"node", "search", "viewer"} {
		op := findOp(t, operations, name)

		req, err := builder.Build(context.Background(), op, builder.GetDefaultParamValues(op))
		require.NoError(t, err)

		query, _ := decodeGraphQLBody(t, req)
		assert.Contains(t, query, "{", "%s must carry a selection set", name)
		assert.NotRegexp(t, `\)\s*\n\}`, query, "%s was selected without a selection set", name)
	}
}

// A custom scalar is a leaf. Appending a selection set to one is a validation
// error, and custom scalars are only recognisable through the schema.
func TestScanPathLeavesCustomScalarsUnselected(t *testing.T) {
	operations, schema, oracle := loadScanPathSchema(t)
	builder := NewRequestBuilder().WithSchema(schema)

	op := findOp(t, operations, "config")
	req, err := builder.Build(context.Background(), op, builder.GetDefaultParamValues(op))
	require.NoError(t, err)

	query, _ := decodeGraphQLBody(t, req)
	assert.NotContains(t, query, "__typename")
	assert.Nil(t, documentErrors(oracle, query))
}

// Introspection reports defaults as GraphQL literals. Passed through as text a
// numeric default becomes a string in the variables map and the server rejects
// it, which is a silent loss of the operation.
func TestScanPathDecodesDefaultsToTheirRealTypes(t *testing.T) {
	operations, schema, _ := loadScanPathSchema(t)
	builder := NewRequestBuilder().WithSchema(schema)

	op := findOp(t, operations, "users")
	values := builder.GetDefaultParamValues(op)

	assert.Equal(t, int64(10), values["limit"], "an Int default must not arrive as a string")
	assert.Equal(t, 0.5, values["ratio"])
	assert.Equal(t, true, values["active"])
	assert.Equal(t, "anon", values["name"], "a String default arrives quoted and must be unquoted")
	assert.Equal(t, "ADMIN", values["role"])

	_ = schema
}

// The type map now carries interfaces, unions and root types. Anything that
// treats every entry as a plain object type has to keep working.
func TestScanPathTypeMapContents(t *testing.T) {
	_, schema, _ := loadScanPathSchema(t)

	for name, kind := range map[string]pkgGraphql.TypeKind{
		"User":         pkgGraphql.TypeKindObject,
		"Node":         pkgGraphql.TypeKindInterface,
		"SearchResult": pkgGraphql.TypeKindUnion,
		"Query":        pkgGraphql.TypeKindObject,
		"Mutation":     pkgGraphql.TypeKindObject,
	} {
		def, ok := schema.Types[name]
		require.True(t, ok, "%s must be in the type map", name)
		assert.Equal(t, kind, def.Kind)
	}

	assert.NotContains(t, schema.Types, "DateTime", "scalars are not composite types")
	assert.NotContains(t, schema.Types, "Role", "enums are not composite types")
	assert.NotContains(t, schema.Types, "Paging", "input objects are not composite types")
}

func documentErrors(schema *ast.Schema, query string) error {
	_, errs := gqlparser.LoadQueryWithRules(schema, query, rules.NewDefaultRules())
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func decodeGraphQLBody(t *testing.T, req *http.Request) (string, map[string]any) {
	t.Helper()

	require.NotNil(t, req.Body)
	raw, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	var body struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	require.NoError(t, json.Unmarshal(raw, &body))

	if body.Variables == nil {
		body.Variables = map[string]any{}
	}
	return body.Query, body.Variables
}

// renderIntrospectionJSON produces the introspection response a server exposing
// this schema would return, so the test starts from the same input the scanner
// stores rather than a hand-written fixture that can drift from the schema.
func renderIntrospectionJSON(t *testing.T, schema *ast.Schema) []byte {
	t.Helper()

	type typeRef struct {
		Kind   string   `json:"kind"`
		Name   string   `json:"name,omitempty"`
		OfType *typeRef `json:"ofType,omitempty"`
	}
	type inputValue struct {
		Name         string  `json:"name"`
		Type         typeRef `json:"type"`
		DefaultValue *string `json:"defaultValue,omitempty"`
	}
	type field struct {
		Name string       `json:"name"`
		Args []inputValue `json:"args"`
		Type typeRef      `json:"type"`
	}
	type fullType struct {
		Kind        string       `json:"kind"`
		Name        string       `json:"name"`
		Fields      []field      `json:"fields,omitempty"`
		InputFields []inputValue `json:"inputFields,omitempty"`
		Interfaces  []typeRef    `json:"interfaces,omitempty"`
		EnumValues  []struct {
			Name string `json:"name"`
		} `json:"enumValues,omitempty"`
		PossibleTypes []typeRef `json:"possibleTypes,omitempty"`
	}

	kindOf := func(name string) string {
		if def, ok := schema.Types[name]; ok {
			return string(def.Kind)
		}
		return "SCALAR"
	}

	var toRef func(t *ast.Type) typeRef
	toRef = func(t *ast.Type) typeRef {
		if t.NonNull {
			inner := toRef(&ast.Type{NamedType: t.NamedType, Elem: t.Elem})
			return typeRef{Kind: "NON_NULL", OfType: &inner}
		}
		if t.Elem != nil {
			inner := toRef(t.Elem)
			return typeRef{Kind: "LIST", OfType: &inner}
		}
		return typeRef{Kind: kindOf(t.NamedType), Name: t.NamedType}
	}

	toInput := func(name string, ty *ast.Type, def *ast.Value) inputValue {
		iv := inputValue{Name: name, Type: toRef(ty)}
		if def != nil {
			literal := def.String()
			iv.DefaultValue = &literal
		}
		return iv
	}

	names := make([]string, 0, len(schema.Types))
	for name := range schema.Types {
		names = append(names, name)
	}
	sort.Strings(names)

	var types []fullType
	for _, name := range names {
		def := schema.Types[name]
		ft := fullType{Kind: string(def.Kind), Name: def.Name}

		switch def.Kind {
		case ast.Object, ast.Interface:
			for _, f := range def.Fields {
				if strings.HasPrefix(f.Name, "__") {
					continue
				}
				entry := field{Name: f.Name, Type: toRef(f.Type), Args: []inputValue{}}
				for _, arg := range f.Arguments {
					entry.Args = append(entry.Args, toInput(arg.Name, arg.Type, arg.DefaultValue))
				}
				ft.Fields = append(ft.Fields, entry)
			}
			for _, iface := range def.Interfaces {
				ft.Interfaces = append(ft.Interfaces, typeRef{Kind: "INTERFACE", Name: iface})
			}
			if def.Kind == ast.Interface {
				for _, possible := range schema.PossibleTypes[def.Name] {
					ft.PossibleTypes = append(ft.PossibleTypes, typeRef{Kind: "OBJECT", Name: possible.Name})
				}
			}
		case ast.InputObject:
			for _, f := range def.Fields {
				ft.InputFields = append(ft.InputFields, toInput(f.Name, f.Type, f.DefaultValue))
			}
		case ast.Enum:
			for _, v := range def.EnumValues {
				ft.EnumValues = append(ft.EnumValues, struct {
					Name string `json:"name"`
				}{Name: v.Name})
			}
		case ast.Union:
			for _, member := range def.Types {
				ft.PossibleTypes = append(ft.PossibleTypes, typeRef{Kind: "OBJECT", Name: member})
			}
		}

		types = append(types, ft)
	}

	root := map[string]any{
		"data": map[string]any{
			"__schema": map[string]any{
				"queryType":    map[string]string{"name": schema.Query.Name},
				"mutationType": map[string]string{"name": schema.Mutation.Name},
				"types":        types,
			},
		},
	}

	body, err := json.Marshal(root)
	require.NoError(t, err)
	return body
}
