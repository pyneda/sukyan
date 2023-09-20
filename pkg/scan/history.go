package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/active"
	"github.com/pyneda/sukyan/pkg/fuzz"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/web"

	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
)

func analyzeInsertionPoints(item *db.History, insertionPoints []fuzz.InsertionPoint) {

	var base64Data []InsertionPoint
	for _, insertionPoint := range insertionPoints {
		if insertionPoint.ValueType == lib.TypeBase64 {
			base64Data = append(base64Data, insertionPoint)
			// NOTE: If at some time, we have a way to tell the scanner checks to encode payloads,
			// we could check which data type is the original data, find insertion points and instruct
			// the scanner checks to base64 encode the original insertion point data.
		}
	}

	if len(base64Data) > 0 {
		var sb strings.Builder
		db.CreateIssueFromHistoryAndTemplate(item, db.Base64EncodedDataInParameterCode, description, 90, sb.String(), item.WorkspaceID)

	}
}

func ActiveScanHistoryItem(item *db.History, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator, options HistoryItemScanOptions) {
	taskLog := log.With().Uint("workspace", options.WorkspaceID).Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Logger()
	taskLog.Info().Msg("Starting to scan history item")

	fuzzer := fuzz.HttpFuzzer{
		Concurrency:         10,
		InteractionsManager: interactionsManager,
		AvoidRepeatedIssues: viper.GetBool("scan.avoid_repeated_issues"),
		WorkspaceID:         options.WorkspaceID,
	}
	insertionPoints, err := fuzz.GetInsertionPoints(item, options.InsertionPoints)
	taskLog.Debug().Interface("insertionPoints", insertionPoints).Msg("Insertion points")
	if err != nil {
		taskLog.Error().Err(err).Msg("Could not get insertion points")
	}

	if len(insertionPoints) > 0 {
		fuzzer.Run(item, payloadGenerators, insertionPoints)
	}

	cspp := active.ClientSidePrototypePollutionAudit{
		HistoryItem: item,
		WorkspaceID: options.WorkspaceID,
	}
	cspp.Run()

	var specificParamsToTest []string
	p := web.WebPage{URL: item.URL}
	hasParams, _ := p.HasParameters()
	if hasParams && options.IsScopedInsertionPoint("param") {
		// ssrf := active.SSRFAudit{
		// 	URL:                 item.URL,
		// 	ParamsToTest:        specificParamsToTest,
		// 	Concurrency:         5,
		// 	StopAfterSuccess:    false,
		// 	InteractionsManager: interactionsManager,
		// 	WorkspaceID:         options.WorkspaceID,
		// }
		// ssrf.Run()
		active.TestXSS(item.URL, specificParamsToTest, "default.txt", false)
		// ssti := active.SSTIAudit{
		// 	URL:              item.URL,
		// 	Params:           specificParamsToTest,
		// 	Concurrency:      20,
		// 	StopAfterSuccess: false,
		// }
		// ssti.Run()
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
	// }
	// sni.Run()
	methods := active.HTTPMethodsAudit{
		HistoryItem: item,
		Concurrency: 5,
		WorkspaceID: options.WorkspaceID,
	}
	methods.Run()

	if options.IsScopedInsertionPoint("headers") {
		log4shell := active.Log4ShellInjectionAudit{
			URL:                 item.URL,
			Concurrency:         10,
			InteractionsManager: interactionsManager,
			WorkspaceID:         options.WorkspaceID,
		}
		log4shell.Run()
		hostHeader := active.HostHeaderInjectionAudit{
			URL:         item.URL,
			Concurrency: 10,
			WorkspaceID: options.WorkspaceID,
		}
		hostHeader.Run()
	}
	log.Info().Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Msg("Finished scanning history item")
}
