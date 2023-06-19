package api

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"github.com/gofiber/fiber/v2"
)

// StartAPI starts the api
func StartAPI() {
	db.InitDb()
	app := fiber.New(fiber.Config{
    // Prefork:       true,
    // CaseSensitive: true,
    // StrictRouting: true,
    ServerHeader:  "Sukyan",
    AppName: "Sukyan API",
})

	// This allows all cors, should probably allow configure it via config and provide strict default
	// app.Use(cors.Default())
	// app.LoadHTMLGlob("templates/*")

	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("API Running")
	})

	app.Get("/issues", FindIssues)
	app.Get("/issues/grouped", FindIssuesGrouped)
	app.Get("/history", FindHistory)
	app.Get("/interactions", FindInteractions)

	if err := app.Listen(":8080"); err != nil {
			log.Warn().Err(err).Msg("Error starting server")
	}
	
}
