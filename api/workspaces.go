package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"

	"github.com/gofiber/fiber/v2"
)

// FindWorkspaces godoc
// @Summary List all workspaces
// @Description Retrieves all workspaces with a count
// @Tags Workspaces
// @Accept  json
// @Produce  json
// @Success 200 {array} db.Workspace
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/workspaces [get]
func FindWorkspaces(c *fiber.Ctx) error {
	items, count, err := db.Connection.ListWorkspaces()
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": items, "count": count})
}
