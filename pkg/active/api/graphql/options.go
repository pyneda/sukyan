package graphql

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
)

// GraphQLAuditOptions embeds ActiveModuleOptions and adds GraphQL-specific options.
// BaseHistory, PayloadGenerators, InteractionsManager and AuditCategories are needed
// for per-operation resolver-argument injection (ScanGraphQLOperation).
type GraphQLAuditOptions struct {
	active.ActiveModuleOptions
	BaseHistory         *db.History
	PayloadGenerators   []*generation.PayloadGenerator
	InteractionsManager *integrations.InteractionsManager
	AuditCategories     scan_options.AuditCategories
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
