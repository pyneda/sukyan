package graphql

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Parser handles GraphQL schema parsing
type Parser struct {
	client  *http.Client
	headers map[string]string
}

// NewParser creates a new GraphQL parser
func NewParser() *Parser {
	return &Parser{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		headers: make(map[string]string),
	}
}

// WithHeaders sets custom headers for the parser
func (p *Parser) WithHeaders(headers map[string]string) *Parser {
	p.headers = headers
	return p
}

// WithClient sets a custom HTTP client
func (p *Parser) WithClient(client *http.Client) *Parser {
	p.client = client
	return p
}

// ParseFromURL fetches and parses a GraphQL schema from an endpoint via introspection
func (p *Parser) ParseFromURL(url string) (*GraphQLSchema, error) {
	response, err := p.executeIntrospection(url)
	if err != nil {
		return nil, fmt.Errorf("introspection failed: %w", err)
	}

	if response.Data == nil || response.Data.Schema == nil {
		if len(response.Errors) > 0 {
			return nil, fmt.Errorf("introspection returned errors: %s", response.Errors[0].Message)
		}
		return nil, fmt.Errorf("introspection returned no schema data")
	}

	return p.convertSchema(response.Data.Schema)
}

// ParseFromJSON parses a GraphQL schema from an introspection JSON response
func (p *Parser) ParseFromJSON(data []byte) (*GraphQLSchema, error) {
	// Try parsing as full response { "data": { "__schema": ... } }
	var response IntrospectionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse introspection response: %w", err)
	}

	// If we got data with schema, use it
	if response.Data != nil && response.Data.Schema != nil {
		return p.convertSchema(response.Data.Schema)
	}

	// Try parsing as just the data portion { "__schema": ... }
	var dataOnly IntrospectionData
	if err := json.Unmarshal(data, &dataOnly); err == nil && dataOnly.Schema != nil {
		return p.convertSchema(dataOnly.Schema)
	}

	return nil, fmt.Errorf("invalid introspection data: schema is nil")
}

// FetchIntrospectionRaw performs introspection and returns the raw response bytes
func (p *Parser) FetchIntrospectionRaw(url string) ([]byte, error) {
	query := struct {
		Query string `json:"query"`
	}{
		Query: IntrospectionQuery,
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	for key, value := range p.headers {
		req.Header.Set(key, value)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return io.ReadAll(resp.Body)
}

// executeIntrospection sends the introspection query to the endpoint
func (p *Parser) executeIntrospection(url string) (*IntrospectionResponse, error) {
	query := struct {
		Query string `json:"query"`
	}{
		Query: IntrospectionQuery,
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	for key, value := range p.headers {
		req.Header.Set(key, value)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var introspectionResp IntrospectionResponse
	if err := json.Unmarshal(respBody, &introspectionResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &introspectionResp, nil
}

// convertSchema converts introspection data to our schema model
func (p *Parser) convertSchema(schema *IntrospectionSchema) (*GraphQLSchema, error) {
	result := &GraphQLSchema{
		Queries:       make([]Operation, 0),
		Mutations:     make([]Operation, 0),
		Subscriptions: make([]Operation, 0),
		Types:         make(map[string]TypeDef),
		Enums:         make(map[string]EnumDef),
		InputTypes:    make(map[string]InputTypeDef),
		Scalars:       make([]string, 0),
		Directives:    make([]DirectiveDef, 0),
	}

	// Build type maps first for reference
	typeMap := make(map[string]*IntrospectionType)
	for i := range schema.Types {
		t := &schema.Types[i]
		typeMap[t.Name] = t
	}

	// Process all types
	for _, t := range schema.Types {
		// Skip built-in types
		if strings.HasPrefix(t.Name, "__") {
			continue
		}

		switch t.Kind {
		case "SCALAR":
			result.Scalars = append(result.Scalars, t.Name)

		case "ENUM":
			result.Enums[t.Name] = p.convertEnum(t)

		case "INPUT_OBJECT":
			result.InputTypes[t.Name] = p.convertInputType(t)

		case "OBJECT":
			if t.Name != schema.QueryType.Name &&
				(schema.MutationType == nil || t.Name != schema.MutationType.Name) &&
				(schema.SubscriptionType == nil || t.Name != schema.SubscriptionType.Name) {
				result.Types[t.Name] = p.convertType(t)
			}
		}
	}

	// Extract Query operations
	if schema.QueryType != nil {
		if queryType, ok := typeMap[schema.QueryType.Name]; ok {
			result.Queries = p.extractOperations(queryType)
		}
	}

	// Extract Mutation operations
	if schema.MutationType != nil {
		if mutationType, ok := typeMap[schema.MutationType.Name]; ok {
			result.Mutations = p.extractOperations(mutationType)
		}
	}

	// Extract Subscription operations
	if schema.SubscriptionType != nil {
		if subscriptionType, ok := typeMap[schema.SubscriptionType.Name]; ok {
			result.Subscriptions = p.extractOperations(subscriptionType)
		}
	}

	// Convert directives
	for _, d := range schema.Directives {
		result.Directives = append(result.Directives, p.convertDirective(d))
	}

	return result, nil
}

// extractOperations extracts operations from a root type (Query/Mutation/Subscription)
func (p *Parser) extractOperations(t *IntrospectionType) []Operation {
	operations := make([]Operation, 0, len(t.Fields))

	for _, field := range t.Fields {
		op := Operation{
			Name:         field.Name,
			Description:  field.Description,
			ReturnType:   convertTypeRef(field.Type),
			IsDeprecated: field.IsDeprecated,
			Deprecation:  field.DeprecationReason,
			Arguments:    make([]Argument, 0, len(field.Args)),
		}

		for _, arg := range field.Args {
			op.Arguments = append(op.Arguments, p.convertArgument(arg))
		}

		operations = append(operations, op)
	}

	return operations
}

// convertArgument converts an introspection input value to an Argument
func (p *Parser) convertArgument(iv IntrospectionInputValue) Argument {
	arg := Argument{
		Name:        iv.Name,
		Description: iv.Description,
		Type:        convertTypeRef(iv.Type),
	}

	if iv.DefaultValue != nil {
		// Default value is a JSON string representation
		arg.DefaultValue = *iv.DefaultValue
	}

	return arg
}

// convertEnum converts an introspection enum type
func (p *Parser) convertEnum(t IntrospectionType) EnumDef {
	enumDef := EnumDef{
		Name:        t.Name,
		Description: t.Description,
		Values:      make([]EnumValue, 0, len(t.EnumValues)),
	}

	for _, ev := range t.EnumValues {
		enumDef.Values = append(enumDef.Values, EnumValue{
			Name:         ev.Name,
			Description:  ev.Description,
			IsDeprecated: ev.IsDeprecated,
			Deprecation:  ev.DeprecationReason,
		})
	}

	return enumDef
}

// convertInputType converts an introspection input object type
func (p *Parser) convertInputType(t IntrospectionType) InputTypeDef {
	inputDef := InputTypeDef{
		Name:        t.Name,
		Description: t.Description,
		Fields:      make([]InputField, 0, len(t.InputFields)),
	}

	for _, f := range t.InputFields {
		field := InputField{
			Name:        f.Name,
			Description: f.Description,
			Type:        convertTypeRef(f.Type),
		}
		if f.DefaultValue != nil {
			field.DefaultValue = *f.DefaultValue
		}
		inputDef.Fields = append(inputDef.Fields, field)
	}

	return inputDef
}

// convertType converts an introspection object type
func (p *Parser) convertType(t IntrospectionType) TypeDef {
	typeDef := TypeDef{
		Name:        t.Name,
		Description: t.Description,
		Fields:      make([]Field, 0, len(t.Fields)),
		Interfaces:  make([]string, 0, len(t.Interfaces)),
	}

	for _, f := range t.Fields {
		field := Field{
			Name:         f.Name,
			Description:  f.Description,
			Type:         convertTypeRef(f.Type),
			IsDeprecated: f.IsDeprecated,
			Deprecation:  f.DeprecationReason,
			Arguments:    make([]Argument, 0, len(f.Args)),
		}

		for _, arg := range f.Args {
			field.Arguments = append(field.Arguments, p.convertArgument(arg))
		}

		typeDef.Fields = append(typeDef.Fields, field)
	}

	for _, iface := range t.Interfaces {
		typeDef.Interfaces = append(typeDef.Interfaces, getBaseTypeName(iface))
	}

	return typeDef
}

// convertDirective converts an introspection directive
func (p *Parser) convertDirective(d IntrospectionDirective) DirectiveDef {
	directive := DirectiveDef{
		Name:        d.Name,
		Description: d.Description,
		Locations:   d.Locations,
		Arguments:   make([]Argument, 0, len(d.Args)),
	}

	for _, arg := range d.Args {
		directive.Arguments = append(directive.Arguments, p.convertArgument(arg))
	}

	return directive
}
