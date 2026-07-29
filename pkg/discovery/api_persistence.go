package discovery

import (
	"bytes"
	"encoding/json"
	"errors"
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

// ErrInvalidDefinitionContent marks a failure caused by the document the caller
// supplied rather than by anything on our side. Import handlers match on it to
// answer 400 instead of 500: a pasted schema we cannot read is the caller's
// problem to fix, and a 500 tells them to file a bug instead.
var ErrInvalidDefinitionContent = errors.New("invalid API definition content")

type APIPersistenceOptions struct {
	WorkspaceID uint
	ScanID      *uint

	// SourceURL, when non-nil, is the URL the definition is stored and identified
	// by, overriding the URL of the history it was parsed from. Pasted content has
	// no source URL at all and passes an empty string: it is stored with none.
	//
	// Storing pasted content under the synthetic request path it was wrapped in
	// ("/") is what made every pasted import in a workspace share one identity —
	// the second paste found the first by source URL and was answered with it.
	SourceURL *string

	// AlwaysCreateNew skips the "this source URL is already in the library" reuse
	// below and stores a new definition instead.
	//
	// Explicit imports set it. An import that reused an existing row updated that
	// row's name from the new document while leaving its operations from the old
	// one, so a re-imported URL renamed a definition the user had already curated
	// and left it describing endpoints that no longer matched its name. Automatic
	// discovery leaves it false: it re-encounters the same spec URL many times in
	// a single scan and must not multiply rows for it.
	AlwaysCreateNew bool
}

// definitionSourceURL resolves the URL a definition is stored under: the override
// when the caller supplied one, otherwise the URL of the response it was parsed
// from.
func (o APIPersistenceOptions) definitionSourceURL(history *db.History) string {
	if o.SourceURL != nil {
		return *o.SourceURL
	}
	return history.URL
}

// existingDefinitionFor returns the definition this persist call should reuse
// instead of creating one, or nil when it must create.
//
// Content with no source URL never matches: rows that share "no source" are not
// the same API, and matching them would collide every pasted import onto one row.
func existingDefinitionFor(opts APIPersistenceOptions, sourceURL string) (*db.APIDefinition, error) {
	if opts.AlwaysCreateNew || sourceURL == "" {
		return nil, nil
	}

	exists, err := db.Connection().APIDefinitionExistsBySourceURL(opts.WorkspaceID, sourceURL)
	if err != nil {
		log.Warn().Err(err).Str("url", sourceURL).Msg("Failed to check for existing API definition")
		return nil, fmt.Errorf("checking for existing definition: %w", err)
	}
	if !exists {
		return nil, nil
	}

	existing, err := db.Connection().GetAPIDefinitionBySourceURL(opts.WorkspaceID, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("retrieving existing definition: %w", err)
	}
	return existing, nil
}

// storeEndpointsWithCount inserts a definition's parsed endpoints and writes the
// resulting endpoint_count in the same transaction, then mirrors the stored value
// onto the in-memory definition.
//
// The count is written unconditionally, including when the parse produced no
// endpoints: it is the definition's only summary of how much surface it covers,
// and a row whose counter disagrees with its api_endpoints rows makes every
// consumer either wrong or forced to re-count by fetching the endpoint list.
// The count is read back from the table rather than taken from len(endpoints) so
// it also reflects rows a retry or a partially applied batch already left behind.
//
// Mirroring onto the struct matters as much as the write: callers hand the same
// struct back to the database afterwards (a named import re-saves it to apply the
// user's name), so a struct carrying a stale zero would undo the row's counter.
func storeEndpointsWithCount(tx *gorm.DB, definition *db.APIDefinition, endpoints []*db.APIEndpoint) error {
	if len(endpoints) > 0 {
		if err := tx.Create(endpoints).Error; err != nil {
			return fmt.Errorf("creating endpoints: %w", err)
		}
	}

	var count int64
	if err := tx.Model(&db.APIEndpoint{}).Where("definition_id = ?", definition.ID).Count(&count).Error; err != nil {
		return fmt.Errorf("counting endpoints: %w", err)
	}

	if err := tx.Model(&db.APIDefinition{}).Where("id = ?", definition.ID).Update("endpoint_count", count).Error; err != nil {
		return fmt.Errorf("updating endpoint count: %w", err)
	}

	definition.EndpointCount = int(count)
	return nil
}

// definitionBaseURL derives the origin a definition's requests are sent to from
// the URL it was imported from.
//
// It yields nothing when there is no usable origin. Pasted content has no source
// URL at all, and lib.GetBaseURL renders that as "://" — a string no HTTP client
// can request, which would then be stored as the definition's base URL and used
// to build every probe.
func definitionBaseURL(sourceURL string) string {
	if sourceURL == "" {
		return ""
	}

	baseURL, err := lib.GetBaseURL(sourceURL)
	if err != nil {
		return ""
	}

	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return baseURL
}

func PersistOpenAPIDefinition(history *db.History, opts APIPersistenceOptions) (*db.APIDefinition, error) {
	body, err := history.ResponseBody()
	if err != nil {
		return nil, err
	}

	sourceURL := opts.definitionSourceURL(history)

	doc, err := openapi.ParseWithOptions(body, openapi.ParseOptions{SourceURL: sourceURL})
	if err != nil {
		log.Debug().Err(err).Str("url", sourceURL).Msg("Failed to parse OpenAPI document for persistence")
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinitionContent, err)
	}

	existingDef, err := existingDefinitionFor(opts, sourceURL)
	if err != nil {
		return nil, err
	}
	if existingDef != nil {
		log.Debug().Str("url", sourceURL).Msg("API definition already exists for this source URL")
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

	// Document.BaseURL already falls back to the source URL's origin and refuses a
	// scheme no HTTP client can request, so a second fallback here only reinstates
	// what it rejected: a spec read from disk yields "file://", which every request
	// build then rejects as unusable. Leaving it empty keeps the definition honest.
	baseURL := doc.BaseURL()

	name := openapiTitle
	if name == "" {
		source := baseURL
		if source == "" {
			source = sourceURL
		}
		// Pasted content with no servers has neither: naming it "OpenAPI - " would
		// put a dangling separator in the library.
		name = "OpenAPI"
		if source != "" {
			name = "OpenAPI - " + source
		}
	}

	historyID := history.ID
	definition := &db.APIDefinition{
		WorkspaceID:     opts.WorkspaceID,
		Name:            name,
		Type:            db.APIDefinitionTypeOpenAPI,
		Status:          db.APIDefinitionStatusParsed,
		SourceURL:       sourceURL,
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
			// The endpoints are what the scanner needs, so a rejected security scheme
			// must not take them down with it. Logging the error is not enough:
			// PostgreSQL aborts the whole transaction on a failed statement, so the
			// insert runs inside a savepoint that can be rolled back on its own.
			savepoint := "api_definition_security_schemes"
			if err := tx.SavePoint(savepoint).Error; err != nil {
				log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to create savepoint for OpenAPI security schemes")
			} else if err := tx.Create(dbSchemes).Error; err != nil {
				log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to persist OpenAPI security schemes")
				if rollbackErr := tx.RollbackTo(savepoint).Error; rollbackErr != nil {
					return fmt.Errorf("rolling back security schemes: %w", rollbackErr)
				}
			}
		}

		globalSecurity := doc.GetGlobalSecurityRequirements()
		if len(globalSecurity) > 0 {
			if globalSecJSON, marshalErr := json.Marshal(globalSecurity); marshalErr == nil {
				definition.GlobalSecurityJSON = globalSecJSON
				// One column, not the whole struct: a full save here would write
				// endpoint_count from memory, and the struct does not learn its
				// count until the endpoints below have been inserted.
				if err := tx.Model(&db.APIDefinition{}).Where("id = ?", definition.ID).
					Update("global_security_json", globalSecJSON).Error; err != nil {
					return fmt.Errorf("updating global security requirements: %w", err)
				}
			}
		}

		return storeEndpointsWithCount(tx, definition, endpoints)
	})
	if txErr != nil {
		log.Warn().Err(txErr).Str("definition_id", definition.ID.String()).Msg("Failed to persist OpenAPI definition child records")
		definition.EndpointCount = 0
	}

	log.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Int("endpoints", definition.EndpointCount).
		Str("source_url", sourceURL).
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

	sourceURL := opts.definitionSourceURL(history)

	parser := graphql.NewParser()
	// ParseSchema, not ParseFromJSON: an introspection response is only one of the
	// two shapes a GraphQL schema arrives in, and the UI offers to paste the other.
	schema, err := parser.ParseSchema(body)
	if err != nil {
		log.Debug().Err(err).Str("url", sourceURL).Msg("Failed to parse GraphQL schema for persistence")
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinitionContent, err)
	}

	existingDef, err := existingDefinitionFor(opts, sourceURL)
	if err != nil {
		return nil, err
	}
	if existingDef != nil {
		log.Debug().Str("url", sourceURL).Msg("GraphQL definition already exists for this source URL")
		return existingDef, nil
	}

	baseURL := definitionBaseURL(sourceURL)
	name := "GraphQL"
	if baseURL != "" {
		name = "GraphQL - " + baseURL
	}

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
		SourceURL:                sourceURL,
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
		return storeEndpointsWithCount(tx, definition, endpoints)
	})
	if txErr != nil {
		log.Warn().Err(txErr).Str("definition_id", definition.ID.String()).Msg("Failed to persist GraphQL definition child records")
		definition.EndpointCount = 0
	}

	log.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Int("queries", queryCount).
		Int("mutations", mutationCount).
		Int("subscriptions", subscriptionCount).
		Str("source_url", sourceURL).
		Msg("Persisted discovered GraphQL definition")

	return definition, nil
}

func PersistWSDLDefinition(history *db.History, opts APIPersistenceOptions) (*db.APIDefinition, error) {
	body, err := history.ResponseBody()
	if err != nil {
		return nil, err
	}

	sourceURL := opts.definitionSourceURL(history)

	parser := pkgWsdl.NewParser()
	wsdlDoc, err := parser.ParseFromBytes(body, sourceURL)
	if err != nil {
		log.Debug().Err(err).Str("url", sourceURL).Msg("Failed to parse WSDL document for persistence")
		return nil, fmt.Errorf("%w: %w", ErrInvalidDefinitionContent, err)
	}

	existingDef, err := existingDefinitionFor(opts, sourceURL)
	if err != nil {
		return nil, err
	}
	if existingDef != nil {
		log.Debug().Str("url", sourceURL).Msg("WSDL definition already exists for this source URL")
		return existingDef, nil
	}

	baseURL := definitionBaseURL(sourceURL)

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
	} else if baseURL != "" {
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
		SourceURL:           sourceURL,
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
		return storeEndpointsWithCount(tx, definition, endpoints)
	})
	if txErr != nil {
		log.Warn().Err(txErr).Str("definition_id", definition.ID.String()).Msg("Failed to persist WSDL definition child records")
		definition.EndpointCount = 0
	}

	log.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Int("services", wsdlServiceCount).
		Int("operations", wsdlOperationCount).
		Int("endpoints", definition.EndpointCount).
		Str("source_url", sourceURL).
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

// PersistAPIDefinitionFromContent stores a definition parsed from a document the
// caller already holds. It backs every explicit import — the API, import-and-scan
// and the CLI — and each call stores a new definition:
//
//   - content pasted by hand has no source URL and is stored with none, so two
//     pastes into one workspace are two definitions rather than one row the second
//     paste silently took over;
//   - re-importing a URL that is already in the library adds a definition instead
//     of rewriting the one that is there. An import is additive on purpose: the
//     row already in the library may carry endpoints the user enabled or disabled,
//     issues found against them and a name they chose, and none of that is the new
//     document's to discard.
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

	// The definition is identified by the source URL the caller gave — not by the
	// synthetic request the content was wrapped in, whose path is "/" for every
	// pasted document in every workspace.
	sourceURL := opts.SourceURL
	persistOpts := APIPersistenceOptions{
		WorkspaceID:     opts.WorkspaceID,
		ScanID:          opts.ScanID,
		SourceURL:       &sourceURL,
		AlwaysCreateNew: true,
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

	// Only the columns the import options actually override are written. This used
	// to build the same map and then ignore it in favour of saving the whole struct,
	// which rewrote every column from memory — including endpoint_count, so a named
	// import stamped the definition's counter back to whatever the struct happened
	// to hold and left definitions reporting 0 endpoints while their rows existed.
	updates := make(map[string]interface{})
	if opts.Name != "" {
		updates["name"] = opts.Name
		definition.Name = opts.Name
	}
	if opts.BaseURL != "" {
		updates["base_url"] = opts.BaseURL
		definition.BaseURL = opts.BaseURL
	}
	if opts.AuthConfigID != nil {
		updates["auth_config_id"] = opts.AuthConfigID
		definition.AuthConfigID = opts.AuthConfigID
	}
	if len(updates) > 0 {
		if err := db.Connection().UpdateAPIDefinitionFields(definition.ID, updates); err != nil {
			log.Warn().Err(err).Str("definition_id", definition.ID.String()).Msg("Failed to apply import overrides to API definition")
		}
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
