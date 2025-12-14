package active

import (
	"context"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const historyItemModulesConcurrency = 10

func ScanHistoryItem(item *db.History, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options scan_options.HistoryItemScanOptions) {
	taskLog := log.With().Uint("workspace", options.WorkspaceID).Str("mode", options.Mode.String()).Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Logger()
	taskLog.Info().Msg("Starting to scan history item")

	// Get context from options, defaulting to background context if not provided
	ctx := options.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Check context before starting
	select {
	case <-ctx.Done():
		taskLog.Info().Msg("Scan cancelled before starting")
		return
	default:
	}

	activeOptions := ActiveModuleOptions{
		Ctx:         ctx,
		Concurrency: historyItemModulesConcurrency,
		WorkspaceID: options.WorkspaceID,
		TaskID:      options.TaskID,
		TaskJobID:   options.TaskJobID,
		ScanID:      options.ScanID,
		ScanJobID:   options.ScanJobID,
		ScanMode:    options.Mode,
	}
	historyCreateOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         options.WorkspaceID,
		TaskID:              options.TaskID,
		CreateNewBodyStream: true,
		TaskJobID:           options.TaskJobID,
		ScanID:              options.ScanID,
		ScanJobID:           options.ScanJobID,
	}
	if item.StatusCode == 401 || item.StatusCode == 403 {
		ForbiddenBypassScan(item, activeOptions)
	}

	// Check context after bypass scan
	select {
	case <-ctx.Done():
		taskLog.Info().Msg("Scan cancelled after bypass scan")
		return
	default:
	}

	// if options.AuditCategories.ServerSide && (options.Mode == scan_options.ScanModeFuzz || scan.PlatformNode.MatchesAnyFingerprint(options.Fingerprints)) {
	if options.AuditCategories.ServerSide {
		rscRCE := React2ShellAudit{
			Options:             activeOptions,
			HistoryItem:         item,
			InteractionsManager: interactionsManager,
		}
		rscRCE.Run()
	}

	// Use comprehensive reflection analysis for context-aware XSS scanning
	insertionPointOptions := scan.InsertionPointAnalysisOptions{
		HistoryCreateOptions:      historyCreateOptions,
		ReflectionAnalysis:        options.AuditCategories.ClientSide, // Enable reflection analysis for client-side checks
		TestCharacterEfficiencies: options.AuditCategories.ClientSide, // Test encoding when client-side checks enabled
	}
	insertionPoints, err := scan.GetAndAnalyzeInsertionPoints(item, options.InsertionPoints, insertionPointOptions)
	taskLog.Debug().Interface("insertionPoints", scan.LogSummarySlice(insertionPoints)).Msg("Insertion points")
	if err != nil {
		taskLog.Error().Err(err).Msg("Could not get insertion points")
	}

	if len(insertionPoints) > 0 {
		var insertionPointsToAudit []scan.InsertionPoint
		var xssInsertionPoints []scan.InsertionPoint
		switch options.Mode {
		case scan_options.ScanModeSmart:
			for _, insertionPoint := range insertionPoints {
				if insertionPoint.Behaviour.IsDynamic || insertionPoint.Behaviour.IsReflected || insertionPoint.Type == scan.InsertionPointTypeBody || insertionPoint.Type == scan.InsertionPointTypeParameter {
					insertionPointsToAudit = append(insertionPointsToAudit, insertionPoint)
					xssInsertionPoints = append(xssInsertionPoints, insertionPoint)
				} else {
					taskLog.Debug().Str("insertionPoint", insertionPoint.Name).Msg("Skipping insertion point")
				}
			}
		case scan_options.ScanModeFast:
			for _, insertionPoint := range insertionPoints {
				if insertionPoint.Behaviour.IsDynamic || insertionPoint.Behaviour.IsReflected {
					insertionPointsToAudit = append(insertionPointsToAudit, insertionPoint)
					xssInsertionPoints = append(xssInsertionPoints, insertionPoint)
				} else {
					taskLog.Debug().Str("insertionPoint", insertionPoint.Name).Msg("Skipping insertion point")
				}
			}

		case scan_options.ScanModeFuzz:
			insertionPointsToAudit = insertionPoints
			xssInsertionPoints = insertionPoints
		}

		if options.AuditCategories.ServerSide {
			// Check context before server-side audits
			select {
			case <-ctx.Done():
				taskLog.Info().Msg("Scan cancelled before server-side audits")
				return
			default:
			}

			scanner := scan.TemplateScanner{
				Ctx:                 ctx,
				Concurrency:         historyItemModulesConcurrency,
				InteractionsManager: interactionsManager,
				AvoidRepeatedIssues: viper.GetBool("scan.avoid_repeated_issues"),
				WorkspaceID:         options.WorkspaceID,
				Mode:                options.Mode,
			}
			scanner.Run(item, payloadGenerators, insertionPointsToAudit, options)
		}

		// Check context before client-side audits
		select {
		case <-ctx.Done():
			taskLog.Info().Msg("Scan cancelled before client-side audits")
			return
		default:
		}

		// reflectedIssues := issues[db.ReflectedInputCode.String()]
		// if len(issues) == 0 {
		// 	taskLog.Info().Int("issues", len(issues)).Msg("Issues detected using template scanner, proceeding to client side audits")
		// }
		// taskLog.Info().Int("reflected_input_issues", len(reflectedIssues)).Msg("Input returned in response issues detected, proceeding to client side audits")
		if options.AuditCategories.ClientSide {

			alert := AlertAudit{
				Ctx:                        ctx,
				WorkspaceID:                options.WorkspaceID,
				TaskID:                     options.TaskID,
				TaskJobID:                  options.TaskJobID,
				ScanID:                     options.ScanID,
				ScanJobID:                  options.ScanJobID,
				SkipInitialAlertValidation: false,
			}
			taskLog.Info().Msg("Starting client side audits")

			alert.RunWithContextAwarePayloads(item, xssInsertionPoints, db.XssReflectedCode)

			cstiPayloads := payloads.GetCSTIPayloads()
			alert.RunWithPayloads(item, xssInsertionPoints, cstiPayloads, db.CstiCode)
			taskLog.Info().Msg("Completed client side audits")
			cspp := ClientSidePrototypePollutionAudit{
				Ctx:         ctx,
				HistoryItem: item,
				WorkspaceID: options.WorkspaceID,
				TaskID:      options.TaskID,
				TaskJobID:   options.TaskJobID,
				ScanID:      options.ScanID,
				ScanJobID:   options.ScanJobID,
			}
			cspp.Run()
			taskLog.Info().Msg("Completed client side prototype pollution audit")

		}

	} else {
		taskLog.Info().Msg("No insertion points to audit")
	}

	if item.StatusCode >= 300 || item.StatusCode < 400 {
		OpenRedirectScan(item, activeOptions, insertionPoints)
	} else {
		var openRedirectInsertionPoints []scan.InsertionPoint
		for _, insertionPoint := range insertionPoints {
			if scan.IsCommonOpenRedirectParameter(insertionPoint.Name) {
				openRedirectInsertionPoints = append(openRedirectInsertionPoints, insertionPoint)
			}
		}
		if len(openRedirectInsertionPoints) > 0 {
			OpenRedirectScan(item, activeOptions, openRedirectInsertionPoints)
		}
	}

	if options.AuditCategories.ClientSide {
		if http_utils.IsHTMLContentType(item.ResponseContentType) || http_utils.IsJavaScriptContentType(item.ResponseContentType) {
			taskLog.Info().Msg("Starting DOM XSS audit")
			domXSS := DOMXSSAudit{
				Options:     activeOptions,
				HistoryItem: item,
			}
			domXSS.Run()
			taskLog.Info().Msg("Completed DOM XSS audit")
		} else {
			taskLog.Debug().Str("content_type", item.ResponseContentType).Msg("Skipping DOM XSS audit - not applicable for content type")
		}
	}

	if options.AuditCategories.ServerSide && (options.Mode == scan_options.ScanModeFuzz || scan.PlatformJava.MatchesAnyFingerprint(options.Fingerprints)) {
		log4shell := Log4ShellInjectionAudit{
			Ctx:                 ctx,
			URL:                 item.URL,
			Concurrency:         historyItemModulesConcurrency,
			InteractionsManager: interactionsManager,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
			ScanID:              options.ScanID,
			ScanJobID:           options.ScanJobID,
		}
		log4shell.Run()
	}

	if options.AuditCategories.ServerSide {

		hostHeader := HostHeaderInjectionAudit{
			Ctx:         ctx,
			URL:         item.URL,
			Concurrency: historyItemModulesConcurrency,
			WorkspaceID: options.WorkspaceID,
			TaskID:      options.TaskID,
			TaskJobID:   options.TaskJobID,
			ScanID:      options.ScanID,
			ScanJobID:   options.ScanJobID,
		}
		hostHeader.Run()
		// NOTE: Checks below are probably not worth to run against every history item,
		// but also not only once per target. Should find a way to run them only in some cases
		// but ensuring they are checked against X different history items per target.
		sni := SNIAudit{
			Ctx:                 ctx,
			HistoryItem:         item,
			InteractionsManager: interactionsManager,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
			ScanID:              options.ScanID,
			ScanJobID:           options.ScanJobID,
		}
		sni.Run()

		HttpVersionsScan(item, activeOptions)
	}

	if options.ExperimentalAudits {

		methods := HTTPMethodsAudit{
			Ctx:         ctx,
			HistoryItem: item,
			Concurrency: 5,
			WorkspaceID: options.WorkspaceID,
			TaskID:      options.TaskID,
			TaskJobID:   options.TaskJobID,
			ScanID:      options.ScanID,
			ScanJobID:   options.ScanJobID,
		}
		methods.Run()
	}
	JSONPCallbackScan(item, activeOptions)

	log.Info().Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Msg("Finished scanning history item")
}
