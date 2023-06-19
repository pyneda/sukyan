package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

func FindIssues(c *fiber.Ctx) error {
	issues, count, err := db.Connection.ListIssues(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues, "count": count})
}

func FindIssuesGrouped(c *fiber.Ctx) error {
	issues, err := db.Connection.ListIssuesGrouped(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues})
}
