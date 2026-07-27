package graphql

import "strings"

// Selection sets are the difference between a request that reaches a resolver and
// one the server throws away during validation. A field returning a composite
// type must carry a selection set, a field with a required argument must be given
// one, and a union has no fields of its own to select. Getting any of those wrong
// invalidates the whole document, so every caller shares the builder here rather
// than keeping its own.

const maxInputLiteralDepth = 4

// LookupType returns the composite type definition for a name.
func (s *GraphQLSchema) LookupType(name string) (TypeDef, bool) {
	if s == nil {
		return TypeDef{}, false
	}
	def, ok := s.Types[name]
	return def, ok
}

// IsLeafType reports whether a named type is selected without a selection set.
func (s *GraphQLSchema) IsLeafType(name string) bool {
	switch name {
	case "String", "Int", "Float", "Boolean", "ID":
		return true
	}
	if s == nil {
		return false
	}
	if _, ok := s.Enums[name]; ok {
		return true
	}
	for _, scalar := range s.Scalars {
		if scalar == name {
			return true
		}
	}
	return false
}

// isComposite reports whether a type reference needs a selection set. The kind
// carried by the reference is authoritative; the type map is the fallback for
// schemas deserialized before kinds were recorded.
func (s *GraphQLSchema) isComposite(ref TypeRef) bool {
	switch ref.BaseKind() {
	case TypeKindObject, TypeKindInterface, TypeKindUnion:
		return true
	case TypeKindScalar, TypeKindEnum, TypeKindInputObject:
		return false
	}

	name := getBaseTypeNameFromRef(ref)
	if s.IsLeafType(name) {
		return false
	}
	_, ok := s.LookupType(name)
	return ok
}

// BuildSelectionSet renders the selection set for a type reference, or the empty
// string when the type is a leaf and must be selected without one.
func (s *GraphQLSchema) BuildSelectionSet(ref TypeRef, opts SelectionOptions) string {
	if !s.isComposite(ref) {
		return ""
	}
	return s.newSelectionBuilder(opts).set(getBaseTypeNameFromRef(ref), 0, 1)
}

// BuildSelectionSetForTypeName is BuildSelectionSet for callers that only carry
// the rendered type signature, such as "[User!]!". It returns the empty string
// for leaf types, which must be selected without a selection set.
func (s *GraphQLSchema) BuildSelectionSetForTypeName(signature string, opts SelectionOptions) string {
	name := StripTypeModifiers(signature)
	if s.IsLeafType(name) {
		return ""
	}
	return s.newSelectionBuilder(opts).set(name, 0, 1)
}

// StripTypeModifiers reduces a type signature to its named type.
func StripTypeModifiers(signature string) string {
	return strings.Trim(signature, "[]!")
}

// selectionBuilder carries a budget of selections shared across the whole
// document. A per-level cap alone does not bound the result: selections multiply
// with depth, so thirty fields over three levels is already tens of thousands of
// them, which costs seconds to build and produces a request no server will take.
type selectionBuilder struct {
	schema    *GraphQLSchema
	opts      SelectionOptions
	remaining int
}

func (s *GraphQLSchema) newSelectionBuilder(opts SelectionOptions) *selectionBuilder {
	opts = opts.withDefaults()
	return &selectionBuilder{schema: s, opts: opts, remaining: opts.MaxTotalFields}
}

func (b *selectionBuilder) set(typeName string, depth, indent int) string {
	if depth > b.opts.MaxDepth {
		return ""
	}
	def, ok := b.schema.LookupType(typeName)
	if !ok {
		return ""
	}

	pad := strings.Repeat("  ", indent+1)
	var lines []string

	if def.Kind == TypeKindUnion {
		lines = b.unionSelections(def, depth, indent, pad)
	} else {
		lines = b.fieldSelections(def, depth, indent, pad)
	}

	// Every composite type answers __typename, so a type whose fields are all
	// unreachable or budgeted out still yields a document the server accepts.
	// The fallback is free: charging for it could leave a selection set empty,
	// which is itself a syntax error.
	if len(lines) == 0 {
		lines = []string{pad + "__typename"}
	}

	return "{\n" + strings.Join(lines, "\n") + "\n" + strings.Repeat("  ", indent) + "}"
}

func (b *selectionBuilder) exhausted(emitted int) bool {
	return b.remaining <= 0 || emitted >= b.opts.MaxFieldsPerLevel
}

func (b *selectionBuilder) unionSelections(def TypeDef, depth, indent int, pad string) []string {
	lines := []string{pad + "__typename"}

	for _, member := range def.PossibleTypes {
		if b.exhausted(len(lines)) {
			break
		}
		if !IsValidName(member) {
			continue
		}
		inner := b.set(member, depth+1, indent+1)
		if inner == "" {
			continue
		}
		b.remaining--
		lines = append(lines, pad+"... on "+member+" "+inner)
	}

	return lines
}

func (b *selectionBuilder) fieldSelections(def TypeDef, depth, indent int, pad string) []string {
	var lines []string

	for _, field := range def.Fields {
		if b.exhausted(len(lines)) {
			break
		}
		if !IsValidName(field.Name) {
			continue
		}
		if field.IsDeprecated && !b.opts.IncludeDeprecated {
			continue
		}

		args, ok := b.schema.renderRequiredArguments(field.Arguments)
		if !ok {
			continue
		}

		if !b.schema.isComposite(field.Type) {
			b.remaining--
			lines = append(lines, pad+field.Name+args)
			continue
		}

		b.remaining--
		inner := b.set(getBaseTypeNameFromRef(field.Type), depth+1, indent+1)
		if inner == "" {
			b.remaining++
			continue
		}
		lines = append(lines, pad+field.Name+args+" "+inner)
	}

	return lines
}

// renderRequiredArguments writes the arguments a nested field cannot be selected
// without. Reporting false means no valid value could be produced, in which case
// the field has to be dropped: selecting it without its arguments would invalidate
// the entire document rather than just that field.
func (s *GraphQLSchema) renderRequiredArguments(args []Argument) (string, bool) {
	var parts []string

	for _, arg := range args {
		if !arg.Type.Required {
			continue
		}
		if !IsValidName(arg.Name) {
			return "", false
		}

		if arg.DefaultLiteral != "" {
			parts = append(parts, arg.Name+": "+arg.DefaultLiteral)
			continue
		}

		literal, ok := s.DefaultLiteral(arg.Type)
		if !ok {
			return "", false
		}
		parts = append(parts, arg.Name+": "+literal)
	}

	if len(parts) == 0 {
		return "", true
	}
	return "(" + strings.Join(parts, ", ") + ")", true
}

// DefaultLiteral renders a schema-valid literal for a type, for the places a
// value must be written inline in the document instead of passed as a variable.
func (s *GraphQLSchema) DefaultLiteral(ref TypeRef) (string, bool) {
	return s.defaultLiteral(ref, 0)
}

func (s *GraphQLSchema) defaultLiteral(ref TypeRef, depth int) (string, bool) {
	if depth > maxInputLiteralDepth {
		return "", false
	}

	switch ref.Kind {
	case TypeKindNonNull:
		if ref.OfType == nil {
			return "", false
		}
		return s.defaultLiteral(*ref.OfType, depth)
	case TypeKindList:
		if ref.OfType == nil {
			return "", false
		}
		inner, ok := s.defaultLiteral(*ref.OfType, depth+1)
		if !ok {
			return "", false
		}
		return "[" + inner + "]", true
	}

	name := ref.Name
	if !IsValidName(name) {
		return "", false
	}

	if enum, ok := s.Enums[name]; ok {
		return enumLiteral(enum)
	}
	if input, ok := s.InputTypes[name]; ok {
		return s.inputObjectLiteral(input, depth)
	}
	return scalarLiteral(name), true
}

func enumLiteral(enum EnumDef) (string, bool) {
	for _, value := range enum.Values {
		if !value.IsDeprecated && IsValidName(value.Name) {
			return value.Name, true
		}
	}
	for _, value := range enum.Values {
		if IsValidName(value.Name) {
			return value.Name, true
		}
	}
	return "", false
}

func (s *GraphQLSchema) inputObjectLiteral(input InputTypeDef, depth int) (string, bool) {
	var parts []string

	for _, field := range input.Fields {
		if !field.Type.Required {
			continue
		}
		if !IsValidName(field.Name) {
			return "", false
		}
		if field.DefaultLiteral != "" {
			parts = append(parts, field.Name+": "+field.DefaultLiteral)
			continue
		}

		literal, ok := s.defaultLiteral(field.Type, depth+1)
		if !ok {
			return "", false
		}
		parts = append(parts, field.Name+": "+literal)
	}

	return "{" + strings.Join(parts, ", ") + "}", true
}

func scalarLiteral(name string) string {
	switch name {
	case "Int":
		return "1"
	case "Float":
		return "1.0"
	case "Boolean":
		return "true"
	}
	return `"1"`
}
