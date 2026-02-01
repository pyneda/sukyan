package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/graphql"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/openapi"
	"github.com/rs/zerolog/log"
)

type ImportOptions struct {
	WorkspaceID  uint
	Name         string
	SourceURL    string
	BaseURL      string
	Type         string
	AuthConfigID *uuid.UUID
}

type FetchedContent struct {
	Content   []byte
	SourceURL string
	Type      db.APIDefinitionType
}

func FetchAPIContent(url, content, typeHint string) (*FetchedContent, error) {
	result := &FetchedContent{}

	if url != "" {
		result.SourceURL = url
		if typeHint == "graphql" {
			parser := graphql.NewParser()
			bodyBytes, err := parser.FetchIntrospectionRaw(url)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch GraphQL schema from URL: %w", err)
			}
			result.Content = bodyBytes
		} else {
			bodyBytes, err := http_utils.FetchOpenAPISpec(url)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch API spec from URL: %w", err)
			}
			result.Content = bodyBytes
		}
	} else if content != "" {
		result.Content = []byte(content)
	} else {
		return nil, fmt.Errorf("either url or content is required")
	}

	result.Type = DetectAPIType(result.Content, result.SourceURL)
	if typeHint != "" {
		result.Type = db.APIDefinitionType(typeHint)
	}

	return result, nil
}

func ImportAPIDefinition(content []byte, sourceURL string, opts ImportOptions) (*db.APIDefinition, error) {
	apiType := db.APIDefinitionType(opts.Type)
	if apiType == "" {
		apiType = DetectAPIType(content, sourceURL)
	}

	switch apiType {
	case db.APIDefinitionTypeOpenAPI:
		return importOpenAPIDefinition(content, sourceURL, opts)
	case db.APIDefinitionTypeGraphQL:
		return importGraphQLDefinition(content, sourceURL, opts)
	case db.APIDefinitionTypeWSDL:
		return nil, fmt.Errorf("WSDL import is not yet supported via this path")
	default:
		return nil, fmt.Errorf("unsupported API type: %s", apiType)
	}
}

func importOpenAPIDefinition(content []byte, sourceURL string, opts ImportOptions) (*db.APIDefinition, error) {
	doc, err := openapi.Parse(content)
	if err != nil {
		return nil, err
	}

	var openapiVersion string
	var openapiTitle string
	var serverCount int

	var jsonObj map[string]interface{}
	if err := json.Unmarshal(content, &jsonObj); err == nil {
		if v, ok := jsonObj["openapi"].(string); ok {
			openapiVersion = v
		} else if v, ok := jsonObj["swagger"].(string); ok {
			openapiVersion = v
		}
		if info, ok := jsonObj["info"].(map[string]interface{}); ok {
			if t, ok := info["title"].(string); ok {
				openapiTitle = t
			}
		}
		if servers, ok := jsonObj["servers"].([]interface{}); ok {
			serverCount = len(servers)
		}
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = doc.BaseURL()
	}
	if baseURL == "" && sourceURL != "" {
		baseURL = DeriveBaseURLFromSpecURL(sourceURL)
	}

	name := opts.Name
	if name == "" {
		if openapiTitle != "" {
			name = openapiTitle
		} else {
			name = "OpenAPI - " + baseURL
		}
	}

	definition := &db.APIDefinition{
		WorkspaceID:    opts.WorkspaceID,
		Name:           name,
		Type:           db.APIDefinitionTypeOpenAPI,
		Status:         db.APIDefinitionStatusParsed,
		SourceURL:      sourceURL,
		BaseURL:        baseURL,
		RawDefinition:  content,
		AutoDiscovered: false,
		AuthConfigID:   opts.AuthConfigID,
		OpenAPIVersion: &openapiVersion,
		OpenAPITitle:   &openapiTitle,
		OpenAPIServers: serverCount,
	}

	definition, err = db.Connection().CreateAPIDefinition(definition)
	if err != nil {
		return nil, err
	}

	persistSecuritySchemes(doc, definition)
	persistGlobalSecurity(doc, definition)
	endpoints := persistOpenAPIEndpoints(doc, definition)
	persistEndpointSecurity(doc, definition, endpoints)

	definition, _ = db.Connection().GetAPIDefinitionByIDWithEndpoints(definition.ID)
	return definition, nil
}

func persistSecuritySchemes(doc *openapi.Document, definition *db.APIDefinition) {
	specSchemes := doc.GetSecuritySchemes()
	if len(specSchemes) == 0 {
		return
	}

	var dbSchemes []*db.APIDefinitionSecurityScheme
	for _, s := range specSchemes {
		dbSchemes = append(dbSchemes, &db.APIDefinitionSecurityScheme{
			DefinitionID:     definition.ID,
			Name:             s.Name,
			Type:             s.Type,
			Scheme:           s.Scheme,
			In:               s.In,
			ParameterName:    s.ParameterName,
			BearerFormat:     s.BearerFormat,
			Description:      s.Description,
			OpenIDConnectURL: s.OpenIDConnectURL,
		})
	}
	if err := db.Connection().CreateAPIDefinitionSecuritySchemes(dbSchemes); err != nil {
		log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to create security schemes")
	}
}

func persistGlobalSecurity(doc *openapi.Document, definition *db.APIDefinition) {
	globalSecurity := doc.GetGlobalSecurityRequirements()
	if len(globalSecurity) == 0 {
		return
	}

	globalSecJSON, err := json.Marshal(globalSecurity)
	if err != nil {
		return
	}

	definition.GlobalSecurityJSON = globalSecJSON
	if _, err := db.Connection().UpdateAPIDefinition(definition); err != nil {
		log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to save global security requirements")
	}
}

func persistOpenAPIEndpoints(doc *openapi.Document, definition *db.APIDefinition) []*db.APIEndpoint {
	operations := doc.GetOperations()
	endpoints := make([]*db.APIEndpoint, 0)

	for path, methods := range operations {
		for method, op := range methods {
			endpoint := &db.APIEndpoint{
				DefinitionID: definition.ID,
				OperationID:  op.OperationID,
				Name:         operationName(op, method, path),
				Summary:      op.Summary,
				Description:  op.Description,
				Enabled:      true,
				Method:       strings.ToUpper(method),
				Path:         path,
			}
			endpoints = append(endpoints, endpoint)
		}
	}

	if len(endpoints) == 0 {
		return endpoints
	}

	if err := db.Connection().CreateAPIEndpoints(endpoints); err != nil {
		log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to create API endpoints")
	}
	if err := db.Connection().UpdateAPIDefinitionEndpointCount(definition.ID); err != nil {
		log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to update endpoint count")
	}

	return endpoints
}

func persistEndpointSecurity(doc *openapi.Document, definition *db.APIDefinition, endpoints []*db.APIEndpoint) {
	operations := doc.GetOperations()
	globalSecurity := doc.GetGlobalSecurityRequirements()

	var endpointSecurities []*db.APIEndpointSecurity
	for _, endpoint := range endpoints {
		if endpoint.ID.String() == "00000000-0000-0000-0000-000000000000" {
			continue
		}
		op := FindOperation(operations, endpoint.Path, strings.ToLower(endpoint.Method))
		if op == nil {
			continue
		}
		secReqs, hasOverride := doc.GetOperationSecurityRequirements(op)
		if !hasOverride {
			secReqs = globalSecurity
		}
		for _, req := range secReqs {
			for _, schemeRef := range req.Schemes {
				scopesStr := strings.Join(schemeRef.Scopes, ",")
				endpointSecurities = append(endpointSecurities, &db.APIEndpointSecurity{
					EndpointID: endpoint.ID,
					SchemeName: schemeRef.Name,
					Scopes:     scopesStr,
				})
			}
		}
	}

	if len(endpointSecurities) > 0 {
		if err := db.Connection().CreateAPIEndpointSecurities(endpointSecurities); err != nil {
			log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to create endpoint security schemes")
		}
	}
}

func importGraphQLDefinition(content []byte, sourceURL string, opts ImportOptions) (*db.APIDefinition, error) {
	parser := graphql.NewParser()
	schema, err := parser.ParseFromJSON(content)
	if err != nil {
		return nil, err
	}

	baseURL := opts.BaseURL
	if baseURL == "" && sourceURL != "" {
		baseURL = sourceURL
	}

	name := opts.Name
	if name == "" {
		name = "GraphQL - " + baseURL
	}

	queryCount := len(schema.Queries)
	mutationCount := len(schema.Mutations)
	subscriptionCount := len(schema.Subscriptions)
	typeCount := len(schema.Types)

	definition := &db.APIDefinition{
		WorkspaceID:              opts.WorkspaceID,
		Name:                     name,
		Type:                     db.APIDefinitionTypeGraphQL,
		Status:                   db.APIDefinitionStatusParsed,
		SourceURL:                sourceURL,
		BaseURL:                  baseURL,
		RawDefinition:            content,
		AutoDiscovered:           false,
		AuthConfigID:             opts.AuthConfigID,
		GraphQLQueryCount:        queryCount,
		GraphQLMutationCount:     mutationCount,
		GraphQLSubscriptionCount: subscriptionCount,
		GraphQLTypeCount:         typeCount,
	}

	definition, err = db.Connection().CreateAPIDefinition(definition)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*db.APIEndpoint, 0)

	for _, query := range schema.Queries {
		endpoints = append(endpoints, &db.APIEndpoint{
			DefinitionID:  definition.ID,
			OperationID:   query.Name,
			Name:          query.Name,
			Summary:       query.Description,
			Description:   query.Description,
			Enabled:       true,
			Method:        "POST",
			Path:          "",
			OperationType: "query",
			ReturnType:    query.ReturnType.Name,
		})
	}

	for _, mutation := range schema.Mutations {
		endpoints = append(endpoints, &db.APIEndpoint{
			DefinitionID:  definition.ID,
			OperationID:   mutation.Name,
			Name:          mutation.Name,
			Summary:       mutation.Description,
			Description:   mutation.Description,
			Enabled:       true,
			Method:        "POST",
			Path:          "",
			OperationType: "mutation",
			ReturnType:    mutation.ReturnType.Name,
		})
	}

	for _, subscription := range schema.Subscriptions {
		endpoints = append(endpoints, &db.APIEndpoint{
			DefinitionID:  definition.ID,
			OperationID:   subscription.Name,
			Name:          subscription.Name,
			Summary:       subscription.Description,
			Description:   subscription.Description,
			Enabled:       true,
			Method:        "POST",
			Path:          "",
			OperationType: "subscription",
			ReturnType:    subscription.ReturnType.Name,
		})
	}

	if len(endpoints) > 0 {
		if err := db.Connection().CreateAPIEndpoints(endpoints); err != nil {
			log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to create GraphQL endpoints")
		}
		if err := db.Connection().UpdateAPIDefinitionEndpointCount(definition.ID); err != nil {
			log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to update endpoint count")
		}
	}

	definition, _ = db.Connection().GetAPIDefinitionByIDWithEndpoints(definition.ID)
	return definition, nil
}

func operationName(op interface{}, method, path string) string {
	type operationWithID interface {
		GetOperationID() string
	}

	if o, ok := op.(operationWithID); ok && o.GetOperationID() != "" {
		return o.GetOperationID()
	}

	cleanPath := strings.ReplaceAll(path, "/", "_")
	cleanPath = strings.ReplaceAll(cleanPath, "{", "")
	cleanPath = strings.ReplaceAll(cleanPath, "}", "")
	cleanPath = strings.Trim(cleanPath, "_")

	return strings.ToUpper(method) + "_" + cleanPath
}

func FindOperation(operations map[string]map[string]*openapi3.Operation, path, method string) *openapi3.Operation {
	if methods, ok := operations[path]; ok {
		if op, ok := methods[method]; ok {
			return op
		}
	}
	return nil
}

func DeriveBaseURLFromSpecURL(specURL string) string {
	if specURL == "" {
		return ""
	}

	lastSlash := strings.LastIndex(specURL, "/")
	if lastSlash == -1 {
		return specURL
	}

	afterSlash := specURL[lastSlash+1:]
	if strings.Contains(afterSlash, ".") {
		baseURL := specURL[:lastSlash]
		if strings.HasSuffix(baseURL, ":") {
			return specURL
		}
		return baseURL
	}

	return specURL
}
