package cmd

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	apidefsScanURL           string
	apidefsScanFile          string
	apidefsScanWorkspaceID   uint
	apidefsScanName          string
	apidefsScanAuthType      string
	apidefsScanUsername      string
	apidefsScanPassword      string
	apidefsScanToken         string
	apidefsScanAPIKeyName    string
	apidefsScanAPIKeyValue   string
	apidefsScanAPIKeyIn      string
	apidefsScanBaseURL       string
	apidefsScanEndpoints     []string
	apidefsScanNoAPITests    bool
	apidefsScanNoStandard    bool
	apidefsScanMode          string
	apidefsScanWorkers       int
	apidefsScanServerSide    bool
	apidefsScanClientSide    bool
	apidefsScanPassive       bool
)

var apidefsScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Parse and scan an API definition",
	Long: `Parse an API definition (OpenAPI, GraphQL, WSDL) and scan all endpoints.

The definition can be provided via URL or local file. Authentication credentials
can be specified to access protected APIs.

Examples:
  # Scan OpenAPI definition from URL
  sukyan apidefs scan --url https://api.example.com/openapi.json -w 1

  # Scan with bearer token authentication
  sukyan apidefs scan --url https://api.example.com/openapi.json -w 1 \
      --auth-type bearer --token "eyJhbGciOiJIUzI1NiIs..."

  # Scan with API key in header
  sukyan apidefs scan --url https://api.example.com/v1/openapi.yaml -w 1 \
      --auth-type api_key --api-key-name "X-API-Key" --api-key-value "secret123"

  # Scan from local file
  sukyan apidefs scan --file ./openapi.yaml -w 1 --name "My API"`,
	Run: runAPIDefsScan,
}

func init() {
	apidefsCmd.AddCommand(apidefsScanCmd)

	apidefsScanCmd.Flags().StringVarP(&apidefsScanURL, "url", "u", "", "API definition URL")
	apidefsScanCmd.Flags().StringVarP(&apidefsScanFile, "file", "f", "", "Local file path")
	apidefsScanCmd.Flags().UintVarP(&apidefsScanWorkspaceID, "workspace", "w", 0, "Workspace ID")
	apidefsScanCmd.Flags().StringVarP(&apidefsScanName, "name", "n", "", "Name for the API definition")
	apidefsScanCmd.Flags().StringVar(&apidefsScanAuthType, "auth-type", "none", "Auth type: none, basic, bearer, api_key")
	apidefsScanCmd.Flags().StringVar(&apidefsScanUsername, "username", "", "Basic auth username")
	apidefsScanCmd.Flags().StringVar(&apidefsScanPassword, "password", "", "Basic auth password")
	apidefsScanCmd.Flags().StringVar(&apidefsScanToken, "token", "", "Bearer token")
	apidefsScanCmd.Flags().StringVar(&apidefsScanAPIKeyName, "api-key-name", "", "API key name")
	apidefsScanCmd.Flags().StringVar(&apidefsScanAPIKeyValue, "api-key-value", "", "API key value")
	apidefsScanCmd.Flags().StringVar(&apidefsScanAPIKeyIn, "api-key-in", "header", "API key location: header, query, cookie")
	apidefsScanCmd.Flags().StringVar(&apidefsScanBaseURL, "base-url", "", "Override base URL from definition")
	apidefsScanCmd.Flags().StringSliceVar(&apidefsScanEndpoints, "endpoints", nil, "Specific endpoint IDs to scan (optional)")
	apidefsScanCmd.Flags().BoolVar(&apidefsScanNoAPITests, "no-api-tests", false, "Skip API-specific tests")
	apidefsScanCmd.Flags().BoolVar(&apidefsScanNoStandard, "no-standard", false, "Skip standard vulnerability tests")
	apidefsScanCmd.Flags().StringVarP(&apidefsScanMode, "mode", "m", "smart", "Scan mode: fast, smart, fuzz")
	apidefsScanCmd.Flags().IntVar(&apidefsScanWorkers, "workers", 0, "Number of concurrent workers (0 uses config)")
	apidefsScanCmd.Flags().BoolVar(&apidefsScanServerSide, "server-side", true, "Enable server-side checks")
	apidefsScanCmd.Flags().BoolVar(&apidefsScanClientSide, "client-side", false, "Enable client-side checks")
	apidefsScanCmd.Flags().BoolVar(&apidefsScanPassive, "passive", true, "Enable passive checks")

	apidefsScanCmd.MarkFlagRequired("workspace")
}

func runAPIDefsScan(cmd *cobra.Command, args []string) {
	logger := log.With().Str("component", "apidefs-scan").Logger()

	if apidefsScanURL == "" && apidefsScanFile == "" {
		logger.Error().Msg("Either --url or --file must be provided")
		os.Exit(1)
	}

	if apidefsScanURL != "" && apidefsScanFile != "" {
		logger.Error().Msg("Cannot provide both --url and --file")
		os.Exit(1)
	}

	if !scan_options.IsValidScanMode(apidefsScanMode) {
		logger.Error().Str("mode", apidefsScanMode).Msg("Invalid scan mode")
		os.Exit(1)
	}

	workspaceExists, _ := db.Connection().WorkspaceExists(apidefsScanWorkspaceID)
	if !workspaceExists {
		logger.Error().Uint("id", apidefsScanWorkspaceID).Msg("Workspace does not exist")
		os.Exit(1)
	}

	content, sourceURL, err := loadAPIDefinitionContent(apidefsScanURL, apidefsScanFile)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load API definition")
		os.Exit(1)
	}

	apiType := detectAPIDefinitionType(content, sourceURL)
	logger.Info().Str("type", string(apiType)).Msg("Detected API type")

	var authConfig *db.APIAuthConfig
	if apidefsScanAuthType != "none" {
		authConfig, err = createAuthConfig(apidefsScanWorkspaceID, apiDefsAuthParams{
			AuthType:    apidefsScanAuthType,
			Username:    apidefsScanUsername,
			Password:    apidefsScanPassword,
			Token:       apidefsScanToken,
			APIKeyName:  apidefsScanAPIKeyName,
			APIKeyValue: apidefsScanAPIKeyValue,
			APIKeyIn:    apidefsScanAPIKeyIn,
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to create auth config")
			os.Exit(1)
		}
		logger.Info().Str("auth_type", apidefsScanAuthType).Msg("Created auth configuration")
	}

	definition, err := parseAndPersistDefinition(content, sourceURL, apiType, apidefsScanWorkspaceID, authConfig)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse and store API definition")
		os.Exit(1)
	}

	if apidefsScanName != "" {
		definition.Name = apidefsScanName
		db.Connection().UpdateAPIDefinition(definition)
	}

	if apidefsScanBaseURL != "" {
		definition.BaseURL = apidefsScanBaseURL
		db.Connection().UpdateAPIDefinition(definition)
	}

	logger.Info().
		Str("definition_id", definition.ID.String()).
		Str("name", definition.Name).
		Int("endpoints", definition.EndpointCount).
		Msg("API definition stored")

	generators, err := generation.LoadGenerators(viper.GetString("generators.directory"))
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load generators")
		os.Exit(1)
	}

	oobPollingInterval := time.Duration(viper.GetInt("scan.oob.poll_interval"))
	oobKeepAliveInterval := time.Duration(viper.GetInt("scan.oob.keep_alive_interval"))
	oobSessionFile := viper.GetString("scan.oob.session_file")

	interactionsManager := &integrations.InteractionsManager{
		GetAsnInfo:            false,
		PollingInterval:       oobPollingInterval * time.Second,
		KeepAliveInterval:     oobKeepAliveInterval * time.Second,
		SessionFile:           oobSessionFile,
		OnInteractionCallback: scan.SaveInteractionCallback,
	}
	interactionsManager.OnEvictionCallback = func() {
		logger.Warn().Msg("Interactsh correlation ID evicted, restarting client")
		interactionsManager.Restart()
	}
	interactionsManager.Start()

	scanTitle := "API Scan - " + definition.Name
	scanOptions := scan_options.FullScanOptions{
		Title:       scanTitle,
		StartURLs:   []string{definition.BaseURL},
		WorkspaceID: apidefsScanWorkspaceID,
		Mode:        scan_options.GetScanMode(apidefsScanMode),
		AuditCategories: scan_options.AuditCategories{
			ServerSide: apidefsScanServerSide,
			ClientSide: apidefsScanClientSide,
			Passive:    apidefsScanPassive,
		},
		APIScanOptions: scan_options.FullScanAPIScanOptions{
			Enabled:             true,
			RunAPISpecificTests: !apidefsScanNoAPITests,
			RunStandardTests:    !apidefsScanNoStandard,
		},
	}

	scanEntity, err := manager.CreateScanRecord(db.Connection(), scanOptions, true, db.ScanStatusPending)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create scan")
		interactionsManager.Stop()
		os.Exit(1)
	}

	definition.ScanID = &scanEntity.ID
	db.Connection().UpdateAPIDefinition(definition)

	if err := db.Connection().LinkAPIDefinitionToScan(scanEntity.ID, definition.ID); err != nil {
		logger.Warn().Err(err).Msg("Failed to link API definition to scan")
	}

	logger.Info().
		Uint("scan_id", scanEntity.ID).
		Str("definition_id", definition.ID.String()).
		Msg("Created scan for API definition")

	cfg := manager.DefaultConfig()
	if apidefsScanWorkers > 0 {
		cfg.WorkerCount = apidefsScanWorkers
	} else {
		cfg.WorkerCount = viper.GetInt("scan.workers")
		if cfg.WorkerCount < 1 {
			cfg.WorkerCount = 5
		}
	}
	cfg.ScanID = &scanEntity.ID

	scanManager := manager.New(cfg, db.Connection(), interactionsManager, generators)
	if err := scanManager.Start(); err != nil {
		logger.Error().Err(err).Msg("Failed to start scan manager")
		os.Exit(1)
	}

	if err := scanManager.StartScan(scanEntity.ID); err != nil {
		logger.Error().Err(err).Msg("Failed to start scan")
		scanManager.Stop()
		interactionsManager.Stop()
		os.Exit(1)
	}

	logger.Info().Uint("scan_id", scanEntity.ID).Msg("API scan started")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

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
			updatedScan, err := db.Connection().GetScanByID(scanEntity.ID)
			if err != nil {
				scanLog.Error().Err(err).Msg("Failed to get scan status")
				continue
			}

			switch updatedScan.Status {
			case db.ScanStatusCompleted:
				scanLog.Info().Msg("API scan completed successfully")
				scanCompleted = true
			case db.ScanStatusFailed:
				scanLog.Error().Msg("API scan failed")
				scanCompleted = true
			case db.ScanStatusCancelled:
				scanLog.Warn().Msg("API scan was cancelled")
				scanCompleted = true
			default:
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

	finalScan, _ := db.Connection().GetScanByID(scanEntity.ID)
	if finalScan != nil {
		scanLog.Info().
			Int("pending_jobs", finalScan.PendingJobsCount).
			Int("completed_jobs", finalScan.CompletedJobsCount).
			Int("failed_jobs", finalScan.FailedJobsCount).
			Msg("Final scan statistics")
	}

	oobWait := time.Duration(viper.GetInt("scan.oob.wait_after_scan"))
	logger.Info().Msgf("Waiting %d seconds for possible OOB interactions...", oobWait)
	time.Sleep(oobWait * time.Second)

	scanManager.Stop()
	interactionsManager.Stop()
	logger.Info().Msg("API scan finished")
}




