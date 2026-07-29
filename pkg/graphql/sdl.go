package graphql

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

// ParseSchema parses a GraphQL schema from whichever of the two shapes a schema
// is handed to us in: an introspection response (what a server answers) or an SDL
// document (what a developer pastes out of their repository).
//
// The two are told apart by the content itself rather than by any type hint the
// caller supplied, because the callers that matter — a pasted import, a stored
// raw definition read back for a scan — have no reliable hint to give. JSON is
// tried as introspection and never as SDL: "{" is not a valid SDL document, so a
// JSON body that fails introspection decoding is a broken introspection response
// and must be reported as one instead of as a syntax error a page away.
func (p *Parser) ParseSchema(data []byte) (*GraphQLSchema, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty GraphQL schema document")
	}

	if trimmed[0] == '{' || trimmed[0] == '[' {
		return p.ParseFromJSON(data)
	}

	return p.ParseFromSDL(data)
}

// ParseFromSDL parses a GraphQL schema from an SDL document.
func (p *Parser) ParseFromSDL(data []byte) (*GraphQLSchema, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("empty GraphQL schema document")
	}

	schema, err := gqlparser.LoadSchema(&ast.Source{Name: "schema.graphql", Input: string(data)})
	if err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL SDL: %w", err)
	}
	if schema == nil {
		return nil, fmt.Errorf("failed to parse GraphQL SDL: no schema produced")
	}

	// The SDL is projected onto the introspection model rather than converted
	// straight into GraphQLSchema so that both entry points share one conversion:
	// type references, default literals, deprecations and enum handling all keep
	// behaving exactly as they do for an introspected schema.
	return p.convertSchema(introspectionFromAST(schema))
}

// introspectionFromAST projects a parsed SDL schema onto the introspection model.
func introspectionFromAST(schema *ast.Schema) *IntrospectionSchema {
	result := &IntrospectionSchema{
		Types:      make([]IntrospectionType, 0, len(schema.Types)),
		Directives: make([]IntrospectionDirective, 0, len(schema.Directives)),
	}

	if schema.Query != nil {
		result.QueryType = &TypeName{Name: schema.Query.Name}
	}
	if schema.Mutation != nil {
		result.MutationType = &TypeName{Name: schema.Mutation.Name}
	}
	if schema.Subscription != nil {
		result.SubscriptionType = &TypeName{Name: schema.Subscription.Name}
	}

	// A type reference in SDL carries no kind, so the kind of every named type is
	// resolved up front. Without it every reference would have to be reported as a
	// scalar, and the generator would stop building selection sets for objects.
	kinds := make(map[string]string, len(schema.Types))
	for name, def := range schema.Types {
		kinds[name] = string(def.Kind)
	}

	// Map iteration order is random and this output ends up stored, so the types
	// and directives are emitted in a stable order.
	for _, name := range sortedTypeNames(schema.Types) {
		result.Types = append(result.Types, introspectionTypeFromAST(schema, schema.Types[name], kinds))
	}

	for _, name := range sortedDirectiveNames(schema.Directives) {
		result.Directives = append(result.Directives, introspectionDirectiveFromAST(schema.Directives[name], kinds))
	}

	return result
}

func sortedTypeNames(types map[string]*ast.Definition) []string {
	names := make([]string, 0, len(types))
	for name := range types {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedDirectiveNames(directives map[string]*ast.DirectiveDefinition) []string {
	names := make([]string, 0, len(directives))
	for name := range directives {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func introspectionTypeFromAST(schema *ast.Schema, def *ast.Definition, kinds map[string]string) IntrospectionType {
	converted := IntrospectionType{
		Kind:        string(def.Kind),
		Name:        def.Name,
		Description: def.Description,
	}

	for _, field := range def.Fields {
		// The introspection meta fields (__typename, __schema, __type) are part of
		// every type but are not part of the API's surface.
		if strings.HasPrefix(field.Name, "__") {
			continue
		}

		if def.Kind == ast.InputObject {
			converted.InputFields = append(converted.InputFields, IntrospectionInputValue{
				Name:         field.Name,
				Description:  field.Description,
				Type:         introspectionTypeRefFromAST(field.Type, kinds),
				DefaultValue: defaultLiteralFromAST(field.DefaultValue),
			})
			continue
		}

		deprecated, reason := deprecationFromAST(field.Directives)
		converted.Fields = append(converted.Fields, IntrospectionField{
			Name:              field.Name,
			Description:       field.Description,
			Args:              introspectionArgumentsFromAST(field.Arguments, kinds),
			Type:              introspectionTypeRefFromAST(field.Type, kinds),
			IsDeprecated:      deprecated,
			DeprecationReason: reason,
		})
	}

	for _, iface := range def.Interfaces {
		converted.Interfaces = append(converted.Interfaces, IntrospectionTypeRef{
			Kind: "INTERFACE",
			Name: iface,
		})
	}

	// Unions list their members in def.Types; interfaces learn theirs only from the
	// schema-wide index the loader builds. Reading the index covers both.
	for _, possible := range schema.GetPossibleTypes(def) {
		converted.PossibleTypes = append(converted.PossibleTypes, IntrospectionTypeRef{
			Kind: string(possible.Kind),
			Name: possible.Name,
		})
	}

	for _, value := range def.EnumValues {
		deprecated, reason := deprecationFromAST(value.Directives)
		converted.EnumValues = append(converted.EnumValues, IntrospectionEnumValue{
			Name:              value.Name,
			Description:       value.Description,
			IsDeprecated:      deprecated,
			DeprecationReason: reason,
		})
	}

	return converted
}

func introspectionDirectiveFromAST(def *ast.DirectiveDefinition, kinds map[string]string) IntrospectionDirective {
	locations := make([]string, 0, len(def.Locations))
	for _, location := range def.Locations {
		locations = append(locations, string(location))
	}

	return IntrospectionDirective{
		Name:        def.Name,
		Description: def.Description,
		Locations:   locations,
		Args:        introspectionArgumentsFromAST(def.Arguments, kinds),
	}
}

func introspectionArgumentsFromAST(args ast.ArgumentDefinitionList, kinds map[string]string) []IntrospectionInputValue {
	converted := make([]IntrospectionInputValue, 0, len(args))
	for _, arg := range args {
		converted = append(converted, IntrospectionInputValue{
			Name:         arg.Name,
			Description:  arg.Description,
			Type:         introspectionTypeRefFromAST(arg.Type, kinds),
			DefaultValue: defaultLiteralFromAST(arg.DefaultValue),
		})
	}
	return converted
}

// introspectionTypeRefFromAST unwraps an SDL type into the NON_NULL/LIST chain
// introspection uses. The recursion is bounded by the nesting the parser accepted.
func introspectionTypeRefFromAST(t *ast.Type, kinds map[string]string) IntrospectionTypeRef {
	if t == nil {
		return IntrospectionTypeRef{}
	}

	if t.NonNull {
		inner := &ast.Type{NamedType: t.NamedType, Elem: t.Elem}
		wrapped := introspectionTypeRefFromAST(inner, kinds)
		return IntrospectionTypeRef{Kind: "NON_NULL", OfType: &wrapped}
	}

	if t.Elem != nil {
		wrapped := introspectionTypeRefFromAST(t.Elem, kinds)
		return IntrospectionTypeRef{Kind: "LIST", OfType: &wrapped}
	}

	kind, ok := kinds[t.NamedType]
	if !ok {
		// The loader rejects references to undefined types, so this only happens for
		// a schema built by hand; a scalar is the safe reading since it stops the
		// generator from trying to build a selection set for it.
		kind = string(ast.Scalar)
	}
	return IntrospectionTypeRef{Kind: kind, Name: t.NamedType}
}

// deprecationFromAST reads the @deprecated directive introspection reports as the
// isDeprecated / deprecationReason pair.
func deprecationFromAST(directives ast.DirectiveList) (bool, string) {
	directive := directives.ForName("deprecated")
	if directive == nil {
		return false, ""
	}

	// The spec gives @deprecated's reason a default, and a server's introspection
	// reports that default rather than an empty string.
	reason := "No longer supported"
	if arg := directive.Arguments.ForName("reason"); arg != nil && arg.Value != nil {
		reason = arg.Value.Raw
	}
	return true, reason
}

// defaultLiteralFromAST renders an SDL default back into the literal text
// introspection reports, which is the form decodeDefault knows how to read.
func defaultLiteralFromAST(value *ast.Value) *string {
	if value == nil {
		return nil
	}
	literal := value.String()
	return &literal
}
