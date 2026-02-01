package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"

	"github.com/spf13/viper"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var startURLs []string
var crawlDepth int
var crawlMaxPages int
var crawlMaxPagesPerSite int
var crawlExcludePatterns []string
var workspaceID uint
var scanTitle string
var requestsHeadersString string
var insertionPoints []string
var urlFile string
var scanMode string
var experimentalAudits bool
var serverSideChecks bool
var clientSideChecks bool
var passiveChecks bool
var discoveryChecks bool
var websocketChecks bool
var maxRetries int
var workers int
var maxConcurrentJobs int
var maxRPS int
var websocketConcurrency int
var websocketReplayMessages bool
var websocketObservationWindow int
var captureBrowserEvents bool
var httpTimeout int
var httpMaxIdleConns int
var httpMaxIdleConnsPerHost int
var httpMaxConnsPerHost int
var httpDisableKeepAlives bool
var noAPIScan bool

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

		if !scan_options.IsValidScanMode(scanMode) {
			log.Error().Str("mode", scanMode).Interface("valid", scan_options.GetValidScanModes()).Msg("Invalid scan mode")
			os.Exit(1)
		}

		generators, err := generation.LoadGenerators(viper.GetString("generators.directory"))
		if err != nil {
			log.Error().Err(err).Msg("Failed to load generators")
			os.Exit(1)
		}

		workspaceExists, _ := db.Connection().WorkspaceExists(workspaceID)

		if !workspaceExists {
			log.Error().Uint("id", workspaceID).Msg("Workspace does not exist")
			workspaces, count, _ := db.Connection().ListWorkspaces(db.WorkspaceFilters{})
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

		if !serverSideChecks && !clientSideChecks && !passiveChecks {
			log.Warn().Msg("Full scan request received witout audit categories enabled")
			os.Exit(1)
		}

		headers := lib.ParseHeadersStringToMap(requestsHeadersString)
		log.Info().Interface("headers", headers).Msg("Parsed headers")

		var maxConcurrentJobsPtr *int
		var maxRPSPtr *int
		if maxConcurrentJobs > 0 {
			maxConcurrentJobsPtr = &maxConcurrentJobs
		}
		if maxRPS > 0 {
			maxRPSPtr = &maxRPS
		}

		var httpTimeoutPtr *int
		var httpMaxIdleConnsPtr *int
		var httpMaxIdleConnsPerHostPtr *int
		var httpMaxConnsPerHostPtr *int
		var httpDisableKeepAlivesPtr *bool
		if httpTimeout > 0 {
			httpTimeoutPtr = &httpTimeout
		}
		if httpMaxIdleConns > 0 {
			httpMaxIdleConnsPtr = &httpMaxIdleConns
		}
		if httpMaxIdleConnsPerHost > 0 {
			httpMaxIdleConnsPerHostPtr = &httpMaxIdleConnsPerHost
		}
		if httpMaxConnsPerHost > 0 {
			httpMaxConnsPerHostPtr = &httpMaxConnsPerHost
		}
		if httpDisableKeepAlives {
			httpDisableKeepAlivesPtr = &httpDisableKeepAlives
		}

		options := scan_options.FullScanOptions{
			Title:              scanTitle,
			StartURLs:          startURLs,
			MaxDepth:           crawlDepth,
			MaxPagesToCrawl:    crawlMaxPages,
			MaxPagesPerSite:    crawlMaxPagesPerSite,
			ExcludePatterns:    crawlExcludePatterns,
			WorkspaceID:        workspaceID,
			PagesPoolSize:      pagesPoolSize,
			Headers:            headers,
			InsertionPoints:    insertionPoints,
			Mode:               scan_options.GetScanMode(scanMode),
			ExperimentalAudits: experimentalAudits,
			AuditCategories: scan_options.AuditCategories{
				ServerSide: serverSideChecks,
				ClientSide: clientSideChecks,
				Passive:    passiveChecks,
				Discovery:  discoveryChecks,
				WebSocket:  websocketChecks,
			},
			WebSocketOptions: scan_options.FullScanWebSocketOptions{
				Concurrency:       websocketConcurrency,
				ReplayMessages:    websocketReplayMessages,
				ObservationWindow: websocketObservationWindow,
			},
			APIScanOptions: scan_options.FullScanAPIScanOptions{
				Enabled:             !noAPIScan,
				RunAPISpecificTests: true,
				RunStandardTests:    true,
			},
			MaxRetries:              maxRetries,
			MaxConcurrentJobs:       maxConcurrentJobsPtr,
			MaxRPS:                  maxRPSPtr,
			CaptureBrowserEvents:    captureBrowserEvents,
			HTTPTimeout:             httpTimeoutPtr,
			HTTPMaxIdleConns:        httpMaxIdleConnsPtr,
			HTTPMaxIdleConnsPerHost: httpMaxIdleConnsPerHostPtr,
			HTTPMaxConnsPerHost:     httpMaxConnsPerHostPtr,
			HTTPDisableKeepAlives:   httpDisableKeepAlivesPtr,
		}
		if err := validate.Struct(options); err != nil {
			log.Error().Err(err).Msg("Validation failed")
			os.Exit(1)
		}

		// Setup OOB interactions manager
		oobPollingInterval := time.Duration(viper.GetInt("scan.oob.poll_interval"))
		oobKeepAliveInterval := time.Duration(viper.GetInt("scan.oob.keep_alive_interval"))
		oobSessionFile := viper.GetString("scan.oob.session_file")

		log.Info().Strs("urls", startURLs).Int("count", len(startURLs)).Msg("Starting the scan")

		interactionsManager := &integrations.InteractionsManager{
			GetAsnInfo:            false,
			PollingInterval:       oobPollingInterval * time.Second,
			KeepAliveInterval:     oobKeepAliveInterval * time.Second,
			SessionFile:           oobSessionFile,
			OnInteractionCallback: scan.SaveInteractionCallback,
		}
		interactionsManager.OnEvictionCallback = func() {
			log.Warn().Msg("Interactsh correlation ID evicted, restarting client")
			interactionsManager.Restart()
		}
		interactionsManager.Start()

		// Create scan record FIRST (before starting manager) for proper isolation.
		// This ensures workers are configured with the scan ID filter from the start,
		// preventing race conditions where other workers could claim our jobs.
		// The isolated=true flag ensures API workers won't claim jobs from this scan.
		scanEntity, err := manager.CreateScanRecord(db.Connection(), options, true, db.ScanStatusPending)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create scan")
			interactionsManager.Stop()
			os.Exit(1)
		}
		log.Info().Uint("scan_id", scanEntity.ID).Msg("Created scan for isolated mode")

		cfg := manager.DefaultConfig()
		// Use CLI-provided workers, fallback to config, then default to 5
		if workers > 0 {
			cfg.WorkerCount = workers
		} else {
			cfg.WorkerCount = viper.GetInt("scan.workers")
			if cfg.WorkerCount < 1 {
				cfg.WorkerCount = 5
			}
		}

		// Configure manager with scan ID filter from the start
		cfg.ScanID = &scanEntity.ID

		scanManager := manager.New(cfg, db.Connection(), interactionsManager, generators)
		if err := scanManager.Start(); err != nil {
			log.Error().Err(err).Msg("Failed to start scan manager")
			os.Exit(1)
		}

		// Start the scan through orchestrator (workers already have filter configured)
		if err := scanManager.StartScan(scanEntity.ID); err != nil {
			log.Error().Err(err).Msg("Failed to start scan")
			scanManager.Stop()
			interactionsManager.Stop()
			os.Exit(1)
		}
		log.Info().Uint("scan_id", scanEntity.ID).Msg("Scan started in isolated mode")

		// Setup signal handler for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		// Poll for scan completion
		pollInterval := 2 * time.Second
		scanLog := log.With().Uint("scan_id", scanEntity.ID).Logger()

		scanCompleted := false
		for !scanCompleted {
			select {
			case sig := <-sigChan:
				scanLog.Warn().Str("signal", sig.String()).Msg("Received signal, cancelling scan")
				if err := scanManager.CancelScan(scanEntity.ID); err != nil {
					scanLog.Error().Err(err).Msg("Failed to cancel scan")
				}
				scanManager.Stop()
				interactionsManager.Stop()
				os.Exit(1)

			case <-time.After(pollInterval):
				// Check scan status
				updatedScan, err := db.Connection().GetScanByID(scanEntity.ID)
				if err != nil {
					scanLog.Error().Err(err).Msg("Failed to get scan status")
					continue
				}

				switch updatedScan.Status {
				case db.ScanStatusCompleted:
					scanLog.Info().Msg("Scan completed successfully")
					scanCompleted = true
				case db.ScanStatusFailed:
					scanLog.Error().Msg("Scan failed")
					scanCompleted = true
				case db.ScanStatusCancelled:
					scanLog.Warn().Msg("Scan was cancelled")
					scanCompleted = true
				default:
					// Log progress
					stats, _ := db.Connection().GetScanJobStats(scanEntity.ID)
					if stats != nil {
						scanLog.Info().
							Str("status", string(updatedScan.Status)).
							Int64("pending", stats[db.ScanJobStatusPending]).
							Int64("running", stats[db.ScanJobStatusRunning]).
							Int64("completed", stats[db.ScanJobStatusCompleted]).
							Int64("failed", stats[db.ScanJobStatusFailed]).
							Msg("Scan progress")
					}
				}
			}
		}

		// Get final scan stats
		finalScan, _ := db.Connection().GetScanByID(scanEntity.ID)
		if finalScan != nil {
			scanLog.Info().
				Int("pending_jobs", finalScan.PendingJobsCount).
				Int("running_jobs", finalScan.RunningJobsCount).
				Int("completed_jobs", finalScan.CompletedJobsCount).
				Int("failed_jobs", finalScan.FailedJobsCount).
				Msg("Final scan statistics")
		}

		// Wait for OOB interactions
		oobWait := time.Duration(viper.GetInt("scan.oob.wait_after_scan"))
		log.Info().Msgf("Waiting %d seconds for possible OOB interactions...", oobWait)
		time.Sleep(oobWait * time.Second)

		// Cleanup
		scanManager.Stop()
		interactionsManager.Stop()
		log.Info().Msg("Scan finished")
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
	scanCmd.Flags().StringArrayVarP(&startURLs, "url", "u", nil, "Target start url(s)")
	scanCmd.Flags().StringVarP(&urlFile, "file", "f", "", "File containing multiple URLs to scan")
	scanCmd.Flags().UintVarP(&workspaceID, "workspace", "w", 0, "Workspace ID")
	scanCmd.Flags().IntVar(&pagesPoolSize, "pool-size", 4, "Page pool size (not used)")
	scanCmd.Flags().IntVar(&crawlMaxPages, "max-pages", 0, "Max pages to crawl (global limit)")
	scanCmd.Flags().IntVar(&crawlMaxPagesPerSite, "max-pages-per-site", 0, "Max pages to crawl per site (scheme://host:port)")
	scanCmd.Flags().StringArrayVar(&crawlExcludePatterns, "exclude-pattern", nil, "URL patterns to ignore when crawling")
	scanCmd.Flags().IntVar(&crawlDepth, "depth", 5, "Max crawl depth")
	scanCmd.Flags().StringVarP(&scanTitle, "title", "t", "Scan", "Scan title")
	scanCmd.Flags().StringVar(&requestsHeadersString, "headers", "", "Headers to use for requests")
	scanCmd.Flags().StringVarP(&scanMode, "mode", "m", "smart", "Scan mode (fast, smart, fuzz)")
	scanCmd.Flags().StringArrayVarP(&insertionPoints, "insertion-points", "I", scan_options.GetValidInsertionPoints(), "Insertion points to scan (all by default)")
	scanCmd.Flags().BoolVar(&experimentalAudits, "experimental", false, "Enable experimental audits")
	scanCmd.Flags().BoolVar(&serverSideChecks, "server-side", true, "Enable server-side audits")
	scanCmd.Flags().BoolVar(&clientSideChecks, "client-side", true, "Enable client-side audits")
	scanCmd.Flags().BoolVar(&passiveChecks, "passive", true, "Enable passive audits")
	scanCmd.Flags().BoolVar(&discoveryChecks, "discovery", true, "Enable content discovery audits")
	scanCmd.Flags().BoolVar(&websocketChecks, "websocket", true, "Enable WebSocket audits")
	scanCmd.Flags().IntVar(&maxRetries, "max-retries", 3, "Maximum number of retries for failed requests (default: 3)")
	scanCmd.Flags().IntVar(&workers, "workers", 0, "Number of concurrent workers (0 uses config setting, defaults to 5)")
	scanCmd.Flags().IntVar(&maxConcurrentJobs, "max-concurrent-jobs", 0, "Maximum concurrent jobs across all workers (0 for unlimited)")
	scanCmd.Flags().IntVar(&maxRPS, "max-rps", 0, "Maximum requests per second (0 for unlimited)")
	scanCmd.Flags().IntVar(&websocketConcurrency, "websocket-concurrency", 1, "WebSocket concurrency level (1-100)")
	scanCmd.Flags().BoolVar(&websocketReplayMessages, "websocket-replay-messages", false, "Replay WebSocket messages")
	scanCmd.Flags().IntVar(&websocketObservationWindow, "websocket-observation-window", 10, "WebSocket observation window in seconds (1-100)")
	scanCmd.Flags().BoolVar(&captureBrowserEvents, "capture-browser-events", false, "Capture and store browser events (console, storage, security, etc.) during scanning")

	// HTTP Client Configuration
	scanCmd.Flags().IntVar(&httpTimeout, "http-timeout", 0, "HTTP client timeout in seconds (0 = use global default)")
	scanCmd.Flags().IntVar(&httpMaxIdleConns, "http-max-idle-conns", 0, "Max idle connections total (0 = use global default)")
	scanCmd.Flags().IntVar(&httpMaxIdleConnsPerHost, "http-max-idle-conns-per-host", 0, "Max idle connections per host (0 = use global default)")
	scanCmd.Flags().IntVar(&httpMaxConnsPerHost, "http-max-conns-per-host", 0, "Max concurrent connections per host (0 = use global default)")
	scanCmd.Flags().BoolVar(&httpDisableKeepAlives, "http-disable-keep-alives", false, "Disable HTTP keep-alives")

	// API Scan Configuration
	scanCmd.Flags().BoolVar(&noAPIScan, "no-api-scan", false, "Disable API scanning phase (for discovered API definitions)")
}
