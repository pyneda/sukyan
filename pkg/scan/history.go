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

func ActiveScanHistoryItem(item *db.History, interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator) {
	log.Info().Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Msg("Starting to scan history item")
	fuzzer := fuzz.HttpFuzzer{
		Concurrency:         10,
		InteractionsManager: interactionsManager,
	}
	insertionPoints, _ := fuzz.GetInsertionPoints(item)
	fuzzer.Run(item, payloadGenerators, insertionPoints)

	var specificParamsToTest []string
	p := web.WebPage{URL: item.URL}
	hasParams, _ := p.HasParameters()
	if hasParams && viper.GetBool("scan.insertion_points.parameters") {
		ssrf := active.SSRFAudit{
			URL:                 item.URL,
			ParamsToTest:        specificParamsToTest,
			Concurrency:         5,
			StopAfterSuccess:    false,
			InteractionsManager: interactionsManager,
		}
		ssrf.Run()
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

	sni := active.SNIAudit{
		HistoryItem:         item,
		InteractionsManager: interactionsManager,
	}
	sni.Run()
	methods := active.HTTPMethodsAudit{
		HistoryItem: item,
		Concurrency: 5,
	}
	methods.Run()

	if viper.GetBool("scan.insertion_points.headers") {
		log4shell := active.Log4ShellInjectionAudit{
			URL:                 item.URL,
			Concurrency:         10,
			InteractionsManager: interactionsManager,
		}
		log4shell.Run()
		hostHeader := active.HostHeaderInjectionAudit{
			URL:         item.URL,
			Concurrency: 10,
		}
		hostHeader.Run()
	}
	log.Info().Str("item", item.URL).Str("method", item.Method).Int("ID", int(item.ID)).Msg("Finished scanning history item")
}
