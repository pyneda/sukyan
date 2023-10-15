package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// FindWorkspaces godoc
// @Summary List all workspaces
// @Description Retrieves all workspaces with a count
// @Tags Workspaces
// @Accept  json
// @Produce  json
// @Param query query string false "Search query"
// @Param page_size query integer false "Size of each page" default(20)
// @Param page query integer false "Page number" default(1)
// @Success 200 {array} db.Workspace
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces [get]
func FindWorkspaces(c *fiber.Ctx) error {
	query := c.Query("query", "")
	pageSize, err := parseInt(c.Query("page_size", "20"))
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "Invalid page size parameter"})
	}
	page, err := parseInt(c.Query("page", "1"))
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": "Invalid page parameter"})
	}

	filters := db.WorkspaceFilters{
		Pagination: db.Pagination{
			PageSize: pageSize,
			Page:     page,
		},
	}
	if query != "" {
		filters.Query = query
	}
	items, count, err := db.Connection.ListWorkspaces(filters)
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
// @Failure 400 {object} ErrorResponse
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

	db.Connection.InitializeWorkspacePlayground(workspace.ID)

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": workspace})
}

// DeleteWorkspace godoc
// @Summary Delete a workspace
// @Description Deletes a workspace and all associated data
// @Tags Workspaces
// @Accept  json
// @Produce  json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{} "message": "Workspace successfully deleted"
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{id} [delete]
func DeleteWorkspace(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"message": "Invalid workspace ID", "error": "Invalid workspace ID"})
	}
	exists, err := db.Connection.WorkspaceExists(uint(id))
	if err != nil || !exists {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Workspace not found"})
	}

	if err := db.Connection.DeleteWorkspace(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"message": "Failed to delete workspace", "error": "Failed to delete workspace"})
	}

	return c.JSON(fiber.Map{"message": "Workspace successfully deleted"})
}

// WorkspaceUpdateInput defines the acceptable input for updating a workspace
type WorkspaceUpdateInput struct {
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// UpdateWorkspace godoc
// @Summary Update a workspace
// @Description Updates a workspace by ID
// @Tags Workspaces
// @Accept  json
// @Produce  json
// @Param id path string true "Workspace ID"
// @Param workspace body WorkspaceUpdateInput true "Workspace object"
// @Success 200 {object} db.Workspace
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{id} [put]
func UpdateWorkspace(c *fiber.Ctx) error {
	var updatedWorkspace db.Workspace
	if err := c.BodyParser(&updatedWorkspace); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"message": "Cannot parse JSON", "error": "Bad request"})
	}

	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"message": "Invalid workspace ID", "error": "Invalid workspace ID"})
	}
	if err := db.Connection.UpdateWorkspace(uint(id), &updatedWorkspace); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Failed to update workspace", "error": "Failed to update workspace"})
	}

	workspace, err := db.Connection.GetWorkspaceByID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Workspace not found", "error": "Workspace not found"})
	}

	return c.JSON(workspace)
}

// GetWorkspaceDetail godoc
// @Summary Get a single workspace
// @Description Retrieves a workspace by ID
// @Tags Workspaces
// @Accept  json
// @Produce  json
// @Param id path string true "Workspace ID"
// @Success 200 {object} db.Workspace
// @Failure 404 {object} ErrorResponse
// @Failure 422 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{id} [get]
func GetWorkspaceDetail(c *fiber.Ctx) error {
	id, err := parseUint(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"message": "Invalid workspace ID", "error": "Invalid workspace ID"})
	}

	workspace, err := db.Connection.GetWorkspaceByID(id)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"message": "Workspace not found", "error": "Workspace not found"})
	}

	return c.JSON(workspace)
}
