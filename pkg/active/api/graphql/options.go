package graphql

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/rs/zerolog/log"
)

// GraphQLAuditOptions embeds ActiveModuleOptions and adds GraphQL-specific options
type GraphQLAuditOptions struct {
	active.ActiveModuleOptions
}

// reportIssue is a helper to report a GraphQL security issue
func reportIssue(history *db.History, code db.IssueCode, details string, confidence int, opts *GraphQLAuditOptions) {
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
		log.Error().Err(err).Interface("code", code).Msg("Failed to create GraphQL issue")
		return
	}
	log.Info().Uint("issue_id", issue.ID).Str("code", string(code)).Msg("Created GraphQL security issue")
}
