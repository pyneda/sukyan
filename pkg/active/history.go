package active

import (
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

	activeOptions := ActiveModuleOptions{
		Concurrency: historyItemModulesConcurrency,
		WorkspaceID: options.WorkspaceID,
		TaskID:      options.TaskID,
		TaskJobID:   options.TaskJobID,
		ScanMode:    options.Mode,
	}
	historyCreateOptions := http_utils.HistoryCreationOptions{
		Source:              db.SourceScanner,
		WorkspaceID:         options.WorkspaceID,
		TaskID:              options.TaskID,
		CreateNewBodyStream: true,
		TaskJobID:           options.TaskJobID,
	}
	if item.StatusCode == 401 || item.StatusCode == 403 {
		ForbiddenBypassScan(item, activeOptions)
	}

	insertionPoints, err := scan.GetAndAnalyzeInsertionPoints(item, options.InsertionPoints, scan.InsertionPointAnalysisOptions{HistoryCreateOptions: historyCreateOptions})
	taskLog.Debug().Interface("insertionPoints", insertionPoints).Msg("Insertion points")
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
			scanner := scan.TemplateScanner{
				Concurrency:         historyItemModulesConcurrency,
				InteractionsManager: interactionsManager,
				AvoidRepeatedIssues: viper.GetBool("scan.avoid_repeated_issues"),
				WorkspaceID:         options.WorkspaceID,
				Mode:                options.Mode,
			}
			scanner.Run(item, payloadGenerators, insertionPointsToAudit, options)
		}

		// reflectedIssues := issues[db.ReflectedInputCode.String()]
		// if len(issues) == 0 {
		// 	taskLog.Info().Int("issues", len(issues)).Msg("Issues detected using template scanner, proceeding to client side audits")
		// }
		// taskLog.Info().Int("reflected_input_issues", len(reflectedIssues)).Msg("Input returned in response issues detected, proceeding to client side audits")
		if options.AuditCategories.ClientSide {

			alert := AlertAudit{
				WorkspaceID:                options.WorkspaceID,
				TaskID:                     options.TaskID,
				TaskJobID:                  options.TaskJobID,
				SkipInitialAlertValidation: false,
			}
			taskLog.Info().Msg("Starting client side audits")

			xssPayloads := payloads.GetXSSPayloads()
			alert.RunWithPayloads(item, xssInsertionPoints, xssPayloads, db.XssReflectedCode)

			cstiPayloads := payloads.GetCSTIPayloads()
			alert.RunWithPayloads(item, xssInsertionPoints, cstiPayloads, db.CstiCode)
			taskLog.Info().Msg("Completed client side audits")
			cspp := ClientSidePrototypePollutionAudit{
				HistoryItem: item,
				WorkspaceID: options.WorkspaceID,
				TaskID:      options.TaskID,
				TaskJobID:   options.TaskJobID,
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

	if options.AuditCategories.ServerSide && (options.Mode == scan_options.ScanModeFuzz || scan.PlatformJava.MatchesAnyFingerprint(options.Fingerprints)) {
		log4shell := Log4ShellInjectionAudit{
			URL:                 item.URL,
			Concurrency:         historyItemModulesConcurrency,
			InteractionsManager: interactionsManager,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
		}
		log4shell.Run()
	}

	if options.AuditCategories.ServerSide {
		hostHeader := HostHeaderInjectionAudit{
			URL:         item.URL,
			Concurrency: historyItemModulesConcurrency,
			WorkspaceID: options.WorkspaceID,
			TaskID:      options.TaskID,
			TaskJobID:   options.TaskJobID,
		}
		hostHeader.Run()
		// NOTE: Checks below are probably not worth to run against every history item,
		// but also not only once per target. Should find a way to run them only in some cases
		// but ensuring they are checked against X different history items per target.
		sni := SNIAudit{
			HistoryItem:         item,
			InteractionsManager: interactionsManager,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
		}
		sni.Run()

		HttpVersionsScan(item, activeOptions)
	}

	if options.ExperimentalAudits {

		methods := HTTPMethodsAudit{
			HistoryItem: item,
			Concurrency: 5,
			WorkspaceID: options.WorkspaceID,
			TaskID:      options.TaskID,
			TaskJobID:   options.TaskJobID,
		}
		methods.Run()
	}
	JSONPCallbackScan(item, activeOptions)

	log.Info().Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Msg("Finished scanning history item")
}
