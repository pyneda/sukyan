package api

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// FindIssues godoc
// @Summary List all issues
// @Description Retrieves all issues with a count
// @Tags Issues
// @Accept  json
// @Produce  json
// @Param workspace query int true "Workspace ID"
// @Success 200 {array} db.Issue
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/issues [get]
func FindIssues(c *fiber.Ctx) error {
	unparsedWorkspaceID := c.Query("workspace")
	workspaceID64, err := strconv.ParseUint(unparsedWorkspaceID, 10, 64)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing workspace parameter query")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}
	workspaceID := uint(workspaceID64)
	workspaceExists, _ := db.Connection.WorkspaceExists(workspaceID)
	if !workspaceExists {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}
	issues, count, err := db.Connection.ListIssues(db.IssueFilter{
		WorkspaceID: workspaceID,
	})
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
// @Security ApiKeyAuth
// @Router /api/v1/issues/grouped [get]
func FindIssuesGrouped(c *fiber.Ctx) error {
	issues, err := db.Connection.ListIssuesGrouped(db.IssueFilter{})
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues})
}
