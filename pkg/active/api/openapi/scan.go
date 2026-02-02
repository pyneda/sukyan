package openapi

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// ScanOpenAPIEndpoint runs OpenAPI-specific security tests on an endpoint.
// Note: Generic tests like MethodOverrideScan and MassAssignmentScan are already
// run by ScanHistoryItem in the scan executor, so we only run API-specific tests here.
func ScanOpenAPIEndpoint(definition *db.APIDefinition, endpoint *db.APIEndpoint, baseHistory *db.History, opts *OpenAPIAuditOptions) {
	taskLog := log.With().
		Str("module", "openapi-endpoint-scan").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping OpenAPI endpoint scan")
			return
		default:
		}
	}

	if definition == nil || definition.Type != db.APIDefinitionTypeOpenAPI {
		return
	}

	if endpoint == nil || baseHistory == nil {
		return
	}

	taskLog.Info().
		Str("path", endpoint.Path).
		Str("method", endpoint.Method).
		Msg("Starting OpenAPI endpoint security scan")

	// Authentication Enforcement: Test if endpoints with declared security
	// requirements actually enforce authentication
	authAudit := AuthenticationEnforcementAudit{
		Options:     opts,
		Definition:  definition,
		Endpoint:    endpoint,
		BaseHistory: baseHistory,
	}
	authAudit.Run()

	// Content-Type Enforcement: Test if endpoints accept undocumented content types
	contentTypeAudit := ContentTypeEnforcementAudit{
		Options:     opts,
		Definition:  definition,
		Endpoint:    endpoint,
		BaseHistory: baseHistory,
	}
	contentTypeAudit.Run()

	taskLog.Info().Msg("Completed OpenAPI endpoint security scan")
}

// ScanOpenAPIDefinition runs API-level security tests on an OpenAPI definition.
// These tests check global API characteristics that don't require per-endpoint context.
func ScanOpenAPIDefinition(definition *db.APIDefinition, opts *OpenAPIAuditOptions) {
	taskLog := log.With().
		Str("module", "openapi-definition-scan").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping OpenAPI definition scan")
			return
		default:
		}
	}

	if definition == nil || definition.Type != db.APIDefinitionTypeOpenAPI {
		return
	}

	taskLog.Info().
		Str("url", definition.BaseURL).
		Msg("Starting OpenAPI definition-level security scan")

	// Future: Add API-level tests like:
	// - Security scheme analysis
	// - CORS misconfiguration
	// - Rate limiting detection
	// - API versioning issues

	taskLog.Info().Msg("Completed OpenAPI definition-level security scan")
}
