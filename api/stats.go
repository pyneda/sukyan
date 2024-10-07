package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// WorkspaceStats retrieves statistics for a given workspace.
//
// @Summary Retrieves workspace statistics including counts of issues, history entries, JWTs,
// websocket connections, tasks, etc
// @Tags Stats
// @Accept json
// @Produce json
// @Param workspace_id path int true "Workspace ID"
// @Success 200 {object} db.WorkspaceStats "Successfully retrieved stats"
// @Failure 400 {object} ErrorResponse "Invalid workspace ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /stats/workspace [get]
func WorkspaceStats(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid workspace",
			Message: "The provided workspace ID does not seem valid. Please provide a valid workspace ID.",
		})
	}

	metrics, err := db.Connection.GetWorkspaceStats(workspaceID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve workspace statistics")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve workspace statistics",
			Message: "An unexpected error occurred while fetching workspace statistics. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(metrics)
}

// SystemStats retrieves overall system statistics.
//
// @Summary Retrieves system statistics such as the current database size.
// @Tags Stats
// @Accept json
// @Produce json
// @Success 200 {object} db.SystemStats "Successfully retrieved system stats"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Router /stats/system [get]
func SystemStats(c *fiber.Ctx) error {
	stats, err := db.Connection.GetSystemStats()
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve system statistics")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve system statistics",
			Message: "An unexpected error occurred while fetching system statistics. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(stats)
}
