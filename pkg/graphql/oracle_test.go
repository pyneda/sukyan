package graphql

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

// This file is the package's validation oracle. Tests describe a schema once, as
// SDL, and this code renders the introspection response a server exposing that
// SDL would return. Everything the package produces from that response is then
// validated back against the same schema by a real GraphQL implementation, so a
// generated document that a server would reject fails the test instead of
// silently wasting a scan.

func mustLoadSDL(t *testing.T, sdl string) *ast.Schema {
	t.Helper()

	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "test.graphql", Input: sdl})
	if err != nil {
		t.Fatalf("invalid test SDL: %v", err)
	}
	return schema
}

// introspectionFromSDL renders the introspection response for an SDL document.
func introspectionFromSDL(t *testing.T, sdl string) []byte {
	t.Helper()
	return renderIntrospection(t, mustLoadSDL(t, sdl))
}

// parseSDL runs an SDL document through the full parser and returns both the
// schema the package produced and the oracle schema to validate against.
func parseSDL(t *testing.T, sdl string) (*GraphQLSchema, *ast.Schema) {
	t.Helper()

	oracle := mustLoadSDL(t, sdl)
	parsed, err := NewParser().ParseFromJSON(renderIntrospection(t, oracle))
	if err != nil {
		t.Fatalf("ParseFromJSON: %v", err)
	}
	return parsed, oracle
}

func renderIntrospection(t *testing.T, schema *ast.Schema) []byte {
	t.Helper()

	b := introspectionBuilder{schema: schema}
	result := &IntrospectionSchema{}

	if schema.Query != nil {
		result.QueryType = &TypeName{Name: schema.Query.Name}
	}
	if schema.Mutation != nil {
		result.MutationType = &TypeName{Name: schema.Mutation.Name}
	}
	if schema.Subscription != nil {
		result.SubscriptionType = &TypeName{Name: schema.Subscription.Name}
	}

	for _, name := range sortedKeys(schema.Types) {
		result.Types = append(result.Types, b.definition(schema.Types[name]))
	}
	for _, name := range sortedDirectiveKeys(schema.Directives) {
		d := schema.Directives[name]
		directive := IntrospectionDirective{Name: d.Name, Description: d.Description}
		for _, loc := range d.Locations {
			directive.Locations = append(directive.Locations, string(loc))
		}
		for _, arg := range d.Arguments {
			directive.Args = append(directive.Args, b.inputValue(arg.Name, arg.Description, arg.Type, arg.DefaultValue))
		}
		result.Directives = append(result.Directives, directive)
	}

	body, err := json.Marshal(IntrospectionResponse{Data: &IntrospectionData{Schema: result}})
	if err != nil {
		t.Fatalf("marshal introspection: %v", err)
	}
	return body
}

type introspectionBuilder struct {
	schema *ast.Schema
}

func (b introspectionBuilder) definition(def *ast.Definition) IntrospectionType {
	it := IntrospectionType{
		Kind:        string(def.Kind),
		Name:        def.Name,
		Description: def.Description,
	}

	switch def.Kind {
	case ast.Object, ast.Interface:
		for _, f := range def.Fields {
			if strings.HasPrefix(f.Name, "__") {
				continue
			}
			field := IntrospectionField{
				Name:        f.Name,
				Description: f.Description,
				Type:        b.typeRef(f.Type),
			}
			field.IsDeprecated, field.DeprecationReason = deprecation(f.Directives)
			for _, arg := range f.Arguments {
				field.Args = append(field.Args, b.inputValue(arg.Name, arg.Description, arg.Type, arg.DefaultValue))
			}
			it.Fields = append(it.Fields, field)
		}
		for _, name := range def.Interfaces {
			it.Interfaces = append(it.Interfaces, b.namedRef(name))
		}
		if def.Kind == ast.Interface {
			for _, possible := range b.schema.PossibleTypes[def.Name] {
				it.PossibleTypes = append(it.PossibleTypes, b.namedRef(possible.Name))
			}
		}

	case ast.InputObject:
		for _, f := range def.Fields {
			it.InputFields = append(it.InputFields, b.inputValue(f.Name, f.Description, f.Type, f.DefaultValue))
		}

	case ast.Enum:
		for _, v := range def.EnumValues {
			value := IntrospectionEnumValue{Name: v.Name, Description: v.Description}
			value.IsDeprecated, value.DeprecationReason = deprecation(v.Directives)
			it.EnumValues = append(it.EnumValues, value)
		}

	case ast.Union:
		for _, name := range def.Types {
			it.PossibleTypes = append(it.PossibleTypes, b.namedRef(name))
		}
	}

	return it
}

func (b introspectionBuilder) inputValue(name, description string, t *ast.Type, def *ast.Value) IntrospectionInputValue {
	iv := IntrospectionInputValue{
		Name:        name,
		Description: description,
		Type:        b.typeRef(t),
	}
	if def != nil {
		literal := def.String()
		iv.DefaultValue = &literal
	}
	return iv
}

// typeRef mirrors how a server renders a type: NON_NULL and LIST wrappers around
// a named type whose kind is looked up in the schema.
func (b introspectionBuilder) typeRef(t *ast.Type) IntrospectionTypeRef {
	if t.NonNull {
		inner := b.typeRef(&ast.Type{NamedType: t.NamedType, Elem: t.Elem})
		return IntrospectionTypeRef{Kind: "NON_NULL", OfType: &inner}
	}
	if t.Elem != nil {
		inner := b.typeRef(t.Elem)
		return IntrospectionTypeRef{Kind: "LIST", OfType: &inner}
	}
	return b.namedRef(t.NamedType)
}

func (b introspectionBuilder) namedRef(name string) IntrospectionTypeRef {
	kind := "SCALAR"
	if def, ok := b.schema.Types[name]; ok {
		kind = string(def.Kind)
	}
	return IntrospectionTypeRef{Kind: kind, Name: name}
}

func deprecation(directives ast.DirectiveList) (bool, string) {
	d := directives.ForName("deprecated")
	if d == nil {
		return false, ""
	}
	reason := "No longer supported"
	if arg := d.Arguments.ForName("reason"); arg != nil && arg.Value != nil {
		reason = arg.Value.Raw
	}
	return true, reason
}

func sortedKeys(m map[string]*ast.Definition) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedDirectiveKeys(m map[string]*ast.DirectiveDefinition) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// documentError validates a generated document against the schema and returns
// the validation error, if any. This is the check a server performs before any
// resolver runs, so a document that fails here reaches nothing.
func documentError(schema *ast.Schema, query string) error {
	_, errs := gqlparser.LoadQueryWithRules(schema, query, rules.NewDefaultRules())
	if len(errs) > 0 {
		return errs
	}
	return nil
}

func assertValidDocument(t *testing.T, schema *ast.Schema, query string) {
	t.Helper()

	if err := documentError(schema, query); err != nil {
		t.Errorf("generated document rejected by server-side validation: %v\n--- query ---\n%s", err, query)
	}
}

// assertValidVariables checks that the generated variables coerce to the types
// the document declares. Variables travel as JSON, so they are round-tripped
// through JSON first to reproduce exactly what the server receives.
func assertValidVariables(t *testing.T, schema *ast.Schema, query string, variables map[string]interface{}) {
	t.Helper()

	doc, errs := gqlparser.LoadQueryWithRules(schema, query, rules.NewDefaultRules())
	if len(errs) > 0 {
		t.Errorf("cannot validate variables, document is invalid: %v\n--- query ---\n%s", errs, query)
		return
	}
	if len(doc.Operations) == 0 {
		t.Errorf("document declares no operations:\n%s", query)
		return
	}

	if _, err := validator.VariableValues(schema, doc.Operations[0], onTheWire(t, variables)); err != nil {
		t.Errorf("generated variables rejected by server-side coercion: %v\n--- query ---\n%s\n--- variables ---\n%s",
			err, query, mustJSON(t, variables))
	}
}

// onTheWire round-trips variables through JSON so tests see the values a server
// decodes rather than the in-process Go values.
func onTheWire(t *testing.T, variables map[string]interface{}) map[string]interface{} {
	t.Helper()

	if variables == nil {
		return map[string]interface{}{}
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(mustJSON(t, variables), &decoded); err != nil {
		t.Fatalf("variables are not JSON round-trippable: %v", err)
	}
	if decoded == nil {
		decoded = map[string]interface{}{}
	}
	return decoded
}

func mustJSON(t *testing.T, v interface{}) []byte {
	t.Helper()

	body, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

// assertGeneratedRequestsAreValid runs the full pipeline over an SDL document:
// parse the introspection response, generate every request, and hold each one to
// what a server would enforce. Fuzz requests carry deliberately invalid values,
// so only their document structure is checked.
func assertGeneratedRequestsAreValid(t *testing.T, sdl string, config GenerationConfig) []OperationEndpoint {
	t.Helper()

	parsed, oracle := parseSDL(t, sdl)
	endpoints := NewGenerator(parsed, config).GenerateRequests()

	for _, endpoint := range endpoints {
		for _, req := range endpoint.Requests {
			t.Run(endpoint.OperationType+"/"+endpoint.Name+"/"+req.Label, func(t *testing.T) {
				assertValidDocument(t, oracle, req.Query)
				if req.Label == labelHappyPath {
					assertValidVariables(t, oracle, req.Query, req.Variables)
				}
			})
		}
	}

	return endpoints
}
