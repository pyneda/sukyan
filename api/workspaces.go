package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

func FindWorkspaces(c *fiber.Ctx) error {
	items, count, err := db.Connection.ListWorkspaces()
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": items, "count": count})
}
