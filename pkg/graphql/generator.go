package graphql

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

// labelHappyPath marks the baseline request for an operation: schema-valid
// values throughout, so a rejection points at the scanner rather than the target.
const labelHappyPath = "Happy Path"

// Generator creates GraphQL requests from parsed schemas
type Generator struct {
	schema    *GraphQLSchema
	config    GenerationConfig
	selection SelectionOptions
}

// NewGenerator creates a new request generator
func NewGenerator(schema *GraphQLSchema, config GenerationConfig) *Generator {
	if config.MaxDepth <= 0 {
		config.MaxDepth = defaultMaxSelectionDepth
	}
	if config.MaxListItems <= 0 {
		config.MaxListItems = 2
	}
	if config.MaxFieldsPerSelection <= 0 {
		config.MaxFieldsPerSelection = defaultMaxFieldsPerSelection
	}
	if config.MaxTotalFields <= 0 {
		config.MaxTotalFields = defaultMaxTotalFields
	}
	if schema == nil {
		schema = &GraphQLSchema{}
	}

	return &Generator{
		schema: schema,
		config: config,
		selection: SelectionOptions{
			MaxDepth:          config.MaxDepth,
			MaxFieldsPerLevel: config.MaxFieldsPerSelection,
			MaxTotalFields:    config.MaxTotalFields,
			IncludeDeprecated: true,
		},
	}
}

// GenerateRequests generates request variations for all operations
func (g *Generator) GenerateRequests() []OperationEndpoint {
	var endpoints []OperationEndpoint

	// Kept in a fixed order rather than a map so repeated runs against the same
	// schema queue the same requests in the same sequence.
	for _, group := range []struct {
		opType     string
		operations []Operation
	}{
		{"query", g.schema.Queries},
		{"mutation", g.schema.Mutations},
		{"subscription", g.schema.Subscriptions},
	} {
		for _, op := range group.operations {
			// A name that is not a legal GraphQL name cannot be written into a
			// document at all; a hostile schema supplying one would otherwise
			// splice arbitrary text into every request built from it.
			if !IsValidName(op.Name) {
				log.Warn().Str("operation", op.Name).Str("type", group.opType).Msg("Skipping GraphQL operation with an invalid name")
				continue
			}
			endpoints = append(endpoints, g.generateOperationEndpoint(group.opType, op))
		}
	}

	return endpoints
}

// generateOperationEndpoint creates an endpoint with all request variations for an operation
func (g *Generator) generateOperationEndpoint(opType string, op Operation) OperationEndpoint {
	endpoint := OperationEndpoint{
		OperationType: opType,
		Name:          op.Name,
		Description:   op.Description,
		ReturnType:    op.ReturnType.Signature(),
		Arguments:     g.buildArgumentMetadata(op.Arguments),
		Requests:      make([]RequestVariation, 0),
	}

	endpoint.Requests = append(endpoint.Requests, g.generateHappyPathRequest(opType, op))

	if g.config.FuzzingEnabled {
		endpoint.Requests = append(endpoint.Requests, g.generateFuzzRequests(opType, op)...)
	}

	endpoint.Requests = g.deduplicateRequests(endpoint.Requests)

	// Variations multiply with argument count, so a handful of wide operations
	// can otherwise dominate a scan's whole request budget.
	if limit := g.config.MaxRequestsPerOperation; limit > 0 && len(endpoint.Requests) > limit {
		log.Debug().
			Str("operation", op.Name).
			Int("generated", len(endpoint.Requests)).
			Int("limit", limit).
			Msg("Truncating GraphQL request variations")
		endpoint.Requests = endpoint.Requests[:limit]
	}

	return endpoint
}

// buildArgumentMetadata builds detailed argument metadata for DAST scanning
func (g *Generator) buildArgumentMetadata(args []Argument) []ArgumentMetadata {
	metadata := make([]ArgumentMetadata, 0, len(args))

	for _, arg := range args {
		meta := ArgumentMetadata{
			Name:          arg.Name,
			TypeName:      getBaseTypeNameFromRef(arg.Type),
			FullType:      arg.Type.Signature(),
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
			FullType:      field.Type.Signature(),
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
	strategy := g.defaultStrategy()
	variables := make(map[string]interface{})

	for _, arg := range op.Arguments {
		if !arg.Type.Required && !g.config.IncludeOptionalParams {
			continue
		}

		value := g.generateArgumentValue(arg, strategy)
		if arg.Type.Required && value == nil {
			// A non-null argument must be both present and non-null. Some scalars
			// have no sensible client-side value at all (Upload without a
			// multipart body), and for those a placeholder still reaches the
			// server's coercion step, whereas null or omission is rejected during
			// validation along with the rest of the document.
			value = g.placeholderForRequired(arg.Type)
		}
		if value != nil {
			variables[arg.Name] = value
		}
	}

	// Build the query string
	query := g.buildQueryString(opType, op, variables)

	return RequestVariation{
		Label:         labelHappyPath,
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
	defaultStrategy := g.defaultStrategy()

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
				Description:   fmt.Sprintf("Fuzzing argument '%s' (%s) with: %s", targetArg.Name, targetArg.Type.Signature(), gv.Description),
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

// buildQueryString constructs the GraphQL query string. Only arguments that are
// present in variables and carry a renderable type are declared, so a malformed
// entry in the schema costs that one argument rather than the whole document.
func (g *Generator) buildQueryString(opType string, op Operation, variables map[string]interface{}) string {
	declared := g.declarableArguments(op.Arguments, variables)

	var sb strings.Builder
	sb.WriteString(opType)
	sb.WriteString(" ")
	sb.WriteString(op.Name)

	if len(declared) > 0 {
		sb.WriteString("(")
		for i, arg := range declared {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("$")
			sb.WriteString(arg.Name)
			sb.WriteString(": ")
			sb.WriteString(arg.Type.Signature())
		}
		sb.WriteString(")")
	}

	sb.WriteString(" {\n  ")
	sb.WriteString(op.Name)

	if len(declared) > 0 {
		sb.WriteString("(")
		for i, arg := range declared {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(arg.Name)
			sb.WriteString(": $")
			sb.WriteString(arg.Name)
		}
		sb.WriteString(")")
	}

	if selectionSet := g.schema.BuildSelectionSet(op.ReturnType, g.selection); selectionSet != "" {
		sb.WriteString(" ")
		sb.WriteString(selectionSet)
	}

	sb.WriteString("\n}")

	return sb.String()
}

// declarableArguments returns the arguments that can be written into a document,
// and prunes any variable that cannot be from the map so the two stay in step:
// a variable supplied but never declared is itself a validation error.
func (g *Generator) declarableArguments(args []Argument, variables map[string]interface{}) []Argument {
	declared := make([]Argument, 0, len(variables))

	for _, arg := range args {
		if _, ok := variables[arg.Name]; !ok {
			continue
		}
		if IsValidName(arg.Name) && arg.Type.Signature() != "" {
			declared = append(declared, arg)
			continue
		}

		log.Debug().Str("argument", arg.Name).Msg("Dropping GraphQL argument that cannot be rendered")
		delete(variables, arg.Name)
	}

	return declared
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
	seen := make(map[string]bool, len(requests))
	unique := make([]RequestVariation, 0, len(requests))

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

// defaultStrategy builds a value strategy bound to this generator's limits, so
// the configured depth and list bounds apply to variables and not only to the
// selection sets built alongside them.
func (g *Generator) defaultStrategy() *DefaultValueStrategy {
	return NewDefaultValueStrategy(g.schema).WithLimits(g.config.MaxDepth, g.config.MaxListItems)
}

// placeholderForRequired produces a value for a non-null argument the value
// strategies had nothing for. It reuses the schema's own literal renderer so the
// placeholder is the same shape the document would carry inline, and decodes it
// back into the Go value a variables map needs.
func (g *Generator) placeholderForRequired(ref TypeRef) interface{} {
	literal, ok := g.schema.DefaultLiteral(ref)
	if !ok {
		return nil
	}

	value, err := ParseLiteral(literal)
	if err != nil {
		log.Debug().Err(err).Str("literal", literal).Msg("Could not decode GraphQL placeholder literal")
		return nil
	}
	return value
}
