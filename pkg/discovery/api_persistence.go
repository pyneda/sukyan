package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/graphql"
	"github.com/pyneda/sukyan/pkg/http_utils"
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
		baseURL, _ = lib.GetBaseURL(history.URL)
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

	definition.EndpointCount = len(endpoints)

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

	baseURL, _ := lib.GetBaseURL(history.URL)
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

	definition.EndpointCount = len(endpoints)

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

	baseURL, _ := lib.GetBaseURL(history.URL)

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

	definition.EndpointCount = len(endpoints)

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

type APIPersistenceFromContentOptions struct {
	WorkspaceID  uint
	ScanID       *uint
	SourceURL    string
	Name         string
	BaseURL      string
	AuthConfigID *uuid.UUID
}

func PersistAPIDefinitionFromContent(content []byte, apiType db.APIDefinitionType, opts APIPersistenceFromContentOptions) (*db.APIDefinition, error) {
	requestURL := &url.URL{Path: "/"}
	if opts.SourceURL != "" {
		if u, err := url.Parse(opts.SourceURL); err == nil {
			requestURL = u
		}
	}

	syntheticResp := &http.Response{
		StatusCode: 200,
		Proto:      "HTTP/1.1",
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(content)),
		Request:    &http.Request{Method: "GET", URL: requestURL},
	}

	history, err := http_utils.ReadHttpResponseAndCreateHistory(syntheticResp, http_utils.HistoryCreationOptions{
		Source:      db.SourceScanner,
		WorkspaceID: opts.WorkspaceID,
	})
	if err != nil {
		return nil, fmt.Errorf("creating history from content: %w", err)
	}

	persistOpts := APIPersistenceOptions{
		WorkspaceID: opts.WorkspaceID,
		ScanID:      opts.ScanID,
	}

	var definition *db.APIDefinition

	switch apiType {
	case db.APIDefinitionTypeOpenAPI:
		definition, err = PersistOpenAPIDefinition(history, persistOpts)
	case db.APIDefinitionTypeGraphQL:
		definition, err = PersistGraphQLDefinition(history, persistOpts)
	case db.APIDefinitionTypeWSDL:
		definition, err = PersistWSDLDefinition(history, persistOpts)
	default:
		return nil, fmt.Errorf("unsupported API type: %s", apiType)
	}

	if err != nil {
		return nil, err
	}

	updates := make(map[string]interface{})
	if opts.Name != "" {
		updates["name"] = opts.Name
	}
	if opts.BaseURL != "" {
		updates["base_url"] = opts.BaseURL
	}
	if opts.AuthConfigID != nil {
		updates["auth_config_id"] = opts.AuthConfigID
	}
	if len(updates) > 0 {
		if opts.Name != "" {
			definition.Name = opts.Name
		}
		if opts.BaseURL != "" {
			definition.BaseURL = opts.BaseURL
		}
		if opts.AuthConfigID != nil {
			definition.AuthConfigID = opts.AuthConfigID
		}
		db.Connection().UpdateAPIDefinition(definition)
	}

	return definition, nil
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
