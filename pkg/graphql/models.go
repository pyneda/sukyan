package graphql

// GraphQLSchema represents a parsed GraphQL schema with all its operations and types
type GraphQLSchema struct {
	Queries       []Operation            `json:"queries"`
	Mutations     []Operation            `json:"mutations"`
	Subscriptions []Operation            `json:"subscriptions"`
	Types         map[string]TypeDef     `json:"types"`
	Enums         map[string]EnumDef     `json:"enums"`
	InputTypes    map[string]InputTypeDef `json:"input_types"`
	Scalars       []string               `json:"scalars"`
	Directives    []DirectiveDef         `json:"directives"`
}

// Operation represents a GraphQL query, mutation, or subscription
type Operation struct {
	Name         string        `json:"name"`
	Description  string        `json:"description,omitempty"`
	Arguments    []Argument    `json:"arguments"`
	ReturnType   TypeRef       `json:"return_type"`
	IsDeprecated bool          `json:"is_deprecated,omitempty"`
	Deprecation  string        `json:"deprecation_reason,omitempty"`
	Requests     []RequestVariation `json:"requests,omitempty"`
}

// Argument represents a GraphQL field argument
type Argument struct {
	Name         string      `json:"name"`
	Description  string      `json:"description,omitempty"`
	Type         TypeRef     `json:"type"`
	DefaultValue interface{} `json:"default_value,omitempty"`
}

// TypeRef represents a reference to a GraphQL type with modifiers
type TypeRef struct {
	Name     string   `json:"name"`
	Kind     TypeKind `json:"kind"`
	OfType   *TypeRef `json:"of_type,omitempty"`   // For NON_NULL and LIST wrappers
	Required bool     `json:"required"`             // True if NonNull at any level
	IsList   bool     `json:"is_list"`              // True if List at any level
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

// TypeDef represents a GraphQL object type definition
type TypeDef struct {
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	Fields      []Field `json:"fields"`
	Interfaces  []string `json:"interfaces,omitempty"`
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
	Label         string            `json:"label"`
	Query         string            `json:"query"`         // The GraphQL query string
	Variables     map[string]interface{} `json:"variables,omitempty"`
	OperationName string            `json:"operation_name,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	Description   string            `json:"description,omitempty"`
}

// GenerationConfig controls how requests are generated
type GenerationConfig struct {
	BaseURL               string            `json:"base_url"`
	IncludeOptionalParams bool              `json:"include_optional_params"`
	FuzzingEnabled        bool              `json:"fuzzing_enabled"`
	Headers               map[string]string `json:"headers,omitempty"`
	MaxDepth              int               `json:"max_depth"`              // Max nesting depth for selection sets
	MaxListItems          int               `json:"max_list_items"`         // Max items to generate for list types
}

// DefaultGenerationConfig returns sensible defaults
func DefaultGenerationConfig() GenerationConfig {
	return GenerationConfig{
		BaseURL:               "http://localhost",
		IncludeOptionalParams: true,
		FuzzingEnabled:        false,
		MaxDepth:              3,
		MaxListItems:          2,
		Headers:               make(map[string]string),
	}
}

// ParseResult contains the full result of parsing a GraphQL endpoint
type ParseResult struct {
	Schema    *GraphQLSchema       `json:"schema"`
	Endpoints []OperationEndpoint  `json:"endpoints"`
	BaseURL   string               `json:"base_url"`
	Count     int                  `json:"count"`
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
	Name         string      `json:"name"`
	TypeName     string      `json:"type_name"`     // The base type name (e.g., "String", "Int", "UserInput")
	FullType     string      `json:"full_type"`     // The full type signature (e.g., "[String!]!")
	Required     bool        `json:"required"`
	IsList       bool        `json:"is_list"`
	IsInputObject bool       `json:"is_input_object"`
	DefaultValue interface{} `json:"default_value,omitempty"`
	Description  string      `json:"description,omitempty"`
	NestedFields []ArgumentMetadata `json:"nested_fields,omitempty"` // For input object types
}
