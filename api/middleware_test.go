package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

func TestRegisterBaseMiddlewareRecoversPanics(t *testing.T) {
	logger := zerolog.Nop()
	app := fiber.New()
	registerBaseMiddleware(app, &logger)

	app.Get("/boom", func(c *fiber.Ctx) error {
		parts := []string{"only-one"}
		return c.SendString(parts[1])
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/boom", nil), -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusInternalServerError)
	}
}

func TestRegisterBaseMiddlewareServesNormalRequests(t *testing.T) {
	logger := zerolog.Nop()
	app := fiber.New()
	registerBaseMiddleware(app, &logger)

	app.Get("/ok", func(c *fiber.Ctx) error {
		return c.SendString("fine")
	})

	resp, err := app.Test(httptest.NewRequest("GET", "/ok", nil), -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}
