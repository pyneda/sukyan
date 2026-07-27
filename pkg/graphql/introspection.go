package graphql

// IntrospectionQuery is the standard GraphQL introspection query
const IntrospectionQuery = `
query IntrospectionQuery {
  __schema {
    queryType { name }
    mutationType { name }
    subscriptionType { name }
    types {
      ...FullType
    }
    directives {
      name
      description
      locations
      args {
        ...InputValue
      }
    }
  }
}

fragment FullType on __Type {
  kind
  name
  description
  fields(includeDeprecated: true) {
    name
    description
    args {
      ...InputValue
    }
    type {
      ...TypeRef
    }
    isDeprecated
    deprecationReason
  }
  inputFields {
    ...InputValue
  }
  interfaces {
    ...TypeRef
  }
  enumValues(includeDeprecated: true) {
    name
    description
    isDeprecated
    deprecationReason
  }
  possibleTypes {
    ...TypeRef
  }
}

fragment InputValue on __InputValue {
  name
  description
  type {
    ...TypeRef
  }
  defaultValue
}

fragment TypeRef on __Type {
  kind
  name
  ofType {
    kind
    name
    ofType {
      kind
      name
      ofType {
        kind
        name
        ofType {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
  }
}
`

// IntrospectionResponse represents the response from an introspection query
type IntrospectionResponse struct {
	Data   *IntrospectionData `json:"data"`
	Errors []GraphQLError     `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error
type GraphQLError struct {
	Message    string                 `json:"message"`
	Locations  []ErrorLocation        `json:"locations,omitempty"`
	Path       []interface{}          `json:"path,omitempty"`
	Extensions map[string]interface{} `json:"extensions,omitempty"`
}

// ErrorLocation represents an error location in the query
type ErrorLocation struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// IntrospectionData contains the schema data
type IntrospectionData struct {
	Schema *IntrospectionSchema `json:"__schema"`
}

// IntrospectionSchema represents the introspected schema
type IntrospectionSchema struct {
	QueryType        *TypeName                `json:"queryType"`
	MutationType     *TypeName                `json:"mutationType"`
	SubscriptionType *TypeName                `json:"subscriptionType"`
	Types            []IntrospectionType      `json:"types"`
	Directives       []IntrospectionDirective `json:"directives"`
}

// TypeName is a simple wrapper for type name reference
type TypeName struct {
	Name string `json:"name"`
}

// IntrospectionType represents a type from introspection
type IntrospectionType struct {
	Kind          string                    `json:"kind"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description"`
	Fields        []IntrospectionField      `json:"fields"`
	InputFields   []IntrospectionInputValue `json:"inputFields"`
	Interfaces    []IntrospectionTypeRef    `json:"interfaces"`
	EnumValues    []IntrospectionEnumValue  `json:"enumValues"`
	PossibleTypes []IntrospectionTypeRef    `json:"possibleTypes"`
}

// IntrospectionField represents a field from introspection
type IntrospectionField struct {
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	Args              []IntrospectionInputValue `json:"args"`
	Type              IntrospectionTypeRef      `json:"type"`
	IsDeprecated      bool                      `json:"isDeprecated"`
	DeprecationReason string                    `json:"deprecationReason"`
}

// IntrospectionInputValue represents an input value (argument or input field)
type IntrospectionInputValue struct {
	Name         string               `json:"name"`
	Description  string               `json:"description"`
	Type         IntrospectionTypeRef `json:"type"`
	DefaultValue *string              `json:"defaultValue"`
}

// IntrospectionTypeRef represents a type reference with nesting
type IntrospectionTypeRef struct {
	Kind   string                `json:"kind"`
	Name   string                `json:"name"`
	OfType *IntrospectionTypeRef `json:"ofType"`
}

// IntrospectionEnumValue represents an enum value from introspection
type IntrospectionEnumValue struct {
	Name              string `json:"name"`
	Description       string `json:"description"`
	IsDeprecated      bool   `json:"isDeprecated"`
	DeprecationReason string `json:"deprecationReason"`
}

// IntrospectionDirective represents a directive from introspection
type IntrospectionDirective struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	Locations   []string                  `json:"locations"`
	Args        []IntrospectionInputValue `json:"args"`
}

// maxTypeRefDepth bounds how many wrappers a type reference may nest. The
// introspection query itself only reaches seven levels, but ParseFromJSON also
// accepts stored and third-party documents, and an arbitrarily deep chain there
// would exhaust the stack in every routine that walks a TypeRef.
const maxTypeRefDepth = 64

// convertTypeRef converts an introspection type reference to our TypeRef model
func convertTypeRef(ref IntrospectionTypeRef) TypeRef {
	return convertTypeRefAtDepth(ref, 0)
}

// A reference nested past the limit converts to the zero TypeRef, whose empty
// signature makes every caller treat it as unusable rather than emit a truncated
// type that no longer describes the argument.
func convertTypeRefAtDepth(ref IntrospectionTypeRef, depth int) TypeRef {
	if depth > maxTypeRefDepth {
		return TypeRef{}
	}

	tr := TypeRef{
		Kind: TypeKind(ref.Kind),
		Name: ref.Name,
	}

	if ref.OfType != nil {
		inner := convertTypeRefAtDepth(*ref.OfType, depth+1)
		if inner.Kind == "" && inner.Name == "" {
			return TypeRef{}
		}
		tr.OfType = &inner
	}

	tr.Required = ref.Kind == "NON_NULL"
	tr.IsList = isList(ref, 0)

	return tr
}

// isList checks if the type is a list at any level
func isList(ref IntrospectionTypeRef, depth int) bool {
	if depth > maxTypeRefDepth {
		return false
	}
	if ref.Kind == "LIST" {
		return true
	}
	if ref.OfType != nil {
		return isList(*ref.OfType, depth+1)
	}
	return false
}

// getBaseTypeName extracts the base type name from a nested type ref
func getBaseTypeName(ref IntrospectionTypeRef) string {
	for depth := 0; depth <= maxTypeRefDepth; depth++ {
		if ref.Name != "" {
			return ref.Name
		}
		if ref.OfType == nil {
			return ""
		}
		ref = *ref.OfType
	}
	return ""
}
