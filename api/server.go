package api

import (
	"fmt"
	"github.com/gofiber/swagger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	_ "github.com/pyneda/sukyan/docs"
)

// StartAPI starts the api
func StartAPI() {
	db.InitDb()
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
	app.Get("/issues", FindIssues)
	app.Get("/issues/grouped", FindIssuesGrouped)
	app.Get("/history", FindHistory)
	app.Get("/api/history/:id/children", GetChildren)
	app.Get("/api/history/root-nodes", GetRootNodes)

	app.Get("/interactions", FindInteractions)
	app.Get("/workspaces", FindWorkspaces)

	listen_addres := fmt.Sprintf("%v:%v", viper.Get("api.listen.host"), viper.Get("api.listen.port"))
	if err := app.Listen(listen_addres); err != nil {
		log.Warn().Err(err).Msg("Error starting server")
	}

}
