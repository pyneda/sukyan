package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/pyneda/sukyan/pkg/api/graphql"
	"github.com/pyneda/sukyan/pkg/api/openapi"
	"github.com/pyneda/sukyan/pkg/api/soap"
	pkgGraphql "github.com/pyneda/sukyan/pkg/graphql"
)

func DetectAPIType(content []byte, sourceURL string) db.APIDefinitionType {
	contentStr := string(content)

	if strings.Contains(contentStr, `"openapi"`) ||
		strings.Contains(contentStr, `"swagger"`) ||
		strings.Contains(contentStr, "openapi:") ||
		strings.Contains(contentStr, "swagger:") {
		return db.APIDefinitionTypeOpenAPI
	}

	var jsonObj map[string]interface{}
	if err := json.Unmarshal(content, &jsonObj); err == nil {
		if data, ok := jsonObj["data"].(map[string]interface{}); ok {
			if _, ok := data["__schema"]; ok {
				return db.APIDefinitionTypeGraphQL
			}
		}
		if _, ok := jsonObj["__schema"]; ok {
			return db.APIDefinitionTypeGraphQL
		}
	}

	if strings.Contains(contentStr, "__schema") ||
		strings.Contains(contentStr, "queryType") ||
		strings.Contains(contentStr, "mutationType") {
		return db.APIDefinitionTypeGraphQL
	}

	if strings.Contains(contentStr, "wsdl:definitions") ||
		strings.Contains(contentStr, "soap:") ||
		strings.Contains(contentStr, "<definitions") {
		return db.APIDefinitionTypeWSDL
	}

	lowURL := strings.ToLower(sourceURL)
	if strings.Contains(lowURL, "graphql") {
		return db.APIDefinitionTypeGraphQL
	}
	if strings.HasSuffix(lowURL, ".wsdl") || strings.Contains(lowURL, "?wsdl") {
		return db.APIDefinitionTypeWSDL
	}

	return db.APIDefinitionTypeOpenAPI
}

func BuildRequest(ctx context.Context, apiType core.APIType, operation core.Operation, paramValues map[string]any) (*http.Request, error) {
	switch apiType {
	case core.APITypeGraphQL:
		return graphql.NewRequestBuilder().Build(ctx, operation, paramValues)
	case core.APITypeSOAP:
		return soap.NewRequestBuilder().Build(ctx, operation, paramValues)
	default:
		return openapi.NewRequestBuilder().Build(ctx, operation, paramValues)
	}
}

func BuildDefaultRequest(ctx context.Context, defType db.APIDefinitionType, operation *core.Operation, graphqlSchema ...*pkgGraphql.GraphQLSchema) (*http.Request, error) {
	switch defType {
	case db.APIDefinitionTypeOpenAPI:
		builder := openapi.NewRequestBuilder()
		defaultValues := builder.GetDefaultParamValues(*operation)
		return builder.Build(ctx, *operation, defaultValues)
	case db.APIDefinitionTypeGraphQL:
		builder := graphql.NewRequestBuilder()
		if len(graphqlSchema) > 0 && graphqlSchema[0] != nil {
			builder = builder.WithSchema(graphqlSchema[0])
		}
		defaultValues := builder.GetDefaultParamValues(*operation)
		return builder.Build(ctx, *operation, defaultValues)
	case db.APIDefinitionTypeWSDL:
		builder := soap.NewRequestBuilder()
		defaultValues := builder.GetDefaultParamValues(*operation)
		return builder.Build(ctx, *operation, defaultValues)
	default:
		builder := core.NewBaseRequestBuilder()
		defaultValues := builder.GetDefaultParamValues(*operation)
		return builder.Build(ctx, *operation, defaultValues)
	}
}
