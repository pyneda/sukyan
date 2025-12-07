package graphql

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Generator creates GraphQL requests from parsed schemas
type Generator struct {
	schema  *GraphQLSchema
	config  GenerationConfig
	seenOps map[string]bool
}

// NewGenerator creates a new request generator
func NewGenerator(schema *GraphQLSchema, config GenerationConfig) *Generator {
	if config.MaxDepth == 0 {
		config.MaxDepth = 3
	}
	if config.MaxListItems == 0 {
		config.MaxListItems = 2
	}

	return &Generator{
		schema:  schema,
		config:  config,
		seenOps: make(map[string]bool),
	}
}

// GenerateRequests generates request variations for all operations
func (g *Generator) GenerateRequests() []OperationEndpoint {
	var endpoints []OperationEndpoint

	// Generate query endpoints
	for _, op := range g.schema.Queries {
		endpoint := g.generateOperationEndpoint("query", op)
		endpoints = append(endpoints, endpoint)
	}

	// Generate mutation endpoints
	for _, op := range g.schema.Mutations {
		endpoint := g.generateOperationEndpoint("mutation", op)
		endpoints = append(endpoints, endpoint)
	}

	// Generate subscription endpoints (for documentation, subscriptions work differently)
	for _, op := range g.schema.Subscriptions {
		endpoint := g.generateOperationEndpoint("subscription", op)
		endpoints = append(endpoints, endpoint)
	}

	return endpoints
}

// generateOperationEndpoint creates an endpoint with all request variations for an operation
func (g *Generator) generateOperationEndpoint(opType string, op Operation) OperationEndpoint {
	endpoint := OperationEndpoint{
		OperationType: opType,
		Name:          op.Name,
		Description:   op.Description,
		ReturnType:    formatTypeRef(op.ReturnType),
		Arguments:     g.buildArgumentMetadata(op.Arguments),
		Requests:      make([]RequestVariation, 0),
	}

	// Generate happy path request
	happyPath := g.generateHappyPathRequest(opType, op)
	endpoint.Requests = append(endpoint.Requests, happyPath)

	// Generate fuzzing variations if enabled
	if g.config.FuzzingEnabled {
		fuzzRequests := g.generateFuzzRequests(opType, op)
		endpoint.Requests = append(endpoint.Requests, fuzzRequests...)
	}

	// Deduplicate requests
	endpoint.Requests = g.deduplicateRequests(endpoint.Requests)

	return endpoint
}

// buildArgumentMetadata builds detailed argument metadata for DAST scanning
func (g *Generator) buildArgumentMetadata(args []Argument) []ArgumentMetadata {
	metadata := make([]ArgumentMetadata, 0, len(args))

	for _, arg := range args {
		meta := ArgumentMetadata{
			Name:          arg.Name,
			TypeName:      getBaseTypeNameFromRef(arg.Type),
			FullType:      formatTypeRef(arg.Type),
			Required:      arg.Type.Required,
			IsList:        arg.Type.IsList,
			DefaultValue:  arg.DefaultValue,
			Description:   arg.Description,
			IsInputObject: g.isInputObjectType(arg.Type),
		}

		// If it's an input object, include nested field metadata
		if meta.IsInputObject {
			meta.NestedFields = g.buildNestedFieldMetadata(arg.Type, 0)
		}

		metadata = append(metadata, meta)
	}

	return metadata
}

// buildNestedFieldMetadata recursively builds metadata for input object fields
func (g *Generator) buildNestedFieldMetadata(typeRef TypeRef, depth int) []ArgumentMetadata {
	if depth > g.config.MaxDepth {
		return nil
	}

	baseName := getBaseTypeNameFromRef(typeRef)
	inputDef, ok := g.schema.InputTypes[baseName]
	if !ok {
		return nil
	}

	metadata := make([]ArgumentMetadata, 0, len(inputDef.Fields))

	for _, field := range inputDef.Fields {
		meta := ArgumentMetadata{
			Name:          field.Name,
			TypeName:      getBaseTypeNameFromRef(field.Type),
			FullType:      formatTypeRef(field.Type),
			Required:      field.Type.Required,
			IsList:        field.Type.IsList,
			DefaultValue:  field.DefaultValue,
			Description:   field.Description,
			IsInputObject: g.isInputObjectType(field.Type),
		}

		if meta.IsInputObject {
			meta.NestedFields = g.buildNestedFieldMetadata(field.Type, depth+1)
		}

		metadata = append(metadata, meta)
	}

	return metadata
}

// isInputObjectType checks if a type reference is an input object
func (g *Generator) isInputObjectType(typeRef TypeRef) bool {
	baseName := getBaseTypeNameFromRef(typeRef)
	_, ok := g.schema.InputTypes[baseName]
	return ok
}

// generateHappyPathRequest creates a baseline working request
func (g *Generator) generateHappyPathRequest(opType string, op Operation) RequestVariation {
	strategy := NewDefaultValueStrategy(g.schema)
	variables := make(map[string]interface{})

	// Generate values for all arguments
	for _, arg := range op.Arguments {
		// Include required arguments, and optional if configured
		if arg.Type.Required || g.config.IncludeOptionalParams {
			value := g.generateArgumentValue(arg, strategy)
			if value != nil {
				variables[arg.Name] = value
			}
		}
	}

	// Build the query string
	query := g.buildQueryString(opType, op, variables)

	return RequestVariation{
		Label:         "Happy Path",
		Query:         query,
		Variables:     variables,
		OperationName: op.Name,
		Headers:       g.buildHeaders(),
		Description:   "Baseline request with default values for all parameters",
	}
}

// generateFuzzRequests creates fuzzing variations for each argument
func (g *Generator) generateFuzzRequests(opType string, op Operation) []RequestVariation {
	var requests []RequestVariation
	strategy := NewInterestingValuesStrategy(g.schema)
	defaultStrategy := NewDefaultValueStrategy(g.schema)

	// For each argument, generate fuzz variations
	for _, targetArg := range op.Arguments {
		// Get interesting values for this argument type
		interestingValues := g.getInterestingValuesForArg(targetArg, strategy)

		for _, gv := range interestingValues {
			variables := make(map[string]interface{})

			// Set default values for other arguments
			for _, arg := range op.Arguments {
				if arg.Name == targetArg.Name {
					variables[arg.Name] = gv.Value
				} else if arg.Type.Required || g.config.IncludeOptionalParams {
					value := g.generateArgumentValue(arg, defaultStrategy)
					if value != nil {
						variables[arg.Name] = value
					}
				}
			}

			query := g.buildQueryString(opType, op, variables)

			requests = append(requests, RequestVariation{
				Label:         fmt.Sprintf("Fuzz '%s': %s", targetArg.Name, gv.Description),
				Query:         query,
				Variables:     variables,
				OperationName: op.Name,
				Headers:       g.buildHeaders(),
				Description:   fmt.Sprintf("Fuzzing argument '%s' (%s) with: %s", targetArg.Name, formatTypeRef(targetArg.Type), gv.Description),
			})
		}

		// If it's an input object, also fuzz individual fields
		if g.isInputObjectType(targetArg.Type) {
			fieldFuzzRequests := g.generateInputObjectFieldFuzzRequests(opType, op, targetArg, defaultStrategy, strategy)
			requests = append(requests, fieldFuzzRequests...)
		}
	}

	return requests
}

// generateInputObjectFieldFuzzRequests generates fuzz variations for fields within an input object
func (g *Generator) generateInputObjectFieldFuzzRequests(opType string, op Operation, targetArg Argument, defaultStrategy *DefaultValueStrategy, fuzzStrategy *InterestingValuesStrategy) []RequestVariation {
	var requests []RequestVariation

	baseName := getBaseTypeNameFromRef(targetArg.Type)
	inputDef, ok := g.schema.InputTypes[baseName]
	if !ok {
		return requests
	}

	// For each field in the input object
	for _, field := range inputDef.Fields {
		interestingValues := g.getInterestingValuesForType(field.Type, fuzzStrategy)

		for _, gv := range interestingValues {
			variables := make(map[string]interface{})

			// Set default values for other arguments
			for _, arg := range op.Arguments {
				if arg.Name == targetArg.Name {
					// Build the input object with this field fuzzed
					obj := g.buildInputObjectWithFuzzedField(inputDef, field.Name, gv.Value, defaultStrategy, 0)
					variables[arg.Name] = obj
				} else if arg.Type.Required || g.config.IncludeOptionalParams {
					value := g.generateArgumentValue(arg, defaultStrategy)
					if value != nil {
						variables[arg.Name] = value
					}
				}
			}

			query := g.buildQueryString(opType, op, variables)

			requests = append(requests, RequestVariation{
				Label:         fmt.Sprintf("Fuzz '%s.%s': %s", targetArg.Name, field.Name, gv.Description),
				Query:         query,
				Variables:     variables,
				OperationName: op.Name,
				Headers:       g.buildHeaders(),
				Description:   fmt.Sprintf("Fuzzing nested field '%s.%s' with: %s", targetArg.Name, field.Name, gv.Description),
			})
		}
	}

	return requests
}

// buildInputObjectWithFuzzedField creates an input object with one field fuzzed
func (g *Generator) buildInputObjectWithFuzzedField(inputDef InputTypeDef, fuzzFieldName string, fuzzValue interface{}, strategy *DefaultValueStrategy, depth int) map[string]interface{} {
	obj := make(map[string]interface{})

	for _, field := range inputDef.Fields {
		if field.Name == fuzzFieldName {
			obj[field.Name] = fuzzValue
		} else if field.Type.Required {
			obj[field.Name] = strategy.generateValueForType(field.Type, g.schema, depth+1)
		}
	}

	return obj
}

// getInterestingValuesForArg gets interesting values for an argument
func (g *Generator) getInterestingValuesForArg(arg Argument, strategy *InterestingValuesStrategy) []GeneratedValue {
	return g.getInterestingValuesForType(arg.Type, strategy)
}

// getInterestingValuesForType gets interesting values for a type
func (g *Generator) getInterestingValuesForType(typeRef TypeRef, strategy *InterestingValuesStrategy) []GeneratedValue {
	baseName := getBaseTypeNameFromRef(typeRef)

	// Check if it's an enum
	if enumDef, ok := g.schema.Enums[baseName]; ok {
		return strategy.GenerateEnum(enumDef)
	}

	// Check if it's an input object
	if inputDef, ok := g.schema.InputTypes[baseName]; ok {
		return strategy.GenerateInputObject(inputDef, g.schema, 0)
	}

	// Must be a scalar
	return strategy.GenerateScalar(baseName)
}

// generateArgumentValue generates a value for an argument using the given strategy
func (g *Generator) generateArgumentValue(arg Argument, strategy *DefaultValueStrategy) interface{} {
	// Use default value if available
	if arg.DefaultValue != nil {
		return arg.DefaultValue
	}

	return strategy.generateValueForType(arg.Type, g.schema, 0)
}

// buildQueryString constructs the GraphQL query string
func (g *Generator) buildQueryString(opType string, op Operation, variables map[string]interface{}) string {
	var sb strings.Builder

	// Operation type and name
	sb.WriteString(opType)
	sb.WriteString(" ")
	sb.WriteString(op.Name)

	// Build variable definitions if there are any
	if len(variables) > 0 {
		sb.WriteString("(")
		first := true
		for _, arg := range op.Arguments {
			if _, ok := variables[arg.Name]; ok {
				if !first {
					sb.WriteString(", ")
				}
				sb.WriteString("$")
				sb.WriteString(arg.Name)
				sb.WriteString(": ")
				sb.WriteString(formatTypeRef(arg.Type))
				first = false
			}
		}
		sb.WriteString(")")
	}

	// Opening brace for selection set
	sb.WriteString(" {\n  ")
	sb.WriteString(op.Name)

	// Arguments using variables
	if len(variables) > 0 {
		sb.WriteString("(")
		first := true
		for _, arg := range op.Arguments {
			if _, ok := variables[arg.Name]; ok {
				if !first {
					sb.WriteString(", ")
				}
				sb.WriteString(arg.Name)
				sb.WriteString(": $")
				sb.WriteString(arg.Name)
				first = false
			}
		}
		sb.WriteString(")")
	}

	// Selection set for return type
	selectionSet := g.buildSelectionSet(op.ReturnType, 0)
	if selectionSet != "" {
		sb.WriteString(" ")
		sb.WriteString(selectionSet)
	}

	sb.WriteString("\n}")

	return sb.String()
}

// buildSelectionSet builds a selection set for a return type
func (g *Generator) buildSelectionSet(typeRef TypeRef, depth int) string {
	if depth > g.config.MaxDepth {
		return ""
	}

	baseName := getBaseTypeNameFromRef(typeRef)

	// Check if it's an object type
	typeDef, ok := g.schema.Types[baseName]
	if !ok {
		// Scalar or enum, no selection set needed
		return ""
	}

	// Build selection set with all scalar fields
	var fields []string
	for _, field := range typeDef.Fields {
		fieldBaseName := getBaseTypeNameFromRef(field.Type)

		// Add scalar and enum fields directly
		if g.isScalarOrEnum(fieldBaseName) {
			fields = append(fields, field.Name)
		} else if depth < g.config.MaxDepth {
			// For object types, recurse
			nestedSelection := g.buildSelectionSet(field.Type, depth+1)
			if nestedSelection != "" {
				fields = append(fields, field.Name+" "+nestedSelection)
			}
		}
	}

	if len(fields) == 0 {
		// Add __typename as fallback
		fields = append(fields, "__typename")
	}

	return "{\n    " + strings.Join(fields, "\n    ") + "\n  }"
}

// isScalarOrEnum checks if a type name is a scalar or enum
func (g *Generator) isScalarOrEnum(typeName string) bool {
	// Built-in scalars
	builtinScalars := map[string]bool{
		"String": true, "Int": true, "Float": true, "Boolean": true, "ID": true,
	}

	if builtinScalars[typeName] {
		return true
	}

	// Custom scalars
	for _, s := range g.schema.Scalars {
		if s == typeName {
			return true
		}
	}

	// Enums
	if _, ok := g.schema.Enums[typeName]; ok {
		return true
	}

	return false
}

// buildHeaders builds the request headers
func (g *Generator) buildHeaders() map[string]string {
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	// Copy configured headers
	for k, v := range g.config.Headers {
		headers[k] = v
	}

	return headers
}

// deduplicateRequests removes duplicate requests based on query + variables
func (g *Generator) deduplicateRequests(requests []RequestVariation) []RequestVariation {
	seen := make(map[string]bool)
	var unique []RequestVariation

	for _, req := range requests {
		sig := g.getRequestSignature(req)
		if !seen[sig] {
			seen[sig] = true
			unique = append(unique, req)
		}
	}

	return unique
}

// getRequestSignature creates a unique signature for a request
func (g *Generator) getRequestSignature(req RequestVariation) string {
	varsJSON, _ := json.Marshal(req.Variables)

	// Sort headers for consistent signature
	var headerParts []string
	for k, v := range req.Headers {
		headerParts = append(headerParts, k+"="+v)
	}
	sort.Strings(headerParts)

	return req.Query + "|" + string(varsJSON) + "|" + strings.Join(headerParts, "&")
}

// formatTypeRef formats a TypeRef as a GraphQL type string
func formatTypeRef(ref TypeRef) string {
	switch ref.Kind {
	case TypeKindNonNull:
		if ref.OfType != nil {
			return formatTypeRef(*ref.OfType) + "!"
		}
		return "!"
	case TypeKindList:
		if ref.OfType != nil {
			return "[" + formatTypeRef(*ref.OfType) + "]"
		}
		return "[]"
	default:
		return ref.Name
	}
}

// HTTPRequest represents the HTTP request to be sent
type HTTPRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

// ToHTTPRequest converts a RequestVariation to an HTTP request
func (rv *RequestVariation) ToHTTPRequest(baseURL string) (*HTTPRequest, error) {
	body := struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables,omitempty"`
		OperationName string                 `json:"operationName,omitempty"`
	}{
		Query:         rv.Query,
		Variables:     rv.Variables,
		OperationName: rv.OperationName,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	return &HTTPRequest{
		URL:     baseURL,
		Method:  "POST",
		Headers: rv.Headers,
		Body:    bodyBytes,
	}, nil
}
