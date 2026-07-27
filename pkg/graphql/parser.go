package graphql

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	defaultIntrospectionTimeout = 30 * time.Second
	// defaultMaxResponseBytes bounds the introspection response read into memory.
	// The endpoint is an untrusted target, and an unbounded io.ReadAll against one
	// that streams indefinitely takes the whole scanner down with it.
	defaultMaxResponseBytes = 32 << 20
	// errorBodyPreview bounds how much of a failed response is quoted back in an
	// error, which is logged and persisted.
	errorBodyPreview = 512
)

// Parser handles GraphQL schema parsing
type Parser struct {
	client           *http.Client
	headers          map[string]string
	maxResponseBytes int64
}

// NewParser creates a new GraphQL parser
func NewParser() *Parser {
	return &Parser{
		client: &http.Client{
			Timeout: defaultIntrospectionTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		headers:          make(map[string]string),
		maxResponseBytes: defaultMaxResponseBytes,
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

// WithMaxResponseSize caps how many bytes of an introspection response are read.
func (p *Parser) WithMaxResponseSize(limit int64) *Parser {
	if limit > 0 {
		p.maxResponseBytes = limit
	}
	return p
}

// ParseFromURL fetches and parses a GraphQL schema from an endpoint via introspection
func (p *Parser) ParseFromURL(url string) (*GraphQLSchema, error) {
	return p.ParseFromURLContext(context.Background(), url)
}

// ParseFromURLContext is ParseFromURL bound to a context, so a cancelled scan
// stops introspecting instead of holding the request open to its own timeout.
func (p *Parser) ParseFromURLContext(ctx context.Context, url string) (*GraphQLSchema, error) {
	body, err := p.FetchIntrospectionRawContext(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("introspection failed: %w", err)
	}
	return p.ParseFromJSON(body)
}

// ParseFromJSON parses a GraphQL schema from an introspection JSON response
func (p *Parser) ParseFromJSON(data []byte) (*GraphQLSchema, error) {
	response, err := decodeIntrospection(data)
	if err != nil {
		return nil, err
	}
	return p.convertSchema(response.Data.Schema)
}

// FetchIntrospectionRaw performs introspection and returns the raw response bytes
func (p *Parser) FetchIntrospectionRaw(url string) ([]byte, error) {
	return p.FetchIntrospectionRawContext(context.Background(), url)
}

// FetchIntrospectionRawContext is FetchIntrospectionRaw bound to a context.
func (p *Parser) FetchIntrospectionRawContext(ctx context.Context, url string) ([]byte, error) {
	body, status, err := p.postIntrospection(ctx, url)
	if err != nil {
		return nil, err
	}

	// A non-2xx status is not conclusive: servers routinely answer introspection
	// with 400 or 500 while still returning a usable schema document, so the body
	// decides and the status only shapes the error when it does not.
	if _, decodeErr := decodeIntrospection(body); decodeErr == nil {
		return body, nil
	} else if status != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", status, preview(body))
	} else {
		return nil, decodeErr
	}
}

// decodeIntrospection accepts either a full GraphQL response or a bare data
// object, which is the shape schemas are stored in once captured.
func decodeIntrospection(data []byte) (*IntrospectionResponse, error) {
	var response IntrospectionResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("failed to parse introspection response: %w", err)
	}

	if response.Data != nil && response.Data.Schema != nil {
		return &response, nil
	}

	var dataOnly IntrospectionData
	if err := json.Unmarshal(data, &dataOnly); err == nil && dataOnly.Schema != nil {
		return &IntrospectionResponse{Data: &dataOnly}, nil
	}

	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("introspection returned errors: %s", response.Errors[0].Message)
	}
	return nil, fmt.Errorf("invalid introspection data: schema is nil")
}

func (p *Parser) postIntrospection(ctx context.Context, url string) ([]byte, int, error) {
	payload, err := json.Marshal(struct {
		Query string `json:"query"`
	}{Query: IntrospectionQuery})
	if err != nil {
		return nil, 0, fmt.Errorf("failed to marshal query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	for key, value := range p.headers {
		req.Header.Set(key, value)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limit := p.maxResponseBytes
	if limit <= 0 {
		limit = defaultMaxResponseBytes
	}

	// Read one byte past the limit so an oversized response is reported rather
	// than silently truncated into an unparseable document.
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, resp.StatusCode, fmt.Errorf("introspection response exceeds %d bytes", limit)
	}

	return body, resp.StatusCode, nil
}

func preview(body []byte) string {
	if len(body) <= errorBodyPreview {
		return string(body)
	}
	return string(body[:errorBodyPreview]) + "... (truncated)"
}

// convertSchema converts introspection data to our schema model
func (p *Parser) convertSchema(schema *IntrospectionSchema) (*GraphQLSchema, error) {
	if schema == nil {
		return nil, fmt.Errorf("invalid introspection data: schema is nil")
	}

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

	typeMap := make(map[string]*IntrospectionType, len(schema.Types))
	for i := range schema.Types {
		typeMap[schema.Types[i].Name] = &schema.Types[i]
	}

	for _, t := range schema.Types {
		if strings.HasPrefix(t.Name, "__") || t.Name == "" {
			continue
		}

		switch t.Kind {
		case "SCALAR":
			result.Scalars = append(result.Scalars, t.Name)
		case "ENUM":
			result.Enums[t.Name] = p.convertEnum(t)
		case "INPUT_OBJECT":
			result.InputTypes[t.Name] = p.convertInputType(t)
		case "OBJECT", "INTERFACE", "UNION":
			// Root operation types are kept here too: a schema may expose one as
			// an ordinary field (`viewer: Query`), and omitting it would leave
			// that field looking like a scalar and break the document.
			result.Types[t.Name] = p.convertType(t)
		default:
			log.Debug().Str("type", t.Name).Str("kind", t.Kind).Msg("Skipping GraphQL type of unknown kind")
		}
	}

	result.Queries = p.rootOperations(schema.QueryType, typeMap)
	result.Mutations = p.rootOperations(schema.MutationType, typeMap)
	result.Subscriptions = p.rootOperations(schema.SubscriptionType, typeMap)

	for _, d := range schema.Directives {
		result.Directives = append(result.Directives, p.convertDirective(d))
	}

	return result, nil
}

func (p *Parser) rootOperations(root *TypeName, typeMap map[string]*IntrospectionType) []Operation {
	if root == nil || root.Name == "" {
		return []Operation{}
	}

	rootType, ok := typeMap[root.Name]
	if !ok {
		log.Warn().Str("type", root.Name).Msg("GraphQL schema names a root type it does not define")
		return []Operation{}
	}

	return p.extractOperations(rootType)
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
			Arguments:    p.convertArguments(field.Args),
		}

		operations = append(operations, op)
	}

	return operations
}

func (p *Parser) convertArguments(args []IntrospectionInputValue) []Argument {
	converted := make([]Argument, 0, len(args))
	for _, arg := range args {
		converted = append(converted, p.convertArgument(arg))
	}
	return converted
}

// convertArgument converts an introspection input value to an Argument
func (p *Parser) convertArgument(iv IntrospectionInputValue) Argument {
	arg := Argument{
		Name:        iv.Name,
		Description: iv.Description,
		Type:        convertTypeRef(iv.Type),
	}

	arg.DefaultLiteral, arg.DefaultValue = decodeDefault(iv.Name, iv.DefaultValue)
	return arg
}

// decodeDefault turns an introspected default into both forms the generator
// needs. A literal that cannot be decoded is dropped rather than passed through
// as text, which would be sent as a string of whatever type it actually is.
func decodeDefault(name string, literal *string) (string, interface{}) {
	if literal == nil || *literal == "" {
		return "", nil
	}

	value, err := ParseLiteral(*literal)
	if err != nil {
		log.Debug().Err(err).Str("argument", name).Str("literal", *literal).Msg("Ignoring unparseable GraphQL default value")
		return "", nil
	}
	return *literal, value
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
		field.DefaultLiteral, field.DefaultValue = decodeDefault(f.Name, f.DefaultValue)
		inputDef.Fields = append(inputDef.Fields, field)
	}

	return inputDef
}

// convertType converts an introspection object, interface or union type
func (p *Parser) convertType(t IntrospectionType) TypeDef {
	typeDef := TypeDef{
		Name:        t.Name,
		Description: t.Description,
		Kind:        TypeKind(t.Kind),
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
			Arguments:    p.convertArguments(f.Args),
		}

		typeDef.Fields = append(typeDef.Fields, field)
	}

	for _, iface := range t.Interfaces {
		if name := getBaseTypeName(iface); name != "" {
			typeDef.Interfaces = append(typeDef.Interfaces, name)
		}
	}

	for _, possible := range t.PossibleTypes {
		if name := getBaseTypeName(possible); name != "" {
			typeDef.PossibleTypes = append(typeDef.PossibleTypes, name)
		}
	}

	return typeDef
}

// convertDirective converts an introspection directive
func (p *Parser) convertDirective(d IntrospectionDirective) DirectiveDef {
	return DirectiveDef{
		Name:        d.Name,
		Description: d.Description,
		Locations:   d.Locations,
		Arguments:   p.convertArguments(d.Args),
	}
}
