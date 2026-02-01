package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	pkgapi "github.com/pyneda/sukyan/pkg/api"
	apicore "github.com/pyneda/sukyan/pkg/api/core"
	apigraphql "github.com/pyneda/sukyan/pkg/api/graphql"
	apiopenapi "github.com/pyneda/sukyan/pkg/api/openapi"
	apisoap "github.com/pyneda/sukyan/pkg/api/soap"
	"github.com/pyneda/sukyan/pkg/graphql"
	"github.com/pyneda/sukyan/pkg/openapi"
	pkgWsdl "github.com/pyneda/sukyan/pkg/wsdl"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

type APIPersistenceOptions struct {
	WorkspaceID uint
	ScanID      *uint
}

func PersistOpenAPIDefinition(history *db.History, opts APIPersistenceOptions) (*db.APIDefinition, error) {
	body, err := history.ResponseBody()
	if err != nil {
		return nil, err
	}

	doc, err := openapi.Parse(body)
	if err != nil {
		log.Debug().Err(err).Str("url", history.URL).Msg("Failed to parse OpenAPI document for persistence")
		return nil, err
	}

	exists, err := db.Connection().APIDefinitionExistsBySourceURL(opts.WorkspaceID, history.URL)
	if err != nil {
		log.Warn().Err(err).Str("url", history.URL).Msg("Failed to check for existing API definition")
		return nil, fmt.Errorf("checking for existing definition: %w", err)
	}
	if exists {
		log.Debug().Str("url", history.URL).Msg("API definition already exists for this source URL")
		existingDef, err := db.Connection().GetAPIDefinitionBySourceURL(opts.WorkspaceID, history.URL)
		if err != nil {
			return nil, fmt.Errorf("retrieving existing definition: %w", err)
		}
		return existingDef, nil
	}

	var jsonObj map[string]interface{}
	if err := json.Unmarshal(body, &jsonObj); err != nil {
		log.Debug().Err(err).Str("url", history.URL).Msg("Failed to parse JSON for metadata extraction")
	}

	var openapiVersion string
	if v, ok := jsonObj["openapi"].(string); ok {
		openapiVersion = v
	} else if v, ok := jsonObj["swagger"].(string); ok {
		openapiVersion = v
	}

	var openapiTitle string
	if info, ok := jsonObj["info"].(map[string]interface{}); ok {
		if t, ok := info["title"].(string); ok {
			openapiTitle = t
		}
	}

	var serverCount int
	if servers, ok := jsonObj["servers"].([]interface{}); ok {
		serverCount = len(servers)
	}

	baseURL := doc.BaseURL()
	if baseURL == "" {
		baseURL, _ = getBaseURLFromHistory(history)
	}

	name := openapiTitle
	if name == "" {
		name = "OpenAPI - " + baseURL
	}

	historyID := history.ID
	definition := &db.APIDefinition{
		WorkspaceID:     opts.WorkspaceID,
		Name:            name,
		Type:            db.APIDefinitionTypeOpenAPI,
		Status:          db.APIDefinitionStatusParsed,
		SourceURL:       history.URL,
		BaseURL:         baseURL,
		SourceHistoryID: &historyID,
		RawDefinition:   body,
		AutoDiscovered:  opts.ScanID != nil,
		ScanID:          opts.ScanID,
		OpenAPIVersion:  &openapiVersion,
		OpenAPITitle:    &openapiTitle,
		OpenAPIServers:  serverCount,
	}

	definition, err = db.Connection().CreateAPIDefinition(definition)
	if err != nil {
		return nil, err
	}

	operations := doc.GetOperations()
	endpoints := make([]*db.APIEndpoint, 0)

	for path, methods := range operations {
		for method, op := range methods {
			endpoint := &db.APIEndpoint{
				DefinitionID: definition.ID,
				OperationID:  op.OperationID,
				Name:         getOperationName(op, method, path),
				Summary:      op.Summary,
				Description:  op.Description,
				Enabled:      true,
				Method:       strings.ToUpper(method),
				Path:         path,
			}

			endpoints = append(endpoints, endpoint)
		}
	}

	txErr := db.Connection().DB().Transaction(func(tx *gorm.DB) error {
		specSchemes := doc.GetSecuritySchemes()
		if len(specSchemes) > 0 {
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
			if err := tx.Create(dbSchemes).Error; err != nil {
				return fmt.Errorf("creating security schemes: %w", err)
			}
		}

		globalSecurity := doc.GetGlobalSecurityRequirements()
		if len(globalSecurity) > 0 {
			if globalSecJSON, marshalErr := json.Marshal(globalSecurity); marshalErr == nil {
				definition.GlobalSecurityJSON = globalSecJSON
				if err := tx.Save(definition).Error; err != nil {
					return fmt.Errorf("updating global security requirements: %w", err)
				}
			}
		}

		if len(endpoints) > 0 {
			if err := tx.Create(endpoints).Error; err != nil {
				return fmt.Errorf("creating endpoints: %w", err)
			}

			var reloadedEndpoints []*db.APIEndpoint
			if err := tx.Where("definition_id = ?", definition.ID).Find(&reloadedEndpoints).Error; err != nil {
				return fmt.Errorf("reloading endpoints: %w", err)
			}

			allParams := collectOpenAPIEndpointParameters(operations, reloadedEndpoints)
			if len(allParams) > 0 {
				if err := tx.Create(allParams).Error; err != nil {
					return fmt.Errorf("creating endpoint parameters: %w", err)
				}
			}

			variations := buildRequestVariations(context.Background(), definition, reloadedEndpoints)
			if len(variations) > 0 {
				if err := tx.Create(variations).Error; err != nil {
					return fmt.Errorf("creating request variations: %w", err)
				}
			}

			var endpointSecurities []*db.APIEndpointSecurity
			for _, endpoint := range reloadedEndpoints {
				methods, ok := operations[endpoint.Path]
				if !ok {
					continue
				}
				op, ok := methods[strings.ToLower(endpoint.Method)]
				if !ok || op.Security == nil {
					continue
				}
				for _, secReq := range *op.Security {
					for schemeName, scopes := range secReq {
						endpointSecurities = append(endpointSecurities, &db.APIEndpointSecurity{
							EndpointID: endpoint.ID,
							SchemeName: schemeName,
							Scopes:     strings.Join(scopes, ","),
						})
					}
				}
			}
			if len(endpointSecurities) > 0 {
				if err := tx.Create(endpointSecurities).Error; err != nil {
					return fmt.Errorf("creating endpoint security requirements: %w", err)
				}
			}

			var count int64
			if err := tx.Model(&db.APIEndpoint{}).Where("definition_id = ?", definition.ID).Count(&count).Error; err != nil {
				return fmt.Errorf("counting endpoints: %w", err)
			}
			if err := tx.Model(&db.APIDefinition{}).Where("id = ?", definition.ID).Update("endpoint_count", count).Error; err != nil {
				return fmt.Errorf("updating endpoint count: %w", err)
			}
		}

		return nil
	})
	if txErr != nil {
		log.Warn().Err(txErr).Str("definition_id", definition.ID.String()).Msg("Failed to persist OpenAPI definition child records")
	}

	log.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Int("endpoints", len(endpoints)).
		Str("source_url", history.URL).
		Msg("Persisted discovered OpenAPI definition")

	return definition, nil
}

func getOperationName(op interface{}, method, path string) string {
	type operationWithID interface {
		GetOperationID() string
	}

	if o, ok := op.(operationWithID); ok && o.GetOperationID() != "" {
		return o.GetOperationID()
	}

	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "{", "")
	path = strings.ReplaceAll(path, "}", "")
	path = strings.Trim(path, "_")

	return strings.ToUpper(method) + "_" + path
}

func getBaseURLFromHistory(history *db.History) (string, error) {
	return lib.GetBaseURL(history.URL)
}

func PersistGraphQLDefinition(history *db.History, opts APIPersistenceOptions) (*db.APIDefinition, error) {
	body, err := history.ResponseBody()
	if err != nil {
		return nil, err
	}

	parser := graphql.NewParser()
	schema, err := parser.ParseFromJSON(body)
	if err != nil {
		log.Debug().Err(err).Str("url", history.URL).Msg("Failed to parse GraphQL schema for persistence")
		return nil, err
	}

	exists, err := db.Connection().APIDefinitionExistsBySourceURL(opts.WorkspaceID, history.URL)
	if err != nil {
		log.Warn().Err(err).Str("url", history.URL).Msg("Failed to check for existing API definition")
		return nil, fmt.Errorf("checking for existing definition: %w", err)
	}
	if exists {
		log.Debug().Str("url", history.URL).Msg("GraphQL definition already exists for this source URL")
		existingDef, err := db.Connection().GetAPIDefinitionBySourceURL(opts.WorkspaceID, history.URL)
		if err != nil {
			return nil, fmt.Errorf("retrieving existing definition: %w", err)
		}
		return existingDef, nil
	}

	baseURL, _ := getBaseURLFromHistory(history)
	name := "GraphQL - " + baseURL

	historyID := history.ID
	queryCount := len(schema.Queries)
	mutationCount := len(schema.Mutations)
	subscriptionCount := len(schema.Subscriptions)
	typeCount := len(schema.Types)

	definition := &db.APIDefinition{
		WorkspaceID:              opts.WorkspaceID,
		Name:                     name,
		Type:                     db.APIDefinitionTypeGraphQL,
		Status:                   db.APIDefinitionStatusParsed,
		SourceURL:                history.URL,
		BaseURL:                  baseURL,
		SourceHistoryID:          &historyID,
		RawDefinition:            body,
		AutoDiscovered:           opts.ScanID != nil,
		ScanID:                   opts.ScanID,
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
		endpoint := &db.APIEndpoint{
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
		}
		endpoints = append(endpoints, endpoint)
	}

	for _, mutation := range schema.Mutations {
		endpoint := &db.APIEndpoint{
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
		}
		endpoints = append(endpoints, endpoint)
	}

	for _, subscription := range schema.Subscriptions {
		endpoint := &db.APIEndpoint{
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
		}
		endpoints = append(endpoints, endpoint)
	}

	txErr := db.Connection().DB().Transaction(func(tx *gorm.DB) error {
		if len(endpoints) > 0 {
			if err := tx.Create(endpoints).Error; err != nil {
				return fmt.Errorf("creating endpoints: %w", err)
			}

			var reloadedEndpoints []*db.APIEndpoint
			if err := tx.Where("definition_id = ?", definition.ID).Find(&reloadedEndpoints).Error; err != nil {
				return fmt.Errorf("reloading endpoints: %w", err)
			}

			allParams := collectGraphQLEndpointParameters(schema, reloadedEndpoints)
			if len(allParams) > 0 {
				if err := tx.Create(allParams).Error; err != nil {
					return fmt.Errorf("creating endpoint parameters: %w", err)
				}
			}

			variations := buildRequestVariations(context.Background(), definition, reloadedEndpoints)
			if len(variations) > 0 {
				if err := tx.Create(variations).Error; err != nil {
					return fmt.Errorf("creating request variations: %w", err)
				}
			}

			var count int64
			if err := tx.Model(&db.APIEndpoint{}).Where("definition_id = ?", definition.ID).Count(&count).Error; err != nil {
				return fmt.Errorf("counting endpoints: %w", err)
			}
			if err := tx.Model(&db.APIDefinition{}).Where("id = ?", definition.ID).Update("endpoint_count", count).Error; err != nil {
				return fmt.Errorf("updating endpoint count: %w", err)
			}
		}

		return nil
	})
	if txErr != nil {
		log.Warn().Err(txErr).Str("definition_id", definition.ID.String()).Msg("Failed to persist GraphQL definition child records")
	}

	log.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Int("queries", queryCount).
		Int("mutations", mutationCount).
		Int("subscriptions", subscriptionCount).
		Str("source_url", history.URL).
		Msg("Persisted discovered GraphQL definition")

	return definition, nil
}

func collectOpenAPIEndpointParameters(operations map[string]map[string]*openapi3.Operation, endpoints []*db.APIEndpoint) []*db.APIEndpointParameter {
	var allParams []*db.APIEndpointParameter

	for _, endpoint := range endpoints {
		methods, ok := operations[endpoint.Path]
		if !ok {
			continue
		}
		op, ok := methods[strings.ToLower(endpoint.Method)]
		if !ok {
			continue
		}
		params := extractOpenAPIParameters(op, endpoint.ID)
		allParams = append(allParams, params...)
	}

	return allParams
}

func collectGraphQLEndpointParameters(schema *graphql.GraphQLSchema, endpoints []*db.APIEndpoint) []*db.APIEndpointParameter {
	var allParams []*db.APIEndpointParameter

	allOps := make(map[string]graphql.Operation)
	for _, q := range schema.Queries {
		allOps[q.Name] = q
	}
	for _, m := range schema.Mutations {
		allOps[m.Name] = m
	}
	for _, s := range schema.Subscriptions {
		allOps[s.Name] = s
	}

	for _, endpoint := range endpoints {
		op, ok := allOps[endpoint.OperationID]
		if !ok {
			continue
		}
		for _, arg := range op.Arguments {
			dbParam := &db.APIEndpointParameter{
				EndpointID: endpoint.ID,
				Name:       arg.Name,
				Location:   db.APIParamLocationBody,
				Required:   arg.Type.Required,
				DataType:   mapGraphQLTypeToString(arg.Type),
			}
			if arg.DefaultValue != nil {
				dbParam.DefaultValue = fmt.Sprintf("%v", arg.DefaultValue)
			}
			allParams = append(allParams, dbParam)
		}
	}

	return allParams
}

func mapGraphQLTypeToString(typeRef graphql.TypeRef) string {
	baseName := getGraphQLBaseTypeName(typeRef)

	switch baseName {
	case "String", "ID":
		return "string"
	case "Int":
		return "integer"
	case "Float":
		return "number"
	case "Boolean":
		return "boolean"
	default:
		if typeRef.IsList {
			return "array"
		}
		if typeRef.Kind == graphql.TypeKindInputObject {
			return "object"
		}
		return "string"
	}
}

func getGraphQLBaseTypeName(typeRef graphql.TypeRef) string {
	if typeRef.Name != "" {
		return typeRef.Name
	}
	if typeRef.OfType != nil {
		return getGraphQLBaseTypeName(*typeRef.OfType)
	}
	return ""
}

func extractOpenAPIParameters(op *openapi3.Operation, endpointID uuid.UUID) []*db.APIEndpointParameter {
	var params []*db.APIEndpointParameter

	for _, paramRef := range op.Parameters {
		if paramRef.Value == nil {
			continue
		}
		p := paramRef.Value
		dbParam := &db.APIEndpointParameter{
			EndpointID: endpointID,
			Name:       p.Name,
			Location:   mapOpenAPIParamLocation(p.In),
			Required:   p.Required,
		}
		if p.Schema != nil && p.Schema.Value != nil {
			populateSchemaFields(p.Schema.Value, dbParam)
		}
		params = append(params, dbParam)
	}

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		for _, mediaType := range op.RequestBody.Value.Content {
			if mediaType.Schema == nil || mediaType.Schema.Value == nil {
				continue
			}
			schema := mediaType.Schema.Value

			if schema.Type != nil && len(schema.Type.Slice()) > 0 && schema.Type.Slice()[0] == "object" {
				for propName, propRef := range schema.Properties {
					if propRef.Value == nil {
						continue
					}
					dbParam := &db.APIEndpointParameter{
						EndpointID: endpointID,
						Name:       propName,
						Location:   db.APIParamLocationBody,
						Required:   isPropertyRequired(propName, schema.Required),
					}
					populateSchemaFields(propRef.Value, dbParam)
					params = append(params, dbParam)
				}
			} else {
				dbParam := &db.APIEndpointParameter{
					EndpointID: endpointID,
					Name:       "body",
					Location:   db.APIParamLocationBody,
					Required:   op.RequestBody.Value.Required,
				}
				populateSchemaFields(schema, dbParam)
				params = append(params, dbParam)
			}
			break
		}
	}

	return params
}

func populateSchemaFields(schema *openapi3.Schema, param *db.APIEndpointParameter) {
	if schema.Type != nil && len(schema.Type.Slice()) > 0 {
		param.DataType = schema.Type.Slice()[0]
	}
	param.Format = schema.Format
	param.Pattern = schema.Pattern

	if schema.MinLength != 0 {
		minLen := int(schema.MinLength)
		param.MinLength = &minLen
	}
	if schema.MaxLength != nil {
		maxLen := int(*schema.MaxLength)
		param.MaxLength = &maxLen
	}
	if schema.Min != nil {
		param.Minimum = schema.Min
	}
	if schema.Max != nil {
		param.Maximum = schema.Max
	}

	if len(schema.Enum) > 0 {
		b, err := json.Marshal(schema.Enum)
		if err == nil {
			param.EnumValues = string(b)
		}
	}

	param.DefaultValue = stringifyValue(schema.Default)
	param.Example = stringifyValue(schema.Example)
}

func mapOpenAPIParamLocation(in string) db.APIParameterLocation {
	switch in {
	case "path":
		return db.APIParamLocationPath
	case "query":
		return db.APIParamLocationQuery
	case "header":
		return db.APIParamLocationHeader
	case "cookie":
		return db.APIParamLocationCookie
	case "body":
		return db.APIParamLocationBody
	default:
		return db.APIParamLocationQuery
	}
}

func isPropertyRequired(propName string, required []string) bool {
	for _, r := range required {
		if r == propName {
			return true
		}
	}
	return false
}

func stringifyValue(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64, float32, int, int64, int32, bool:
		return fmt.Sprintf("%v", val)
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

func buildRequestVariations(ctx context.Context, definition *db.APIDefinition, endpoints []*db.APIEndpoint) []*db.APIRequestVariation {
	var operations []apicore.Operation
	var err error

	switch definition.Type {
	case db.APIDefinitionTypeOpenAPI:
		parser := apiopenapi.NewParser()
		operations, err = parser.Parse(definition)
	case db.APIDefinitionTypeGraphQL:
		parser := apigraphql.NewParser()
		operations, err = parser.Parse(definition)
	case db.APIDefinitionTypeWSDL:
		parser := apisoap.NewParser()
		operations, err = parser.Parse(definition)
	default:
		log.Warn().Str("type", string(definition.Type)).Msg("Unsupported definition type for request variation generation")
		return nil
	}

	if err != nil {
		log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to parse definition for request variation generation")
		return nil
	}

	opByID := make(map[string]*apicore.Operation)
	opByPathMethod := make(map[string]*apicore.Operation)
	for i := range operations {
		op := &operations[i]
		if op.OperationID != "" {
			opByID[op.OperationID] = op
		}
		if op.Name != "" && op.Name != op.OperationID {
			opByID[op.Name] = op
		}
		key := strings.ToUpper(op.Method) + ":" + op.Path
		opByPathMethod[key] = op
	}

	var variations []*db.APIRequestVariation

	for _, endpoint := range endpoints {
		var matchedOp *apicore.Operation

		if endpoint.OperationID != "" {
			matchedOp = opByID[endpoint.OperationID]
		}
		if matchedOp == nil {
			key := strings.ToUpper(endpoint.Method) + ":" + endpoint.Path
			matchedOp = opByPathMethod[key]
		}

		if matchedOp == nil {
			log.Debug().
				Str("endpoint_id", endpoint.ID.String()).
				Str("path", endpoint.Path).
				Str("method", endpoint.Method).
				Msg("No matching operation found for request variation generation")
			continue
		}

		req, buildErr := pkgapi.BuildDefaultRequest(ctx, definition.Type, matchedOp)
		if buildErr != nil {
			log.Warn().Err(buildErr).
				Str("endpoint_id", endpoint.ID.String()).
				Msg("Failed to build request for variation generation")
			continue
		}

		variation, serErr := serializeRequestToVariation(endpoint.ID, req, definition.Type)
		if serErr != nil {
			log.Warn().Err(serErr).
				Str("endpoint_id", endpoint.ID.String()).
				Msg("Failed to serialize request to variation")
			continue
		}

		variations = append(variations, variation)
	}

	return variations
}

func serializeRequestToVariation(endpointID uuid.UUID, req *http.Request, defType db.APIDefinitionType) (*db.APIRequestVariation, error) {
	variation := &db.APIRequestVariation{
		EndpointID: endpointID,
		Label:      "Happy Path",
		URL:        req.URL.String(),
		Method:     req.Method,
	}

	filteredHeaders := make(http.Header)
	for name, values := range req.Header {
		lower := strings.ToLower(name)
		if lower == "authorization" || lower == "proxy-authorization" || lower == "cookie" {
			continue
		}
		filteredHeaders[name] = values
	}
	if len(filteredHeaders) > 0 {
		headersJSON, err := json.Marshal(filteredHeaders)
		if err == nil {
			variation.Headers = headersJSON
		}
	}

	variation.ContentType = req.Header.Get("Content-Type")

	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("reading request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		variation.Body = bodyBytes

		if defType == db.APIDefinitionTypeGraphQL && len(bodyBytes) > 0 {
			var gqlReq struct {
				Query         string         `json:"query"`
				Variables     map[string]any `json:"variables"`
				OperationName string         `json:"operationName"`
			}
			if err := json.Unmarshal(bodyBytes, &gqlReq); err != nil {
				log.Warn().Err(err).
					Str("endpoint_id", endpointID.String()).
					Msg("Failed to parse GraphQL request body for variation metadata")
			} else {
				variation.Query = gqlReq.Query
				variation.OperationName = gqlReq.OperationName
				if len(gqlReq.Variables) > 0 {
					varsJSON, marshalErr := json.Marshal(gqlReq.Variables)
					if marshalErr != nil {
						log.Warn().Err(marshalErr).
							Str("endpoint_id", endpointID.String()).
							Msg("Failed to marshal GraphQL variables")
					} else {
						variation.Variables = varsJSON
					}
				}
			}
		}
	}

	return variation, nil
}

func PersistWSDLDefinition(history *db.History, opts APIPersistenceOptions) (*db.APIDefinition, error) {
	body, err := history.ResponseBody()
	if err != nil {
		return nil, err
	}

	parser := pkgWsdl.NewParser()
	wsdlDoc, err := parser.ParseFromBytes(body, history.URL)
	if err != nil {
		log.Debug().Err(err).Str("url", history.URL).Msg("Failed to parse WSDL document for persistence")
		return nil, err
	}

	exists, err := db.Connection().APIDefinitionExistsBySourceURL(opts.WorkspaceID, history.URL)
	if err != nil {
		log.Warn().Err(err).Str("url", history.URL).Msg("Failed to check for existing API definition")
		return nil, fmt.Errorf("checking for existing definition: %w", err)
	}
	if exists {
		log.Debug().Str("url", history.URL).Msg("WSDL definition already exists for this source URL")
		existingDef, err := db.Connection().GetAPIDefinitionBySourceURL(opts.WorkspaceID, history.URL)
		if err != nil {
			return nil, fmt.Errorf("retrieving existing definition: %w", err)
		}
		return existingDef, nil
	}

	baseURL, _ := getBaseURLFromHistory(history)

	var wsdlServiceCount int
	var wsdlPortCount int
	var wsdlOperationCount int
	var detectedSOAPVersion string
	for _, service := range wsdlDoc.Services {
		wsdlServiceCount++
		for _, port := range service.Ports {
			wsdlPortCount++
			if detectedSOAPVersion == "" && port.SOAPVersion != "" {
				detectedSOAPVersion = port.SOAPVersion
			}
			binding := findWSDLBinding(wsdlDoc, port.Binding)
			if binding != nil {
				wsdlOperationCount += len(binding.Operations)
				if detectedSOAPVersion == "" && binding.SOAPVersion != "" {
					detectedSOAPVersion = binding.SOAPVersion
				}
			}
		}
	}

	name := "WSDL"
	if wsdlDoc.Name != "" {
		name = "WSDL - " + wsdlDoc.Name
	} else if len(wsdlDoc.Services) > 0 {
		name = "WSDL - " + wsdlDoc.Services[0].Name
	} else {
		name = "WSDL - " + baseURL
	}

	historyID := history.ID
	var wsdlTargetNamespace *string
	if wsdlDoc.TargetNamespace != "" {
		wsdlTargetNamespace = &wsdlDoc.TargetNamespace
	}
	var wsdlSOAPVersion *string
	if detectedSOAPVersion != "" {
		wsdlSOAPVersion = &detectedSOAPVersion
	}

	definition := &db.APIDefinition{
		WorkspaceID:         opts.WorkspaceID,
		Name:                name,
		Type:                db.APIDefinitionTypeWSDL,
		Status:              db.APIDefinitionStatusParsed,
		SourceURL:           history.URL,
		BaseURL:             baseURL,
		SourceHistoryID:     &historyID,
		RawDefinition:       body,
		AutoDiscovered:      opts.ScanID != nil,
		ScanID:              opts.ScanID,
		WSDLTargetNamespace: wsdlTargetNamespace,
		WSDLServiceCount:    wsdlServiceCount,
		WSDLPortCount:       wsdlPortCount,
		WSDLSOAPVersion:     wsdlSOAPVersion,
	}

	definition, err = db.Connection().CreateAPIDefinition(definition)
	if err != nil {
		return nil, err
	}

	endpoints := make([]*db.APIEndpoint, 0)

	for _, service := range wsdlDoc.Services {
		for _, port := range service.Ports {
			binding := findWSDLBinding(wsdlDoc, port.Binding)
			if binding == nil {
				continue
			}

			for _, bindingOp := range binding.Operations {
				endpoint := &db.APIEndpoint{
					DefinitionID:  definition.ID,
					OperationID:   bindingOp.Name,
					Name:          bindingOp.Name,
					Summary:       service.Name + " - " + bindingOp.Name,
					Enabled:       true,
					Method:        "POST",
					Path:          "",
					OperationType: "soap",
					SOAPAction:    bindingOp.SOAPAction,
				}
				endpoints = append(endpoints, endpoint)
			}
		}
	}

	txErr := db.Connection().DB().Transaction(func(tx *gorm.DB) error {
		if len(endpoints) > 0 {
			if err := tx.Create(endpoints).Error; err != nil {
				return fmt.Errorf("creating endpoints: %w", err)
			}

			var reloadedEndpoints []*db.APIEndpoint
			if err := tx.Where("definition_id = ?", definition.ID).Find(&reloadedEndpoints).Error; err != nil {
				return fmt.Errorf("reloading endpoints: %w", err)
			}

			allParams := collectWSDLEndpointParameters(definition, reloadedEndpoints)
			if len(allParams) > 0 {
				if err := tx.Create(allParams).Error; err != nil {
					return fmt.Errorf("creating endpoint parameters: %w", err)
				}
			}

			variations := buildRequestVariations(context.Background(), definition, reloadedEndpoints)
			if len(variations) > 0 {
				if err := tx.Create(variations).Error; err != nil {
					return fmt.Errorf("creating request variations: %w", err)
				}
			}

			var count int64
			if err := tx.Model(&db.APIEndpoint{}).Where("definition_id = ?", definition.ID).Count(&count).Error; err != nil {
				return fmt.Errorf("counting endpoints: %w", err)
			}
			if err := tx.Model(&db.APIDefinition{}).Where("id = ?", definition.ID).Update("endpoint_count", count).Error; err != nil {
				return fmt.Errorf("updating endpoint count: %w", err)
			}
		}

		return nil
	})
	if txErr != nil {
		log.Warn().Err(txErr).Str("definition_id", definition.ID.String()).Msg("Failed to persist WSDL definition child records")
	}

	log.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Int("services", wsdlServiceCount).
		Int("operations", wsdlOperationCount).
		Int("endpoints", len(endpoints)).
		Str("source_url", history.URL).
		Msg("Persisted discovered WSDL definition")

	return definition, nil
}

func findWSDLBinding(doc *pkgWsdl.WSDLDocument, bindingName string) *pkgWsdl.Binding {
	localName := extractWSDLLocalName(bindingName)
	for i := range doc.Bindings {
		if doc.Bindings[i].Name == localName || doc.Bindings[i].Name == bindingName {
			return &doc.Bindings[i]
		}
	}
	return nil
}

func extractWSDLLocalName(qname string) string {
	for i := len(qname) - 1; i >= 0; i-- {
		if qname[i] == ':' {
			return qname[i+1:]
		}
	}
	return qname
}

func collectWSDLEndpointParameters(definition *db.APIDefinition, endpoints []*db.APIEndpoint) []*db.APIEndpointParameter {
	soapParser := apisoap.NewParser()
	operations, err := soapParser.Parse(definition)
	if err != nil {
		log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to parse WSDL for parameter extraction")
		return nil
	}

	opByName := make(map[string]*apicore.Operation)
	for i := range operations {
		opByName[operations[i].Name] = &operations[i]
	}

	var allParams []*db.APIEndpointParameter
	for _, endpoint := range endpoints {
		op, ok := opByName[endpoint.OperationID]
		if !ok {
			continue
		}
		for _, param := range op.Parameters {
			dbParam := &db.APIEndpointParameter{
				EndpointID: endpoint.ID,
				Name:       param.Name,
				Location:   db.APIParamLocationBody,
				Required:   param.Required,
				DataType:   string(param.DataType),
			}
			if param.DefaultValue != nil {
				dbParam.DefaultValue = fmt.Sprintf("%v", param.DefaultValue)
			}
			allParams = append(allParams, dbParam)
		}
	}

	return allParams
}

type APIPersistFunc func(*db.History, APIPersistenceOptions) (*db.APIDefinition, error)

func persistDiscoveredAPIDefinitions(results DiscoverAndCreateIssueResults, options DiscoveryOptions, validationFunc ValidationFunc, persistFunc APIPersistFunc, apiType string) {
	if len(results.Issues) == 0 {
		return
	}
	for _, history := range results.Responses {
		validationCtx := &ValidationContext{SiteBehavior: options.SiteBehavior}
		if valid, _, _ := validationFunc(history, validationCtx); !valid {
			continue
		}
		persistOpts := APIPersistenceOptions{
			WorkspaceID: options.HistoryCreationOptions.WorkspaceID,
		}
		if options.HistoryCreationOptions.ScanID > 0 {
			scanID := options.HistoryCreationOptions.ScanID
			persistOpts.ScanID = &scanID
		}
		_, persistErr := persistFunc(history, persistOpts)
		if persistErr != nil {
			log.Debug().Err(persistErr).Str("url", history.URL).Msgf("Failed to persist %s definition", apiType)
		}
	}
}
