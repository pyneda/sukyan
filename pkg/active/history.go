package active

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func ScanHistoryItem(item *db.History, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options scan.HistoryItemScanOptions) {
	taskLog := log.With().Uint("workspace", options.WorkspaceID).Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Logger()
	taskLog.Info().Msg("Starting to scan history item")

	activeOptions := ActiveModuleOptions{
		Concurrency: 10,
		WorkspaceID: options.WorkspaceID,
		TaskID:      options.TaskID,
		TaskJobID:   options.TaskJobID,
	}
	if item.StatusCode == 401 || item.StatusCode == 403 {
		ForbiddenBypassScan(item, activeOptions)
	}

	insertionPoints, err := scan.GetInsertionPoints(item, options.InsertionPoints)
	taskLog.Debug().Interface("insertionPoints", insertionPoints).Msg("Insertion points")
	if err != nil {
		taskLog.Error().Err(err).Msg("Could not get insertion points")
	}

	if len(insertionPoints) > 0 {
		scanner := scan.TemplateScanner{
			Concurrency:         10,
			InteractionsManager: interactionsManager,
			AvoidRepeatedIssues: viper.GetBool("scan.avoid_repeated_issues"),
			WorkspaceID:         options.WorkspaceID,
		}
		issues := scanner.Run(item, payloadGenerators, insertionPoints, options)
		reflectedIssues := issues[db.ReflectedInputCode.String()]
		taskLog.Info().Int("reflected_input_issues", len(reflectedIssues)).Msg("Input returned in response issues detected, proceeding to client side audits")
		alert := AlertAudit{
			WorkspaceID:                options.WorkspaceID,
			TaskID:                     options.TaskID,
			TaskJobID:                  options.TaskJobID,
			SkipInitialAlertValidation: false,
		}

		var xssInsertionPoints []scan.InsertionPoint
		switch options.Mode {
		case scan.ScanModeSmart:
			// TODO: Think about a better way to decide which insertion points to use
			for _, reflected := range reflectedIssues {
				xssInsertionPoints = append(xssInsertionPoints, reflected.InsertionPoint)
			}
			if len(xssInsertionPoints) == 0 {
				for _, insertionPoint := range insertionPoints {
					if insertionPoint.Type != scan.InsertionPointTypeHeader && insertionPoint.Type != scan.InsertionPointTypeCookie {
						xssInsertionPoints = append(xssInsertionPoints, insertionPoint)
					}
				}
			}
		case scan.ScanModeFast:
			for _, reflected := range reflectedIssues {
				xssInsertionPoints = append(xssInsertionPoints, reflected.InsertionPoint)
			}
		case scan.ScanModeFuzz:
			xssInsertionPoints = insertionPoints
		}
		xssPayloads := payloads.GetXSSPayloads()
		// alert.Run(item, xssInsertionPoints, "default.txt", db.XssReflectedCode)
		alert.RunWithPayloads(item, xssInsertionPoints, xssPayloads, db.XssReflectedCode)

		cstiPayloads := payloads.GetCSTIPayloads()
		alert.RunWithPayloads(item, xssInsertionPoints, cstiPayloads, db.CstiCode)
	}

	if options.ExperimentalAudits {
		cspp := ClientSidePrototypePollutionAudit{
			HistoryItem: item,
			WorkspaceID: options.WorkspaceID,
			TaskID:      options.TaskID,
			TaskJobID:   options.TaskJobID,
		}
		cspp.Run()
	}

	// var specificParamsToTest []string
	// // NOTE: This should be deprecated
	// p := web.WebPage{URL: item.URL}
	// hasParams, _ := p.HasParameters()
	// if hasParams && options.IsScopedInsertionPoint("parameters") {
	// 	// TestXSS(item.URL, specificParamsToTest, "default.txt", false)
	// 	// log.Warn().Msg("Starting XSS Audit")
	// 	// xss := XSSAudit{
	// 	// 	WorkspaceID: options.WorkspaceID,
	// 	// 	TaskID:      options.TaskID,
	// 	// 	TaskJobID:   options.TaskJobID,
	// 	// }
	// 	// xss.Run(item.URL, specificParamsToTest, "default.txt", false)
	// 	// log.Warn().Msg("Completed XSS Audit")

	// 	// pathTraversal := PathTraversalAudit{
	// 	// 	URL:              item.URL,
	// 	// 	Params:           specificParamsToTest,
	// 	// 	Concurrency:      20,
	// 	// 	PayloadsDepth:    5,
	// 	// 	Platform:         "all",
	// 	// 	StopAfterSuccess: false,
	// 	// }
	// 	// pathTraversal.Run()
	// }

	if options.IsScopedInsertionPoint("headers") {
		log4shell := Log4ShellInjectionAudit{
			URL:                 item.URL,
			Concurrency:         10,
			InteractionsManager: interactionsManager,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
		}
		log4shell.Run()
	}
	hostHeader := HostHeaderInjectionAudit{
		URL:         item.URL,
		Concurrency: 10,
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
	methods := HTTPMethodsAudit{
		HistoryItem: item,
		Concurrency: 5,
		WorkspaceID: options.WorkspaceID,
		TaskID:      options.TaskID,
		TaskJobID:   options.TaskJobID,
	}
	methods.Run()
	log.Info().Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Msg("Finished scanning history item")
}
