package openapi

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/api/core"
	"github.com/rs/zerolog/log"
)

// OpenAPIAuditOptions embeds ActiveModuleOptions and adds OpenAPI-specific options
type OpenAPIAuditOptions struct {
	active.ActiveModuleOptions
	Operation      *core.Operation
	BehaviorResult *db.APIBehaviorResult
}

// reportIssue is a helper to report an OpenAPI security issue
func reportIssue(history *db.History, code db.IssueCode, details string, confidence int, opts *OpenAPIAuditOptions) {
	issue, err := db.CreateIssueFromHistoryAndTemplate(
		history,
		code,
		details,
		confidence,
		"",
		&opts.WorkspaceID,
		&opts.TaskID,
		&opts.TaskJobID,
		&opts.ScanID,
		&opts.ScanJobID,
	)
	if err != nil {
		log.Error().Err(err).Interface("code", code).Msg("Failed to create OpenAPI issue")
		return
	}
	log.Info().Uint("issue_id", issue.ID).Str("code", string(code)).Msg("Created OpenAPI security issue")
}
