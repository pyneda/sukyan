package cmd

import (
	"github.com/go-playground/validator/v10"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/engine"

	"os"
	"time"

	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var startURLs []string
var crawlDepth int
var crawlMaxPages int
var crawlExcludePatterns []string
var workspaceID uint
var scanTests []string
var scanTitle string
var requestsHeadersString string
var insertionPoints []string
var urlFile string
var scanMode string
var experimentalAudits bool

var validate = validator.New()

// scanCmd represents the audit command
var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Full site scan: including crawl + scan each url",
	Long:  `Runs a configurable audit either to a simple url or to different sites if crawl and multiple initial urls domains are provided`,
	Run: func(cmd *cobra.Command, args []string) {

		if urlFile != "" {
			urlsFromFile, err := lib.ReadFileByLines(urlFile)
			if err != nil {
				log.Error().Err(err).Msg("Failed to read URLs from file")
				os.Exit(1)
			}
			startURLs = append(startURLs, urlsFromFile...)
		}

		startURLs = lib.GetUniqueItems(startURLs)

		if len(startURLs) == 0 {
			log.Error().Msg("At least one crawl starting url should be provided")
			os.Exit(1)
		}

		if !scan.IsValidScanMode(scanMode) {
			log.Error().Str("mode", scanMode).Interface("valid", scan.GetValidScanModes()).Msg("Invalid scan mode")
			os.Exit(1)
		}

		generators, err := generation.LoadGenerators(viper.GetString("generators.directory"))
		if err != nil {
			log.Error().Err(err).Msg("Failed to load generators")
			os.Exit(1)
		}

		workspaceExists, _ := db.Connection.WorkspaceExists(workspaceID)

		if !workspaceExists {
			log.Error().Uint("id", workspaceID).Msg("Workspace does not exist")
			workspaces, count, _ := db.Connection.ListWorkspaces(db.WorkspaceFilters{})
			if count == 0 {
				log.Info().Msg("No workspaces found")
			} else {
				log.Info().Msg("Available workspaces:")
				for _, workspace := range workspaces {
					log.Info().Msgf("ID: %d, Code: %s, Title: %s", workspace.ID, workspace.Code, workspace.Title)
				}
			}
			os.Exit(1)
		}

		headers := lib.ParseHeadersStringToMap(requestsHeadersString)
		log.Info().Interface("headers", headers).Msg("Parsed headers")

		options := scan.FullScanOptions{
			Title:              scanTitle,
			StartURLs:          startURLs,
			MaxDepth:           crawlDepth,
			MaxPagesToCrawl:    crawlMaxPages,
			ExcludePatterns:    crawlExcludePatterns,
			WorkspaceID:        workspaceID,
			PagesPoolSize:      pagesPoolSize,
			Headers:            headers,
			InsertionPoints:    insertionPoints,
			Mode:               scan.GetScanMode(scanMode),
			ExperimentalAudits: experimentalAudits,
		}
		if err := validate.Struct(options); err != nil {
			log.Error().Err(err).Msg("Validation failed")
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
		engine := engine.NewScanEngine(generators, viper.GetInt("scan.concurrency.passive"), viper.GetInt("scan.concurrency.active"), interactionsManager)
		task, _ := engine.FullScan(options, true)
		log.Info().Msg("Scan completed")
		stats, err := db.Connection.GetTaskStatsFromID(uint(task.ID))
		if err != nil {
			log.Error().Err(err).Msg("Failed to get task stats")
		} else {
			log.Info().Interface("stats", stats).Msg("Scan stats")
		}

		oobWait := time.Duration(viper.GetInt("scan.oob.wait_after_scan"))
		log.Info().Msgf("Waiting %d seconds for possible interactions...", oobWait)
		time.Sleep(oobWait * time.Second)
		engine.Stop()
		interactionsManager.Stop()
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringArrayVarP(&startURLs, "url", "u", nil, "Target start url(s)")
	scanCmd.Flags().StringVarP(&urlFile, "file", "f", "", "File containing multiple URLs to scan")
	scanCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	scanCmd.Flags().IntVar(&pagesPoolSize, "pool-size", 4, "Page pool size (not used)")
	scanCmd.Flags().IntVar(&crawlMaxPages, "max-pages", 0, "Max pages to crawl")
	scanCmd.Flags().StringArrayVar(&crawlExcludePatterns, "exclude-pattern", nil, "URL patterns to ignore when crawling")
	scanCmd.Flags().IntVar(&crawlDepth, "depth", 5, "Max crawl depth")
	// scanCmd.Flags().StringArrayVar(&scanTests, "test", nil, "Tests to run (all by default)")
	scanCmd.Flags().StringVarP(&scanTitle, "title", "t", "Scan", "Scan title")
	scanCmd.Flags().StringVar(&requestsHeadersString, "headers", "", "Headers to use for requests")
	scanCmd.Flags().StringVarP(&scanMode, "mode", "m", "smart", "Scan mode (fast, smart, fuzz)")
	scanCmd.Flags().StringArrayVarP(&insertionPoints, "insertion-points", "I", scan.GetValidInsertionPoints(), "Insertion points to scan (all by default)")
	scanCmd.Flags().BoolVar(&experimentalAudits, "experimental", false, "Enable experimental audits")
}
