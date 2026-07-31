package discovery

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/api/core"
	apigraphql "github.com/pyneda/sukyan/pkg/api/graphql"
	apiopenapi "github.com/pyneda/sukyan/pkg/api/openapi"
	"github.com/pyneda/sukyan/pkg/api/operationlink"
	apisoap "github.com/pyneda/sukyan/pkg/api/soap"
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

// attachOperations normalizes the document a second time and stores the resulting
// operation on each endpoint row, so the detail API can serve parameters, schemas
// and an example request without re-parsing the specification on every read.
//
// Failure is deliberately not fatal: the endpoints are what the scanner needs, and
// the detail API backfills from the stored raw definition on first read.
func attachOperations(endpoints []*db.APIEndpoint, body []byte, definition *db.APIDefinition, sourceURL string) {
	if len(endpoints) == 0 {
		return
	}

	var (
		operations []core.Operation
		err        error
	)
	switch definition.Type {
	case db.APIDefinitionTypeGraphQL:
		operations, _, err = apigraphql.ParseFromRawDefinition(body, definition.BaseURL)
	case db.APIDefinitionTypeWSDL:
		operations, err = apisoap.ParseFromRawDefinition(body, sourceURL)
	default:
		operations, err = apiopenapi.ParseFromRawDefinition(body)
	}
	if err != nil {
		log.Warn().Err(err).Str("source_url", sourceURL).Str("type", string(definition.Type)).
			Msg("Could not normalize operations; the detail view will backfill on first read")
		return
	}

	matched := operationlink.AttachOperationJSON(endpoints, operations, definition.Type)
	if matched < len(endpoints) {
		log.Debug().Int("matched", matched).Int("endpoints", len(endpoints)).Str("source_url", sourceURL).
			Msg("Some endpoints could not be matched to a parsed operation")
	}
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

	attachOperations(endpoints, body, definition, sourceURL)

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

	if opts.AlwaysCreateNew {
		definition, err = db.Connection().CreateAPIDefinition(definition)
		if err != nil {
			return nil, err
		}
	} else {
		stored, reused, storeErr := storeGraphQLDefinitionUnlessAliased(definition, graphQLSchemaFingerprint(schema))
		if storeErr != nil {
			return nil, storeErr
		}
		if reused {
			log.Debug().
				Str("definition_id", stored.ID.String()).
				Str("source_url", sourceURL).
				Str("canonical_url", stored.SourceURL).
				Msg("GraphQL endpoint serves a schema already stored for this origin")
			return stored, nil
		}
		definition = stored
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

	attachOperations(endpoints, body, definition, sourceURL)

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

// storeGraphQLDefinitionUnlessAliased inserts the definition unless the workspace
// already holds one for the same origin serving the same schema, in which case
// that one is returned with reused set.
//
// One GraphQL endpoint answers on many paths. A server mounted with
// app.use('/graphql', ...) replies to every path below the mount point, an SPA
// fallback replies to all of them, and plain aliases (/graphql and /api/graphql)
// are routine — while discovery POSTs the introspection query to about forty
// candidate paths. Keyed on the source URL alone, one endpoint became one
// definition per alias: a measured Apollo scan stored eight identical definitions
// of 89 operations, queued 712 api_scan jobs where 89 would do, and reported every
// finding eight times.
//
// Two different origins serving the same schema stay two definitions: they are two
// targets, they can answer differently under test, and their findings belong apart.
func storeGraphQLDefinitionUnlessAliased(definition *db.APIDefinition, fingerprint string) (*db.APIDefinition, bool, error) {
	// Without an origin there is nothing to scope the schema to, and rows that
	// share "no origin" are not the same API — the same reasoning that keeps
	// definitions with no source URL out of the source URL lookup. An empty
	// fingerprint is likewise not an identity: it is what an unreadable stored
	// document yields, and matching on it would fold those together.
	if definition.BaseURL == "" || fingerprint == "" {
		stored, err := db.Connection().CreateAPIDefinition(definition)
		return stored, false, err
	}

	var (
		stored *db.APIDefinition
		reused bool
	)

	// The check and the insert share one transaction holding an advisory lock on
	// the identity being checked. Workers race here by construction: they probe the
	// aliases of one endpoint concurrently, and possibly from separate processes, so
	// a plain check-then-act lets every one of them read "not stored yet" and insert.
	// The lock is transaction-scoped, so it is released by the commit that publishes
	// the row the losers are waiting to see.
	err := db.Connection().DB().Transaction(func(tx *gorm.DB) error {
		lockKey := graphQLDefinitionLockKey(definition.WorkspaceID, definition.BaseURL, fingerprint)
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", lockKey).Error; err != nil {
			return fmt.Errorf("locking GraphQL definition identity: %w", err)
		}

		siblings, err := db.ListGraphQLAPIDefinitionsByBaseURL(tx, definition.WorkspaceID, definition.BaseURL)
		if err != nil {
			return fmt.Errorf("listing GraphQL definitions for origin: %w", err)
		}

		for _, sibling := range siblings {
			if graphQLRawDefinitionFingerprint(sibling.RawDefinition) != fingerprint {
				continue
			}
			stored, reused = sibling, true
			return adoptCanonicalGraphQLSourceURL(tx, sibling, definition.SourceURL)
		}

		if err := db.CreateAPIDefinitionTx(tx, definition); err != nil {
			return fmt.Errorf("creating GraphQL definition: %w", err)
		}
		stored = definition
		return nil
	})
	if err != nil {
		log.Error().Err(err).Str("source_url", definition.SourceURL).Msg("Failed to store GraphQL definition")
		return nil, false, err
	}

	return stored, reused, nil
}

// adoptCanonicalGraphQLSourceURL points the definition at the most canonical alias
// seen so far for its schema.
//
// The aliases are probed concurrently, so /graphql/playground can reach the
// database before /graphql and the endpoint every later request is sent to would
// otherwise be whichever alias won the race. The ranking is a total order, so the
// arrival order of the remaining aliases cannot change where it settles.
func adoptCanonicalGraphQLSourceURL(tx *gorm.DB, existing *db.APIDefinition, candidate string) error {
	if !isMoreCanonicalGraphQLURL(candidate, existing.SourceURL) {
		return nil
	}
	if err := tx.Model(&db.APIDefinition{}).Where("id = ?", existing.ID).
		Update("source_url", candidate).Error; err != nil {
		return fmt.Errorf("updating canonical GraphQL source URL: %w", err)
	}
	existing.SourceURL = candidate
	return nil
}

func isMoreCanonicalGraphQLURL(candidate, current string) bool {
	if candidate == "" {
		return false
	}
	if current == "" {
		return true
	}

	candidateSegments, candidateLength := graphQLURLRank(candidate)
	currentSegments, currentLength := graphQLURLRank(current)
	if candidateSegments != currentSegments {
		return candidateSegments < currentSegments
	}
	if candidateLength != currentLength {
		return candidateLength < currentLength
	}
	return candidate < current
}

func graphQLURLRank(rawURL string) (segments int, length int) {
	path := rawURL
	if parsed, err := url.Parse(rawURL); err == nil {
		path = parsed.EscapedPath()
	}

	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return 0, len(path)
	}
	return strings.Count(trimmed, "/") + 1, len(path)
}

// graphQLSchemaFingerprint reduces a parsed schema to the surface it exposes:
// every operation, type, input, enum, scalar and directive, rendered with its
// argument and return signatures and sorted.
//
// The parsed schema is fingerprinted rather than the introspection bytes because
// the bytes are the server's rendering of it, not its identity: nothing obliges a
// server to order its JSON keys, its type list or a type's fields the same way
// twice, and one reordered field would make an alias look like a new API. Sorting
// everything makes the fingerprint depend only on what the scanner can inject
// into. Descriptions are left out — prose changing does not make it another API.
func graphQLSchemaFingerprint(schema *graphql.GraphQLSchema) string {
	if schema == nil {
		return ""
	}

	entries := make([]string, 0, len(schema.Queries)+len(schema.Mutations)+len(schema.Subscriptions)+
		len(schema.Types)+len(schema.InputTypes)+len(schema.Enums)+len(schema.Scalars)+len(schema.Directives))

	appendOperations := func(kind string, operations []graphql.Operation) {
		for _, operation := range operations {
			entries = append(entries, kind+" "+operation.Name+graphQLArgumentsSignature(operation.Arguments)+":"+operation.ReturnType.Signature())
		}
	}
	appendOperations("query", schema.Queries)
	appendOperations("mutation", schema.Mutations)
	appendOperations("subscription", schema.Subscriptions)

	for name, typeDef := range schema.Types {
		fields := make([]string, 0, len(typeDef.Fields))
		for _, field := range typeDef.Fields {
			fields = append(fields, field.Name+graphQLArgumentsSignature(field.Arguments)+":"+field.Type.Signature())
		}
		possibleTypes := append([]string(nil), typeDef.PossibleTypes...)
		sort.Strings(fields)
		sort.Strings(possibleTypes)
		entries = append(entries, string(typeDef.Kind)+" "+name+"{"+strings.Join(fields, ",")+"}="+strings.Join(possibleTypes, ","))
	}

	for name, inputDef := range schema.InputTypes {
		fields := make([]string, 0, len(inputDef.Fields))
		for _, field := range inputDef.Fields {
			fields = append(fields, field.Name+":"+field.Type.Signature())
		}
		sort.Strings(fields)
		entries = append(entries, "input "+name+"{"+strings.Join(fields, ",")+"}")
	}

	for name, enumDef := range schema.Enums {
		values := make([]string, 0, len(enumDef.Values))
		for _, value := range enumDef.Values {
			values = append(values, value.Name)
		}
		sort.Strings(values)
		entries = append(entries, "enum "+name+"{"+strings.Join(values, ",")+"}")
	}

	for _, scalar := range schema.Scalars {
		entries = append(entries, "scalar "+scalar)
	}

	for _, directive := range schema.Directives {
		locations := append([]string(nil), directive.Locations...)
		sort.Strings(locations)
		entries = append(entries, "directive "+directive.Name+graphQLArgumentsSignature(directive.Arguments)+"@"+strings.Join(locations, ","))
	}

	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:])
}

func graphQLArgumentsSignature(arguments []graphql.Argument) string {
	rendered := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		rendered = append(rendered, argument.Name+":"+argument.Type.Signature())
	}
	sort.Strings(rendered)
	return "(" + strings.Join(rendered, ",") + ")"
}

// graphQLRawDefinitionFingerprint fingerprints a stored definition. A document
// that no longer parses yields the empty string, which matches nothing: a schema
// we cannot read is not evidence that the one being persisted is a duplicate.
func graphQLRawDefinitionFingerprint(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	schema, err := graphql.NewParser().ParseSchema(raw)
	if err != nil {
		return ""
	}
	return graphQLSchemaFingerprint(schema)
}

// graphQLDefinitionLockKey derives the bigint PostgreSQL advisory locks take from
// the identity being serialised on.
func graphQLDefinitionLockKey(workspaceID uint, baseURL, fingerprint string) int64 {
	sum := sha256.Sum256(fmt.Appendf(nil, "api_definition:graphql:%d:%s:%s", workspaceID, baseURL, fingerprint))
	return int64(binary.BigEndian.Uint64(sum[:8]))
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

	attachOperations(endpoints, body, definition, sourceURL)

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

	scanID := options.HistoryCreationOptions.ScanID

	for _, history := range results.Responses {
		validationCtx := &ValidationContext{SiteBehavior: options.SiteBehavior}
		if valid, _, _ := validationFunc(history, validationCtx); !valid {
			continue
		}
		persistOpts := APIPersistenceOptions{
			WorkspaceID: options.HistoryCreationOptions.WorkspaceID,
		}
		if scanID > 0 {
			persistOpts.ScanID = &scanID
		}
		definition, persistErr := persistFunc(history, persistOpts)
		if persistErr != nil {
			log.Debug().Err(persistErr).Str("url", history.URL).Msgf("Failed to persist %s definition", apiType)
			continue
		}
		claimRediscoveredDefinitionForScan(definition, scanID, apiType)
	}
}

// claimRediscoveredDefinitionForScan gives the scan doing the discovery a claim on
// a definition an earlier scan already stored.
//
// A scan reaches a definition two ways: api_definitions.scan_id, which records the
// scan that discovered it, or the scan_api_definitions join table. Rediscovery
// reuses the stored row and wrote neither, so the second scan of a target found no
// definitions at all — HasLinkedAPIDefinitions answered false, ScheduleAPIScan
// enumerated nothing, and the entire API scan phase was skipped. On a GraphQL
// target that is effectively the whole scan, and it made continuous or scheduled
// scanning API-scan the target exactly once, ever.
//
// scan_id is deliberately left as it is. It records which scan discovered the
// definition; re-pointing it at the current scan would take the definition out of
// the discovering scan's own view, moving the gap rather than closing it.
func claimRediscoveredDefinitionForScan(definition *db.APIDefinition, scanID uint, apiType string) {
	if definition == nil || scanID == 0 {
		return
	}
	if definition.ScanID != nil && *definition.ScanID == scanID {
		return
	}

	if err := db.Connection().LinkAPIDefinitionToScan(scanID, definition.ID); err != nil {
		log.Warn().Err(err).
			Uint("scan_id", scanID).
			Str("definition_id", definition.ID.String()).
			Msgf("Failed to link rediscovered %s definition to scan", apiType)
		return
	}

	log.Info().
		Uint("scan_id", scanID).
		Str("definition_id", definition.ID.String()).
		Int("endpoints", definition.EndpointCount).
		Msgf("Linked previously discovered %s definition to this scan", apiType)
}
