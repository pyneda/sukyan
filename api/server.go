package api

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan"
	"os"
	"time"

	_ "github.com/pyneda/sukyan/docs"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// StartAPI starts the api
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
		AllowOrigins: "http://localhost:3001, http://127.0.0.1:3001",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("API Running")
	})
	if viper.GetBool("api.docs.enabled") {
		app.Get(fmt.Sprintf("%v/*", viper.GetString("api.docs.path")), swagger.HandlerDefault)
	}

	api := app.Group("/api/v1")
	api.Get("/history", FindHistory)
	api.Get("/issues", FindIssues)
	api.Get("/issues/grouped", FindIssuesGrouped)
	api.Get("/history/:id/children", GetChildren)
	api.Get("/history/root-nodes", GetRootNodes)
	api.Get("/history/websocket/connections", FindWebSocketConnections)
	api.Get("/history/websocket/messages", FindWebSocketMessages)

	api.Get("/workspaces", FindWorkspaces)
	api.Get("/interactions", FindInteractions)
	api.Get("/tasks", FindTasks)
	api.Get("/tasks/jobs", FindTaskJobs)
	api.Post("/tokens/jwts", JwtListHandler)

	// Make a group for all scan endpoints which require the scan engine
	scan_app := api.Group("/scan")
	scan_app.Use(func(c *fiber.Ctx) error {
		c.Locals("engine", engine)
		return c.Next()
	})
	scan_app.Post("/passive", PassiveScanHandler)
	scan_app.Post("/active", ActiveScanHandler)

	listen_addres := fmt.Sprintf("%v:%v", viper.Get("api.listen.host"), viper.Get("api.listen.port"))
	if err := app.Listen(listen_addres); err != nil {
		log.Warn().Err(err).Msg("Error starting server")
	}

}
