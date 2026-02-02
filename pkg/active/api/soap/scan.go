package soap

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// ScanSOAPEndpoint runs all security tests on a SOAP/WSDL endpoint.
func ScanSOAPEndpoint(definition *db.APIDefinition, endpoint *db.APIEndpoint, baseHistory *db.History, opts *SOAPAuditOptions) {
	taskLog := log.With().
		Str("module", "soap-endpoint-scan").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping SOAP endpoint scan")
			return
		default:
		}
	}

	if definition == nil || definition.Type != db.APIDefinitionTypeWSDL {
		return
	}

	taskLog.Info().
		Str("url", definition.BaseURL).
		Msg("Starting SOAP endpoint security scan")

	// SOAP Action spoofing audit
	actionSpoofing := ActionSpoofingAudit{
		Options:     opts,
		Definition:  definition,
		Endpoint:    endpoint,
		BaseHistory: baseHistory,
	}
	actionSpoofing.Run()

	taskLog.Info().Msg("Completed SOAP endpoint security scan")
}

// ScanSOAPDefinition runs API-level security tests on a SOAP/WSDL definition.
// These tests check global API characteristics that don't require per-endpoint context.
func ScanSOAPDefinition(definition *db.APIDefinition, opts *SOAPAuditOptions) {
	taskLog := log.With().
		Str("module", "soap-definition-scan").
		Uint("workspace", opts.WorkspaceID).
		Logger()

	if opts.Ctx != nil {
		select {
		case <-opts.Ctx.Done():
			taskLog.Debug().Msg("Context cancelled, skipping SOAP definition scan")
			return
		default:
		}
	}

	if definition == nil || definition.Type != db.APIDefinitionTypeWSDL {
		return
	}

	taskLog.Info().
		Str("url", definition.BaseURL).
		Msg("Starting SOAP definition-level security scan")

	taskLog.Info().Msg("Completed SOAP definition-level security scan")
}
