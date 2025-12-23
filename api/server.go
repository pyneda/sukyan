package api

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/contrib/fiberzerolog"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/pprof"
	"github.com/gofiber/swagger"
	"github.com/pyneda/sukyan/db"
	_ "github.com/pyneda/sukyan/docs"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// @title Sukyan API
// @version 0.1
// @description The Sukyan API documentation.
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
func StartAPI() {
	apiLogger := log.With().Str("type", "api").Logger()

	apiLogger.Info().Msg("Initializing...")
	generators, err := generation.LoadGenerators(viper.GetString("generators.directory"))
	if err != nil {
		apiLogger.Error().Err(err).Msg("Failed to load generators")
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
	// Set up auto-recovery on eviction
	interactionsManager.OnEvictionCallback = func() {
		apiLogger.Warn().Msg("Interactsh correlation ID evicted, restarting client")
		interactionsManager.Restart()
	}
	interactionsManager.Start()

	// Initialize the scan manager (orchestrator / Scan Engine V2)
	if err := InitScanManager(interactionsManager, generators); err != nil {
		apiLogger.Warn().Err(err).Msg("Failed to initialize scan manager - orchestrator scans will not be available")
	}

	apiLogger.Info().Msg("Initialized everything. Starting the API...")

	app := fiber.New(fiber.Config{
		// Prefork:       true,
		// CaseSensitive: true,
		// StrictRouting: true,
		ServerHeader: "Sukyan",
		AppName:      "Sukyan API",
	})

	// This allows all cors, should probably allow configure it via config and provide strict default
	// app.Use(cors.Default())
	// app.LoadHTMLGlob("templates/*")
	app.Use(cors.New(cors.Config{
		AllowOrigins:  strings.Join(viper.GetStringSlice("api.cors.origins"), ","),
		AllowHeaders:  "Origin, Content-Type, Accept, Authorization",
		ExposeHeaders: "Content-Disposition",
	}))

	app.Use(fiberzerolog.New(fiberzerolog.Config{
		Logger: &apiLogger,
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("API Running")
	})

	if viper.GetBool("api.docs.enabled") {
		app.Get(fmt.Sprintf("%v/*", viper.GetString("api.docs.path")), swagger.HandlerDefault)
	}

	if viper.GetBool("api.metrics.enabled") {
		app.Get(fmt.Sprintf("%v/*", viper.GetString("api.metrics.path")), monitor.New(monitor.Config{Title: viper.GetString("api.metrics.title")}))
	}

	if viper.GetBool("api.pprof.enabled") {
		app.Use(pprof.New(pprof.Config{Prefix: viper.GetString("api.pprof.prefix")}))
	}

	api := app.Group("/api/v1")
	api.Get("/history", JWTProtected(), FindHistory)
	api.Post("/history", JWTProtected(), FindHistoryPost)
	api.Get("/history/:id", JWTProtected(), GetHistoryDetail)
	api.Get("/issues", JWTProtected(), FindIssues)
	api.Get("/issues/grouped", JWTProtected(), FindIssuesGrouped)
	api.Get("/issues/:id", JWTProtected(), GetIssueDetail)
	api.Post("/issues/:id/set-false-positive", SetFalsePositive)
	api.Get("/history/:id/children", JWTProtected(), GetChildren)
	api.Get("/history/root-nodes", JWTProtected(), GetRootNodes)
	api.Get("/history/websocket/connections/:id", JWTProtected(), FindWebSocketConnectionByID)
	api.Get("/history/websocket/connections", JWTProtected(), FindWebSocketConnections)
	api.Get("/history/websocket/messages", JWTProtected(), FindWebSocketMessages)
	api.Get("/workspaces", JWTProtected(), FindWorkspaces)
	api.Post("/workspaces", JWTProtected(), CreateWorkspace)
	api.Get("/workspaces/:id", JWTProtected(), GetWorkspaceDetail)
	api.Delete("/workspaces/:id", JWTProtected(), DeleteWorkspace)
	api.Put("/workspaces/:id", JWTProtected(), UpdateWorkspace)
	api.Get("/interactions", JWTProtected(), FindInteractions)
	api.Get("/interactions/:id", JWTProtected(), GetInteractionDetail)
	api.Post("/oob-tests", JWTProtected(), FindOOBTests)
	api.Get("/oob-tests/:id", JWTProtected(), GetOOBTestDetail)
	api.Get("/tasks", JWTProtected(), FindTasks)
	api.Get("/tasks/jobs", JWTProtected(), FindTaskJobs)
	api.Post("/tokens/jwts", JWTProtected(), JwtListHandler)
	api.Post("/report", JWTProtected(), ReportHandler)
	api.Get("/sitemap", JWTProtected(), GetSitemap)
	api.Post("/playground/replay", JWTProtected(), ReplayRequest)
	api.Post("/playground/fuzz", JWTProtected(), FuzzRequest)
	api.Get("/playground/collections/:id", JWTProtected(), GetPlaygroundCollection)
	api.Get("/playground/collections", JWTProtected(), ListPlaygroundCollections)
	api.Post("/playground/collections", JWTProtected(), CreatePlaygroundCollection)
	api.Get("/playground/sessions/:id", JWTProtected(), GetPlaygroundSession)
	api.Get("/playground/sessions", JWTProtected(), ListPlaygroundSessions)
	api.Post("/playground/sessions", JWTProtected(), CreatePlaygroundSession)
	api.Put("/playground/sessions/:id", JWTProtected(), UpdatePlaygroundSession)
	api.Get("/playground/wordlists", JWTProtected(), ListAvailableWordlists)

	api.Post("/playground/openapi/parse", JWTProtected(), ParseOpenAPISpec)
	api.Post("/playground/graphql/parse", JWTProtected(), ParseGraphQLSchema)
	api.Post("/playground/graphql/parse-introspection", JWTProtected(), ParseGraphQLFromIntrospection)
	api.Post("/playground/wsdl/parse", JWTProtected(), ParseWSDL)
	api.Post("/playground/wsdl/parse-content", JWTProtected(), ParseWSDLFromBytes)
	api.Get("/stats/workspace", JWTProtected(), WorkspaceStats)
	api.Get("/stats/system", JWTProtected(), SystemStats)
	api.Get("/stats/workers", JWTProtected(), ListWorkerNodes)
	api.Post("/stats/workers/cleanup", JWTProtected(), CleanupStaleWorkers)
	api.Post("/browser-actions", JWTProtected(), CreateStoredBrowserActions)
	api.Get("/browser-actions", JWTProtected(), ListStoredBrowserActions)
	api.Get("/browser-actions/:id", JWTProtected(), GetStoredBrowserActions)
	api.Put("/browser-actions/:id", JWTProtected(), UpdateStoredBrowserActions)
	api.Delete("/browser-actions/:id", JWTProtected(), DeleteStoredBrowserActions)

	// Browser events endpoints
	api.Get("/browser-events", JWTProtected(), FindBrowserEvents)
	api.Get("/browser-events/stats", JWTProtected(), GetBrowserEventStats)
	api.Get("/browser-events/:id", JWTProtected(), GetBrowserEventByID)

	// Knowledge base endpoints
	api.Get("/kb/issues", JWTProtected(), ListIssueTemplates)

	// Auth related endpoints
	auth_app := api.Group("/auth")
	auth_app.Use(limiter.New(limiter.Config{
		Max:               20,
		Expiration:        30 * time.Second,
		LimiterMiddleware: limiter.SlidingWindow{},
	}))

	auth_app.Post("/token/renew", JWTProtected(), RenewTokens)
	auth_app.Post("/user/sign/out", JWTProtected(), UserSignOut)
	auth_app.Post("/user/sign/in", UserSignIn)

	// Make a group for all scan endpoints which require the scan engine
	scan_app := api.Group("/scan")
	scan_app.Use(func(c *fiber.Ctx) error {
		// c.Locals("engine", engine)
		c.Locals("generators", generators)
		c.Locals("interactionsManager", interactionsManager)
		return c.Next()
	})

	scan_app.Post("/full", JWTProtected(), FullScanHandler)
	scan_app.Post("/passive", JWTProtected(), PassiveScanHandler)
	scan_app.Post("/active", JWTProtected(), ActiveScanHandler)
	scan_app.Post("/active/websocket", JWTProtected(), ActiveWebSocketScanHandler)
	scan_app.Get("/options/platforms", JWTProtected(), GetScanOptionsPlatforms)
	scan_app.Get("/options/categories", JWTProtected(), GetScanOptionsCategories)

	// Scans endpoints (Scan Engine V2 / Orchestrator)
	scans_app := api.Group("/scans")
	scans_app.Get("", JWTProtected(), ListScansHandler)
	scans_app.Get("/:id", JWTProtected(), GetScanHandler)
	scans_app.Patch("/:id", JWTProtected(), UpdateScanHandler)
	scans_app.Delete("/:id", JWTProtected(), DeleteScanHandler)
	scans_app.Post("/:id/cancel", JWTProtected(), CancelScanHandler)
	scans_app.Post("/:id/pause", JWTProtected(), PauseScanHandler)
	scans_app.Post("/:id/resume", JWTProtected(), ResumeScanHandler)
	scans_app.Get("/:id/jobs", JWTProtected(), GetScanJobsHandler)
	scans_app.Get("/:id/jobs/:job_id", JWTProtected(), GetScanJobHandler)
	scans_app.Post("/:id/jobs/:job_id/cancel", JWTProtected(), CancelScanJobHandler)
	scans_app.Get("/:id/stats", JWTProtected(), GetScanStatsHandler)
	scans_app.Post("/:id/schedule-items", JWTProtected(), ScheduleHistoryItemScansHandler)

	// Dashboard endpoints (separate from API using basic auth)
	if viper.GetBool("api.dashboard.enabled") {
		dashboardPath := viper.GetString("api.dashboard.path")
		if dashboardPath == "" {
			dashboardPath = "/dashboard"
		}

		dashboardMiddleware := []fiber.Handler{DashboardBasicAuth()}

		// Register dashboard routes
		app.Get(dashboardPath, append(dashboardMiddleware, DashboardHTMLHandler)...)
		app.Get(dashboardPath+"/stats", append(dashboardMiddleware, GetDashboardStatsHandler)...)
	}

	certPath := viper.GetString("server.cert.file")
	keyPath := viper.GetString("server.key.file")
	caCertPath := viper.GetString("server.caCert.file")
	caKeyPath := viper.GetString("server.caKey.file")

	_, _, err = lib.EnsureCertificatesExist(certPath, keyPath, caCertPath, caKeyPath)
	if err != nil {
		apiLogger.Error().Err(err).Msg("Failed to load or generate certificates")

	}

	// Set up graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in a goroutine
	go func() {
		listen_addres := fmt.Sprintf("%v:%v", viper.Get("api.listen.host"), viper.Get("api.listen.port"))
		if err := app.ListenTLS(listen_addres, certPath, keyPath); err != nil {
			apiLogger.Warn().Err(err).Msg("Error starting server")
		}
	}()

	apiLogger.Info().
		Str("host", viper.GetString("api.listen.host")).
		Int("port", viper.GetInt("api.listen.port")).
		Msg("API server started")

	// Wait for shutdown signal
	sig := <-sigCh
	apiLogger.Info().Str("signal", sig.String()).Msg("Received shutdown signal, starting graceful shutdown...")

	// Graceful shutdown sequence
	// 1. Stop accepting new requests
	if err := app.Shutdown(); err != nil {
		apiLogger.Warn().Err(err).Msg("Error during server shutdown")
	}

	// 2. Stop the scan manager (this releases jobs and deregisters workers)
	if sm := GetScanManager(); sm != nil {
		apiLogger.Info().Msg("Stopping scan manager...")
		sm.Stop()
		apiLogger.Info().Msg("Scan manager stopped")
	}

	// 3. Stop interactions manager
	if interactionsManager != nil {
		apiLogger.Info().Msg("Stopping interactions manager...")
		interactionsManager.Stop()
		apiLogger.Info().Msg("Interactions manager stopped")
	}

	// 4. Cleanup database connections
	db.Cleanup()

	apiLogger.Info().Msg("Graceful shutdown completed")
}
