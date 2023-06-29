package cmd

import (
	"fmt"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"

	"github.com/spf13/viper"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var startURLs []string
var crawlDepth int

// scanCmd represents the audit command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Full site scan: including crawl + scan each url",
	Long:  `Runs a configurable audit either to a simple url or to different sites if crawl and multiple initial urls domains are provided`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("audit called")

		if len(startURLs) == 0 {
			log.Error().Msg("At least one crawl starting url should be provided")
			os.Exit(1)
		}
		generators, err := generation.LoadGenerators(viper.GetString("generators.directory"))
		if err != nil {
			log.Error().Err(err).Msg("Failed to load generators")
			os.Exit(1)
		}
		oobPollingInterval := time.Duration(viper.GetInt("scan.oob.poll_interval"))
		log.Info().Strs("urls", startURLs).Int("count", len(startURLs)).Msg("Starting the audit")
		interactionsManager := &integrations.InteractionsManager{
			GetAsnInfo:            false,
			PollingInterval:       oobPollingInterval * time.Second,
			OnInteractionCallback: scan.SaveInteractionCallback,
		}
		interactionsManager.Start()
		engine := scan.NewScanEngine(generators, 100, 30, interactionsManager)
		engine.Start()

		engine.CrawlAndAudit(startURLs, 0, crawlDepth, pagesPoolSize, true)
		oobWait := time.Duration(viper.GetInt("scan.oob.wait_after_scan"))
		log.Info().Msgf("Audit finished, waiting %d seconds for possible interactions...", oobWait)
		time.Sleep(oobWait * time.Second)
		engine.Stop()
		interactionsManager.Stop()
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringArrayVarP(&startURLs, "url", "u", nil, "Target start url(s)")
	// scanCmd.Flags().BoolVar(&cmdScanner.AuditHeaders, "audit-headers", true, "Audit HTTP headers")
	// scanCmd.Flags().BoolVar(&cmdScanner.DiscoverParams, "param-discovery", true, "Enables parameter discovery (Not implemented yet)")
	// scanCmd.Flags().BoolVar(&cmdScanner.ShouldCrawl, "crawl", false, "Enables the crawler")
	scanCmd.Flags().IntVar(&pagesPoolSize, "pool-size", 4, "Page pool size (not used)")
	scanCmd.Flags().IntVar(&crawlDepth, "depth", 5, "Max crawl depth")
}
