package api

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"

	"github.com/gofiber/swagger"
	"github.com/pyneda/sukyan/db"
	_ "github.com/pyneda/sukyan/docs"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"os"
	"strings"
	"time"
)

// @title Sukyan API
// @version 0.1
// @description The Sukyan API documentation.
// @termsOfService http://swagger.io/terms/
// @contact.name Contact
// @contact.email contact@sukyan.com
// @BasePath /
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
func StartAPI() {
	log.Info().Msg("Initializing...")
	db.InitDb()
	generators, err := generation.LoadGenerators(viper.GetString("generators.directory"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to load generators")
		os.Exit(1)
	}
	oobPollingInterval := time.Duration(viper.GetInt("scan.oob.poll_interval"))
	interactionsManager := &integrations.InteractionsManager{
		GetAsnInfo:            false,
		PollingInterval:       oobPollingInterval * time.Second,
		OnInteractionCallback: scan.SaveInteractionCallback,
	}
	interactionsManager.Start()

	engine := scan.NewScanEngine(generators, 100, 100, interactionsManager)
	engine.Start()

	log.Info().Msg("Initialized everything. Starting the API...")

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
		AllowOrigins: strings.Join(viper.GetStringSlice("api.cors.origins"), ","),
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("API Running")
	})
	if viper.GetBool("api.docs.enabled") {
		app.Get(fmt.Sprintf("%v/*", viper.GetString("api.docs.path")), swagger.HandlerDefault)
	}

	api := app.Group("/api/v1")
	api.Get("/history", JWTProtected(), FindHistory)
	api.Get("/issues", JWTProtected(), FindIssues)
	api.Get("/issues/grouped", JWTProtected(), FindIssuesGrouped)
	api.Get("/history/:id/children", JWTProtected(), GetChildren)
	api.Get("/history/root-nodes", JWTProtected(), GetRootNodes)
	api.Get("/history/websocket/connections", JWTProtected(), FindWebSocketConnections)
	api.Get("/history/websocket/messages", JWTProtected(), FindWebSocketMessages)

	api.Get("/workspaces", JWTProtected(), FindWorkspaces)
	api.Post("/workspaces", JWTProtected(), CreateWorkspace)
	api.Get("/interactions", JWTProtected(), FindInteractions)
	api.Get("/tasks", JWTProtected(), FindTasks)
	api.Get("/tasks/jobs", JWTProtected(), FindTaskJobs)
	api.Post("/tokens/jwts", JWTProtected(), JwtListHandler)

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
		c.Locals("engine", engine)
		return c.Next()
	})
	scan_app.Post("/passive", JWTProtected(), PassiveScanHandler)
	scan_app.Post("/active", JWTProtected(), ActiveScanHandler)

	certPath := viper.GetString("server.cert.file")
	keyPath := viper.GetString("server.key.file")
	caCertPath := viper.GetString("server.caCert.file")
	caKeyPath := viper.GetString("server.caKey.file")

	_, _, err = lib.EnsureCertificatesExist(certPath, keyPath, caCertPath, caKeyPath)
	if err != nil {
		log.Error().Err(err).Msg("Failed to load or generate certificates")

	}

	listen_addres := fmt.Sprintf("%v:%v", viper.Get("api.listen.host"), viper.Get("api.listen.port"))
	if err := app.ListenTLS(listen_addres, certPath, keyPath); err != nil {
		log.Warn().Err(err).Msg("Error starting server")
	}

}
