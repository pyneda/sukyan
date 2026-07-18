package graphql

import (
	"context"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const resolverInjectionConcurrency = 10

// shouldRunResolverArgInjection reports whether per-operation resolver-arg injection
// should run for the given mode. In Fuzz mode the standard base-history scan already
// keeps and injects the GraphQL insertion points, so running here would duplicate it.
// In Smart/Fast mode the standard scan drops those points, so this is the only path
// that reaches the resolver args.
func shouldRunResolverArgInjection(mode scan_options.ScanMode) bool {
	return mode != scan_options.ScanModeFuzz
}

// graphqlResolverInsertionPoints derives the resolver-argument insertion points
// (graphql variables + inline args) from a GraphQL operation's base request, so
// per-operation injection targets exactly the resolver args and nothing else. The
// JSON body/fullbody points that GetInsertionPoints also emits are dropped here —
// those are covered by the standard scan of the base history.
func graphqlResolverInsertionPoints(history *db.History) ([]scan.InsertionPoint, error) {
	points, err := scan.GetInsertionPoints(history, []string{"graphql"})
	if err != nil {
		return nil, err
	}

	var graphqlPoints []scan.InsertionPoint
	for _, point := range points {
		if point.Type == scan.InsertionPointTypeGraphQLVariable || point.Type == scan.InsertionPointTypeGraphQLInlineArg {
			graphqlPoints = append(graphqlPoints, point)
		}
	}
	return graphqlPoints, nil
}

// runResolverArgInjection injects server-side payloads (SQLi/SSTI/cmdi/SSRF/XXE/
// path-traversal) into a GraphQL operation's resolver arguments and runs detection.
//
// The GraphQL insertion points are fed to the TemplateScanner directly so they are
// not dropped by the Smart/Fast mode filtering in active.ScanHistoryItem (those
// modes keep only dynamic/reflected/body/parameter points, and graphql points are
// none of those). Reflection/analysis is skipped because these are pre-derived from
// the base request.
func runResolverArgInjection(opts *GraphQLAuditOptions) {
	if opts.BaseHistory == nil {
		return
	}
	if !opts.AuditCategories.ServerSide {
		return
	}
	if len(opts.PayloadGenerators) == 0 || opts.InteractionsManager == nil {
		return
	}
	if !shouldRunResolverArgInjection(opts.ScanMode) {
		return
	}

	ctx := opts.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return
	default:
	}

	insertionPoints, err := graphqlResolverInsertionPoints(opts.BaseHistory)
	if err != nil {
		log.Error().Err(err).Uint("history", opts.BaseHistory.ID).Msg("Failed to derive GraphQL resolver insertion points")
		return
	}
	if len(insertionPoints) == 0 {
		return
	}

	scanner := scan.TemplateScanner{
		Ctx:                 ctx,
		Concurrency:         resolverInjectionConcurrency,
		InteractionsManager: opts.InteractionsManager,
		AvoidRepeatedIssues: viper.GetBool("scan.avoid_repeated_issues"),
		WorkspaceID:         opts.WorkspaceID,
		Mode:                opts.ScanMode,
	}

	scanOptions := scan_options.HistoryItemScanOptions{
		Ctx:             ctx,
		WorkspaceID:     opts.WorkspaceID,
		TaskID:          opts.TaskID,
		TaskJobID:       opts.TaskJobID,
		ScanID:          opts.ScanID,
		ScanJobID:       opts.ScanJobID,
		Mode:            opts.ScanMode,
		InsertionPoints: []string{"graphql"},
		AuditCategories: opts.AuditCategories,
		HTTPClient:      opts.HTTPClient,
	}

	scanner.Run(opts.BaseHistory, opts.PayloadGenerators, insertionPoints, scanOptions)
}
