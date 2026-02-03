package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/pkg/api/core"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
)

type RequestBuilder struct {
	DefaultHeaders map[string]string
	AuthConfig     *AuthConfig
	MaxDepth       int
	schema         *pkgGraphql.GraphQLSchema
}

type AuthConfig struct {
	BearerToken   string
	CustomHeaders map[string]string
}

type GraphQLRequest struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

func NewRequestBuilder() *RequestBuilder {
	return &RequestBuilder{
		DefaultHeaders: map[string]string{
			"User-Agent":   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36",
			"Content-Type": "application/json",
			"Accept":       "application/json",
		},
		MaxDepth: 3,
	}
}

func (b *RequestBuilder) WithAuth(config *AuthConfig) *RequestBuilder {
	b.AuthConfig = config
	return b
}

func (b *RequestBuilder) WithMaxDepth(depth int) *RequestBuilder {
	b.MaxDepth = depth
	return b
}

func (b *RequestBuilder) WithSchema(schema *pkgGraphql.GraphQLSchema) *RequestBuilder {
	b.schema = schema
	return b
}

func (b *RequestBuilder) Build(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	if op.GraphQL == nil {
		return nil, fmt.Errorf("operation is not a GraphQL operation")
	}

	query := b.buildQuery(op, paramValues)

	variables := make(map[string]any)
	for _, param := range op.Parameters {
		value := paramValues[param.Name]
		if value == nil {
			value = param.GetEffectiveValue()
		}
		if value != nil {
			variables[param.Name] = value
		}
	}

	gqlRequest := GraphQLRequest{
		Query:         query,
		OperationName: op.Name,
	}

	if len(variables) > 0 {
		gqlRequest.Variables = variables
	}

	body, err := json.Marshal(gqlRequest)
	if err != nil {
		return nil, fmt.Errorf("marshaling GraphQL request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", op.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	b.addDefaultHeaders(req)
	b.applyAuth(req)

	return req, nil
}

func (b *RequestBuilder) BuildWithModifiedParam(ctx context.Context, op core.Operation, paramName string, modifiedValue any, paramValues map[string]any) (*http.Request, error) {
	modifiedValues := make(map[string]any)
	for k, v := range paramValues {
		modifiedValues[k] = v
	}
	modifiedValues[paramName] = modifiedValue
	return b.Build(ctx, op, modifiedValues)
}

func (b *RequestBuilder) GetDefaultParamValues(op core.Operation) map[string]any {
	values := make(map[string]any)
	for _, param := range op.Parameters {
		values[param.Name] = param.GetEffectiveValue()
	}
	return values
}

func (b *RequestBuilder) buildQuery(op core.Operation, paramValues map[string]any) string {
	if op.GraphQL == nil {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(op.GraphQL.OperationType)
	sb.WriteString(" ")
	sb.WriteString(op.Name)

	if len(op.Parameters) > 0 {
		sb.WriteString("(")
		var params []string
		for _, param := range op.Parameters {
			typeName := b.getGraphQLTypeName(param)
			params = append(params, fmt.Sprintf("$%s: %s", param.Name, typeName))
		}
		sb.WriteString(strings.Join(params, ", "))
		sb.WriteString(")")
	}

	sb.WriteString(" {\n")
	sb.WriteString("  ")
	sb.WriteString(op.Name)

	if len(op.Parameters) > 0 {
		sb.WriteString("(")
		var args []string
		for _, param := range op.Parameters {
			args = append(args, fmt.Sprintf("%s: $%s", param.Name, param.Name))
		}
		sb.WriteString(strings.Join(args, ", "))
		sb.WriteString(")")
	}

	if b.schema != nil && op.GraphQL != nil && op.GraphQL.ReturnType != "" {
		selectionSet := b.buildSelectionSetFromTypeName(op.GraphQL.ReturnType, 0)
		if selectionSet != "" {
			sb.WriteString(" ")
			sb.WriteString(selectionSet)
		} else {
			sb.WriteString(" {\n    __typename\n  }")
		}
	} else {
		sb.WriteString(" {\n    __typename\n  }")
	}

	sb.WriteString("\n}")

	return sb.String()
}

func (b *RequestBuilder) getGraphQLTypeName(param core.Parameter) string {
	typeName := ""

	switch param.DataType {
	case core.DataTypeString:
		if param.Constraints.Format == "id" {
			typeName = "ID"
		} else {
			typeName = "String"
		}
	case core.DataTypeInteger:
		typeName = "Int"
	case core.DataTypeNumber:
		typeName = "Float"
	case core.DataTypeBoolean:
		typeName = "Boolean"
	case core.DataTypeArray:
		innerType := "String"
		if len(param.NestedParams) > 0 {
			innerType = b.getGraphQLTypeName(param.NestedParams[0])
		}
		typeName = fmt.Sprintf("[%s]", innerType)
	case core.DataTypeObject:
		typeName = "JSONObject"
	default:
		typeName = "String"
	}

	if len(param.Constraints.Enum) > 0 {
		typeName = param.Name + "Enum"
	}

	if param.Required {
		typeName += "!"
	}

	return typeName
}

func (b *RequestBuilder) addDefaultHeaders(req *http.Request) {
	for k, v := range b.DefaultHeaders {
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
}

func (b *RequestBuilder) applyAuth(req *http.Request) {
	if b.AuthConfig == nil {
		return
	}

	if b.AuthConfig.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.AuthConfig.BearerToken)
	}

	for k, v := range b.AuthConfig.CustomHeaders {
		req.Header.Set(k, v)
	}
}

func (b *RequestBuilder) buildSelectionSetFromTypeName(typeName string, depth int) string {
	if b.schema == nil || depth > b.MaxDepth {
		return ""
	}

	baseName := stripTypeModifiers(typeName)

	typeDef, ok := b.schema.Types[baseName]
	if !ok {
		return ""
	}

	var fields []string
	for _, field := range typeDef.Fields {
		fieldBaseName := getBaseTypeNameFromRef(field.Type)
		if b.isScalarOrEnumType(fieldBaseName) {
			fields = append(fields, field.Name)
		} else if depth < b.MaxDepth {
			nestedSelection := b.buildSelectionSetFromTypeName(fieldBaseName, depth+1)
			if nestedSelection != "" {
				fields = append(fields, field.Name+" "+nestedSelection)
			}
		}
	}

	if len(fields) == 0 {
		fields = append(fields, "__typename")
	}

	return "{\n    " + strings.Join(fields, "\n    ") + "\n  }"
}

func (b *RequestBuilder) isScalarOrEnumType(typeName string) bool {
	builtinScalars := map[string]bool{
		"String": true, "Int": true, "Float": true, "Boolean": true, "ID": true,
	}
	if builtinScalars[typeName] {
		return true
	}
	if b.schema != nil {
		for _, s := range b.schema.Scalars {
			if s == typeName {
				return true
			}
		}
		if _, ok := b.schema.Enums[typeName]; ok {
			return true
		}
	}
	return false
}

func getBaseTypeNameFromRef(ref pkgGraphql.TypeRef) string {
	if ref.OfType != nil {
		return getBaseTypeNameFromRef(*ref.OfType)
	}
	return ref.Name
}

func stripTypeModifiers(typeName string) string {
	replacer := strings.NewReplacer("[", "", "]", "", "!", "")
	return replacer.Replace(typeName)
}

func BuildIntrospectionRequest(ctx context.Context, baseURL string) (*http.Request, error) {
	introspectionQuery := `query IntrospectionQuery {
  __schema {
    types {
      name
    }
  }
}`

	gqlRequest := GraphQLRequest{
		Query: introspectionQuery,
	}

	body, err := json.Marshal(gqlRequest)
	if err != nil {
		return nil, fmt.Errorf("marshaling introspection request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return req, nil
}

func BuildBatchRequest(ctx context.Context, baseURL string, queries []GraphQLRequest) (*http.Request, error) {
	body, err := json.Marshal(queries)
	if err != nil {
		return nil, fmt.Errorf("marshaling batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return req, nil
}

func BuildRequest(ctx context.Context, op core.Operation, paramValues map[string]any) (*http.Request, error) {
	builder := NewRequestBuilder()
	return builder.Build(ctx, op, paramValues)
}
