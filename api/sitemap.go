package api

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// GetSitemap godoc
// @Summary Retrieve sitemap based on filters
// @Description Retrieves sitemap based on workspace and task ID
// @Tags Sitemap
// @Accept  json
// @Produce  json
// @Param workspace query int false "Workspace ID filter"
// @Param task query int false "Task ID filter"
// @Success 200 {array} db.SitemapNode
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/sitemap [get]
func GetSitemap(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}
	taskID, err := parseTaskID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}
	sitemap, err := db.Connection().ConstructSitemap(db.SitemapFilter{
		WorkspaceID: workspaceID,
		TaskID:      taskID,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error constructing sitemap")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}
	return c.Status(http.StatusOK).JSON(sitemap)
}
