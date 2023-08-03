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
// @Security ApiKeyAuth
// @Router /api/v1/workspaces [get]
func FindWorkspaces(c *fiber.Ctx) error {
	items, count, err := db.Connection.ListWorkspaces()
	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": items, "count": count})
}

// WorkspaceCreateInput defines the acceptable input for creating a workspace
type WorkspaceCreateInput struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// CreateWorkspace godoc
// @Summary Create a new workspace
// @Description Saves a new workspace to the database
// @Tags Workspaces
// @Accept  json
// @Produce  json
// @Param workspace body WorkspaceCreateInput true "Workspace to create"
// @Success 201 {object} db.Workspace
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces [post]
func CreateWorkspace(c *fiber.Ctx) error {
	input := new(WorkspaceCreateInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Cannot parse JSON"})
	}

	workspace := &db.Workspace{
		Code:        input.Code,
		Title:       input.Title,
		Description: input.Description,
	}

	workspace, err := db.Connection.GetOrCreateWorkspace(workspace)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": workspace})
}
