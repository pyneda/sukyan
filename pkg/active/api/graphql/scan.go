package graphql

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/rs/zerolog/log"
)

// ScanGraphQLAPI runs all API-level security tests on a GraphQL endpoint.
// These tests check global API characteristics that don't require per-operation context.
func ScanGraphQLAPI(definition *db.APIDefinition, opts *GraphQLAuditOptions) {
	taskLog := log.With().
		Str("module", "graphql-api-scan").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping GraphQL API scan")
			return
		default:
		}
	}

	if definition == nil || definition.Type != db.APIDefinitionTypeGraphQL {
		return
	}

	taskLog.Info().Str("url", definition.BaseURL).Msg("Starting GraphQL API-level security scan")

	// Introspection audit
	introspection := IntrospectionAudit{
		Options:    opts,
		Definition: definition,
	}
	introspection.Run()

	// Batching audit
	batching := BatchingAudit{
		Options:    opts,
		Definition: definition,
	}
	batching.Run()

	// Depth limit audit
	depth := DepthLimitAudit{
		Options:    opts,
		Definition: definition,
	}
	depth.Run()

	// Field suggestions audit
	suggestions := FieldSuggestionsAudit{
		Options:    opts,
		Definition: definition,
	}
	suggestions.Run()

	// Directives audit
	directives := DirectivesAudit{
		Options:    opts,
		Definition: definition,
	}
	directives.Run()

	// Sensitive fields audit
	sensitiveFields := SensitiveFieldsAudit{
		Options:    opts,
		Definition: definition,
	}
	sensitiveFields.Run()

	taskLog.Info().Msg("Completed GraphQL API-level security scan")
}

// ScanGraphQLOperation runs security tests on a specific GraphQL operation.
// These tests are operation-specific and require operation context.
func ScanGraphQLOperation(definition *db.APIDefinition, operation *core.Operation, opts *GraphQLAuditOptions) {
	taskLog := log.With().
		Str("module", "graphql-operation-scan").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping GraphQL operation scan")
			return
		default:
		}
	}

	if definition == nil || definition.Type != db.APIDefinitionTypeGraphQL {
		return
	}

	if operation == nil || operation.GraphQL == nil {
		taskLog.Debug().Msg("No GraphQL operation metadata available")
		return
	}

	taskLog.Debug().
		Str("operation", operation.Name).
		Str("type", operation.GraphQL.OperationType).
		Msg("Starting GraphQL operation-specific security scan")

	// Operation-specific tests would go here
	// For now, operations share the API-level tests since GraphQL
	// queries are sent to a single endpoint

}
