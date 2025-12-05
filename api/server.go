package api

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/pprof"

	"os"
	"strings"

	"github.com/gofiber/contrib/fiberzerolog"
	"github.com/gofiber/swagger"
	"github.com/pyneda/sukyan/db"
	_ "github.com/pyneda/sukyan/docs"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"time"
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
	api.Get("/playground/wordlists", JWTProtected(), ListAvailableWordlists)
	api.Get("/stats/workspace", JWTProtected(), WorkspaceStats)
	api.Get("/stats/system", JWTProtected(), SystemStats)
	api.Get("/stats/workers", JWTProtected(), ListWorkerNodes)
	api.Post("/stats/workers/cleanup", JWTProtected(), CleanupStaleWorkers)
	api.Post("/browser-actions", JWTProtected(), CreateStoredBrowserActions)
	api.Get("/browser-actions", JWTProtected(), ListStoredBrowserActions)
	api.Get("/browser-actions/:id", JWTProtected(), GetStoredBrowserActions)
	api.Put("/browser-actions/:id", JWTProtected(), UpdateStoredBrowserActions)
	api.Delete("/browser-actions/:id", JWTProtected(), DeleteStoredBrowserActions)

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

	// Scans endpoints (Scan Engine V2 / Orchestrator)
	scans_app := api.Group("/scans")
	scans_app.Get("", JWTProtected(), ListScansHandler)
	scans_app.Get("/:id", JWTProtected(), GetScanHandler)
	scans_app.Delete("/:id", JWTProtected(), DeleteScanHandler)
	scans_app.Post("/:id/cancel", JWTProtected(), CancelScanHandler)
	scans_app.Post("/:id/pause", JWTProtected(), PauseScanHandler)
	scans_app.Post("/:id/resume", JWTProtected(), ResumeScanHandler)
	scans_app.Get("/:id/jobs", JWTProtected(), GetScanJobsHandler)
	scans_app.Get("/:id/jobs/:job_id", JWTProtected(), GetScanJobHandler)
	scans_app.Post("/:id/jobs/:job_id/cancel", JWTProtected(), CancelScanJobHandler)
	scans_app.Get("/:id/stats", JWTProtected(), GetScanStatsHandler)
	scans_app.Post("/:id/schedule-items", JWTProtected(), ScheduleHistoryItemScansHandler)

	// Dashboard endpoints (separate from API, with configurable path and optional basic auth)
	if viper.GetBool("api.dashboard.enabled") {
		dashboardPath := viper.GetString("api.dashboard.path")
		if dashboardPath == "" {
			dashboardPath = "/dashboard"
		}

		// Create middleware chain for dashboard
		var dashboardMiddleware []fiber.Handler
		if viper.GetBool("api.dashboard.basic_auth.enabled") {
			dashboardMiddleware = append(dashboardMiddleware, DashboardBasicAuth())
		}

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

	defer db.Cleanup()

	listen_addres := fmt.Sprintf("%v:%v", viper.Get("api.listen.host"), viper.Get("api.listen.port"))
	if err := app.ListenTLS(listen_addres, certPath, keyPath); err != nil {
		apiLogger.Warn().Err(err).Msg("Error starting server")
	}

}
