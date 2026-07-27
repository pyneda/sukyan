package graphql

// GraphQLSchema represents a parsed GraphQL schema with all its operations and types
type GraphQLSchema struct {
	Queries       []Operation             `json:"queries"`
	Mutations     []Operation             `json:"mutations"`
	Subscriptions []Operation             `json:"subscriptions"`
	Types         map[string]TypeDef      `json:"types"`
	Enums         map[string]EnumDef      `json:"enums"`
	InputTypes    map[string]InputTypeDef `json:"input_types"`
	Scalars       []string                `json:"scalars"`
	Directives    []DirectiveDef          `json:"directives"`
}

// Operation represents a GraphQL query, mutation, or subscription
type Operation struct {
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Arguments    []Argument         `json:"arguments"`
	ReturnType   TypeRef            `json:"return_type"`
	IsDeprecated bool               `json:"is_deprecated,omitempty"`
	Deprecation  string             `json:"deprecation_reason,omitempty"`
	Requests     []RequestVariation `json:"requests,omitempty"`
}

// Argument represents a GraphQL field argument
type Argument struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Type        TypeRef `json:"type"`
	// DefaultValue is the default decoded into a Go value ready to be sent as a
	// JSON variable. Introspection reports defaults as GraphQL literals, so
	// `first: Int = 10` arrives as the text "10" and must be decoded before use:
	// sent verbatim the server sees the string "10" for an Int and rejects it.
	DefaultValue interface{} `json:"default_value,omitempty"`
	// DefaultLiteral is that same default in its original GraphQL literal form,
	// which is what has to be written when the value appears inline in a document
	// rather than in the variables map.
	DefaultLiteral string `json:"default_literal,omitempty"`
}

// TypeRef represents a reference to a GraphQL type with modifiers
type TypeRef struct {
	Name   string   `json:"name"`
	Kind   TypeKind `json:"kind"`
	OfType *TypeRef `json:"of_type,omitempty"` // For NON_NULL and LIST wrappers
	// Required reports whether the outermost wrapper is NON_NULL, which is what
	// decides whether a value has to be supplied. Inner non-nullability
	// ([String!] vs [String]) constrains the elements, not the argument, and is
	// only visible through Signature.
	Required bool `json:"required"`
	IsList   bool `json:"is_list"` // True if List at any level
}

// Signature renders the type exactly as it appears in the schema, e.g.
// "[String!]!" or "PostInput!". Variable declarations must reuse the schema's own
// name: a reconstructed one ("JSONObject" for any object, "<arg>Enum" for any
// enum) is rejected by the server as an unknown type before any resolver runs.
// A wrapper with no inner type is malformed and yields the empty string, which
// callers treat as "not renderable" rather than emitting a broken signature.
func (t TypeRef) Signature() string {
	switch t.Kind {
	case TypeKindNonNull:
		if t.OfType == nil {
			return ""
		}
		inner := t.OfType.Signature()
		if inner == "" {
			return ""
		}
		return inner + "!"
	case TypeKindList:
		if t.OfType == nil {
			return ""
		}
		inner := t.OfType.Signature()
		if inner == "" {
			return ""
		}
		return "[" + inner + "]"
	default:
		return t.Name
	}
}

// BaseKind returns the kind of the underlying named type, unwrapping NON_NULL and
// LIST wrappers. Kind alone reports the outermost wrapper, so a required input
// object reads as NON_NULL rather than INPUT_OBJECT.
func (t TypeRef) BaseKind() TypeKind {
	if t.OfType != nil {
		return t.OfType.BaseKind()
	}
	return t.Kind
}

// TypeKind represents the kind of GraphQL type
type TypeKind string

const (
	TypeKindScalar      TypeKind = "SCALAR"
	TypeKindObject      TypeKind = "OBJECT"
	TypeKindInterface   TypeKind = "INTERFACE"
	TypeKindUnion       TypeKind = "UNION"
	TypeKindEnum        TypeKind = "ENUM"
	TypeKindInputObject TypeKind = "INPUT_OBJECT"
	TypeKindList        TypeKind = "LIST"
	TypeKindNonNull     TypeKind = "NON_NULL"
)

// TypeDef represents a GraphQL composite type: an object, an interface or a
// union. Interfaces and unions live here alongside objects because a field
// returning one still needs a selection set, and a type missing from this map is
// indistinguishable from a scalar to everything that builds queries.
type TypeDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Kind        TypeKind `json:"kind"`
	Fields      []Field  `json:"fields"`
	Interfaces  []string `json:"interfaces,omitempty"`
	// PossibleTypes lists the concrete object types an interface or union
	// resolves to. Unions declare no fields of their own, so these names are the
	// only way to write a selection set for one.
	PossibleTypes []string `json:"possible_types,omitempty"`
}

// Field represents a field within a GraphQL type
type Field struct {
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	Arguments    []Argument `json:"arguments,omitempty"`
	Type         TypeRef    `json:"type"`
	IsDeprecated bool       `json:"is_deprecated,omitempty"`
	Deprecation  string     `json:"deprecation_reason,omitempty"`
}

// InputTypeDef represents a GraphQL input object type
type InputTypeDef struct {
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Fields      []InputField `json:"fields"`
}

// InputField represents a field within an input type
type InputField struct {
	Name         string      `json:"name"`
	Description  string      `json:"description,omitempty"`
	Type         TypeRef     `json:"type"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	// DefaultLiteral holds the default in GraphQL literal form. See Argument.
	DefaultLiteral string `json:"default_literal,omitempty"`
}

// EnumDef represents a GraphQL enum type
type EnumDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Values      []EnumValue `json:"values"`
}

// EnumValue represents a single enum value
type EnumValue struct {
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	IsDeprecated bool   `json:"is_deprecated,omitempty"`
	Deprecation  string `json:"deprecation_reason,omitempty"`
}

// DirectiveDef represents a GraphQL directive
type DirectiveDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Locations   []string   `json:"locations"`
	Arguments   []Argument `json:"arguments,omitempty"`
}

// RequestVariation represents a generated GraphQL request
type RequestVariation struct {
	Label         string                 `json:"label"`
	Query         string                 `json:"query"` // The GraphQL query string
	Variables     map[string]interface{} `json:"variables,omitempty"`
	OperationName string                 `json:"operation_name,omitempty"`
	Headers       map[string]string      `json:"headers,omitempty"`
	Description   string                 `json:"description,omitempty"`
}

// GenerationConfig controls how requests are generated
type GenerationConfig struct {
	BaseURL               string            `json:"base_url"`
	IncludeOptionalParams bool              `json:"include_optional_params"`
	FuzzingEnabled        bool              `json:"fuzzing_enabled"`
	Headers               map[string]string `json:"headers,omitempty"`
	MaxDepth              int               `json:"max_depth"`      // Max nesting depth for selection sets
	MaxListItems          int               `json:"max_list_items"` // Max items to generate for list types
	// MaxFieldsPerSelection caps how many fields are selected at each level, and
	// MaxTotalFields caps them across the whole document. Both are needed:
	// selections fan out multiplicatively with depth, so a per-level cap alone
	// still allows documents large enough to be refused outright.
	MaxFieldsPerSelection int `json:"max_fields_per_selection"`
	MaxTotalFields        int `json:"max_total_fields"`
	// MaxRequestsPerOperation caps the fuzzing variations kept per operation.
	// Zero means unlimited.
	MaxRequestsPerOperation int `json:"max_requests_per_operation"`
}

// DefaultGenerationConfig returns sensible defaults
func DefaultGenerationConfig() GenerationConfig {
	return GenerationConfig{
		BaseURL:                 "http://localhost",
		IncludeOptionalParams:   true,
		FuzzingEnabled:          false,
		MaxDepth:                3,
		MaxListItems:            2,
		MaxFieldsPerSelection:   defaultMaxFieldsPerSelection,
		MaxTotalFields:          defaultMaxTotalFields,
		MaxRequestsPerOperation: defaultMaxRequestsPerOperation,
		Headers:                 make(map[string]string),
	}
}

// SelectionOptions bounds the selection set built for a composite return type.
type SelectionOptions struct {
	MaxDepth          int
	MaxFieldsPerLevel int
	// MaxTotalFields bounds the selections in the whole document. Depth and
	// per-level caps multiply rather than add, so they do not bound the result on
	// their own: thirty fields over three levels is already tens of thousands of
	// selections and a megabyte of query.
	MaxTotalFields    int
	IncludeDeprecated bool
}

const (
	defaultMaxSelectionDepth       = 3
	defaultMaxFieldsPerSelection   = 30
	defaultMaxTotalFields          = 200
	defaultMaxRequestsPerOperation = 100
)

// DefaultSelectionOptions returns the bounds used when a caller supplies none.
func DefaultSelectionOptions() SelectionOptions {
	return SelectionOptions{
		MaxDepth:          defaultMaxSelectionDepth,
		MaxFieldsPerLevel: defaultMaxFieldsPerSelection,
		MaxTotalFields:    defaultMaxTotalFields,
		IncludeDeprecated: true,
	}
}

func (o SelectionOptions) withDefaults() SelectionOptions {
	if o.MaxDepth <= 0 {
		o.MaxDepth = defaultMaxSelectionDepth
	}
	if o.MaxFieldsPerLevel <= 0 {
		o.MaxFieldsPerLevel = defaultMaxFieldsPerSelection
	}
	if o.MaxTotalFields <= 0 {
		o.MaxTotalFields = defaultMaxTotalFields
	}
	return o
}

// ParseResult contains the full result of parsing a GraphQL endpoint
type ParseResult struct {
	Schema    *GraphQLSchema      `json:"schema"`
	Endpoints []OperationEndpoint `json:"endpoints"`
	BaseURL   string              `json:"base_url"`
	Count     int                 `json:"count"`
}

// OperationEndpoint represents a single GraphQL operation ready for testing
type OperationEndpoint struct {
	OperationType string             `json:"operation_type"` // query, mutation, subscription
	Name          string             `json:"name"`
	Description   string             `json:"description,omitempty"`
	Arguments     []ArgumentMetadata `json:"arguments"`
	ReturnType    string             `json:"return_type"`
	Requests      []RequestVariation `json:"requests"`
}

// ArgumentMetadata provides detailed information about an argument for DAST scanning
type ArgumentMetadata struct {
	Name          string             `json:"name"`
	TypeName      string             `json:"type_name"` // The base type name (e.g., "String", "Int", "UserInput")
	FullType      string             `json:"full_type"` // The full type signature (e.g., "[String!]!")
	Required      bool               `json:"required"`
	IsList        bool               `json:"is_list"`
	IsInputObject bool               `json:"is_input_object"`
	DefaultValue  interface{}        `json:"default_value,omitempty"`
	Description   string             `json:"description,omitempty"`
	NestedFields  []ArgumentMetadata `json:"nested_fields,omitempty"` // For input object types
}
