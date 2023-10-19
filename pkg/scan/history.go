package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/web"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func ActiveScanHistoryItem(item *db.History, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options HistoryItemScanOptions) {
	taskLog := log.With().Uint("workspace", options.WorkspaceID).Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Logger()
	taskLog.Info().Msg("Starting to scan history item")

	activeOptions := active.ActiveModuleOptions{
		Concurrency: 10,
		WorkspaceID: options.WorkspaceID,
		TaskID:      options.TaskID,
		TaskJobID:   options.TaskJobID,
	}
	if item.StatusCode == 401 || item.StatusCode == 403 {
		active.AuthBypassScan(item, activeOptions)
	}

	insertionPoints, err := GetInsertionPoints(item, options.InsertionPoints)
	taskLog.Debug().Interface("insertionPoints", insertionPoints).Msg("Insertion points")
	if err != nil {
		taskLog.Error().Err(err).Msg("Could not get insertion points")
	}

	if len(insertionPoints) > 0 {
		scanner := TemplateScanner{
			Concurrency:         10,
			InteractionsManager: interactionsManager,
			AvoidRepeatedIssues: viper.GetBool("scan.avoid_repeated_issues"),
			WorkspaceID:         options.WorkspaceID,
		}
		scanner.Run(item, payloadGenerators, insertionPoints, options)
	}

	cspp := active.ClientSidePrototypePollutionAudit{
		HistoryItem: item,
		WorkspaceID: options.WorkspaceID,
		TaskID:      options.TaskID,
		TaskJobID:   options.TaskJobID,
	}
	cspp.Run()

	var specificParamsToTest []string
	p := web.WebPage{URL: item.URL}
	hasParams, _ := p.HasParameters()
	if hasParams && options.IsScopedInsertionPoint("param") {
		active.TestXSS(item.URL, specificParamsToTest, "default.txt", false)
		// pathTraversal := active.PathTraversalAudit{
		// 	URL:              item.URL,
		// 	Params:           specificParamsToTest,
		// 	Concurrency:      20,
		// 	PayloadsDepth:    5,
		// 	Platform:         "all",
		// 	StopAfterSuccess: false,
		// }
		// pathTraversal.Run()
	}

	// sni := active.SNIAudit{
	// 	HistoryItem:         item,
	// 	InteractionsManager: interactionsManager,
	// 	WorkspaceID:         options.WorkspaceID,
	// 	TaskID:      options.TaskID,
	// TaskJobID:   options.TaskJobID,
	// }
	// sni.Run()
	methods := active.HTTPMethodsAudit{
		HistoryItem: item,
		Concurrency: 5,
		WorkspaceID: options.WorkspaceID,
		TaskID:      options.TaskID,
		TaskJobID:   options.TaskJobID,
	}
	methods.Run()

	if options.IsScopedInsertionPoint("headers") {
		log4shell := active.Log4ShellInjectionAudit{
			URL:                 item.URL,
			Concurrency:         10,
			InteractionsManager: interactionsManager,
			WorkspaceID:         options.WorkspaceID,
			TaskID:              options.TaskID,
			TaskJobID:           options.TaskJobID,
		}
		log4shell.Run()
		hostHeader := active.HostHeaderInjectionAudit{
			URL:         item.URL,
			Concurrency: 10,
			WorkspaceID: options.WorkspaceID,
			TaskID:      options.TaskID,
			TaskJobID:   options.TaskJobID,
		}
		hostHeader.Run()
	}
	log.Info().Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Msg("Finished scanning history item")
}
