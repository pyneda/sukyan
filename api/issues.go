package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// FindIssues godoc
// @Summary List all issues
// @Description Retrieves all issues with a count
// @Tags Issues
// @Accept  json
// @Produce  json
// @Success 200 {array} db.Issue
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/issues [get]
func FindIssues(c *fiber.Ctx) error {
	issues, count, err := db.Connection.ListIssues(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues, "count": count})
}

// FindIssuesGrouped godoc
// @Summary List all issues grouped
// @Description Retrieves all issues grouped
// @Tags Issues
// @Accept  json
// @Produce  json
// @Success 200 {array} db.GroupedIssue
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/issues/grouped [get]
func FindIssuesGrouped(c *fiber.Ctx) error {
	issues, err := db.Connection.ListIssuesGrouped(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues})
}
