package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/browser/actions"
)

// convertToBrowserActions converts BrowserActions to db.StoredBrowserActions
func convertToBrowserActions(ba actions.BrowserActions, workspaceID *uint, scope db.BrowserActionScope) db.StoredBrowserActions {
	return db.StoredBrowserActions{
		Title:       ba.Title,
		Actions:     ba.Actions,
		WorkspaceID: workspaceID,
		Scope:       scope,
	}
}

// buildValidationErrorMessage creates a comprehensive error message from validation errors
func buildValidationErrorMessage(err error) string {
	var errMsgs []string
	for _, err := range err.(validator.ValidationErrors) {
		switch err.Tag() {
		case "required":
			errMsgs = append(errMsgs, fmt.Sprintf("%s is required", err.Field()))
		case "min":
			errMsgs = append(errMsgs, fmt.Sprintf("%s must have at least %s items", err.Field(), err.Param()))
		case "oneof":
			errMsgs = append(errMsgs, fmt.Sprintf("%s must be one of: %s", err.Field(), err.Param()))
		case "required_if":
			errMsgs = append(errMsgs, fmt.Sprintf("%s is required when %s", err.Field(), err.Param()))
		case "url":
			errMsgs = append(errMsgs, fmt.Sprintf("%s must be a valid URL", err.Field()))
		case "gt":
			errMsgs = append(errMsgs, fmt.Sprintf("%s must be greater than %s", err.Field(), err.Param()))
		default:
			errMsgs = append(errMsgs, fmt.Sprintf("%s is invalid", err.Field()))
		}
	}
	return strings.Join(errMsgs, "; ")
}

// CreateStoredBrowserActions handles the API request for creating a new StoredBrowserActions
// @Summary Create a new StoredBrowserActions
// @Description Creates a new StoredBrowserActions record
// @Tags Browser Actions
// @Accept json
// @Produce json
// @Param input body actions.BrowserActions true "BrowserActions object to create"
// @Param workspace_id query int false "Workspace ID"
// @Param scope query string false "Scope (global or workspace)" Enums(global, workspace)
// @Success 201 {object} db.StoredBrowserActions
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-actions [post]
func CreateStoredBrowserActions(c *fiber.Ctx) error {
	input := new(actions.BrowserActions)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot parse JSON",
			Message: err.Error(),
		})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: buildValidationErrorMessage(err),
		})
	}

	var workspaceID *uint
	if workspaceIDStr := c.Query("workspace_id"); workspaceIDStr != "" {
		if id, err := strconv.ParseUint(workspaceIDStr, 10, 32); err == nil {
			uintID := uint(id)
			workspaceID = &uintID
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace_id",
				Message: "The provided workspace_id is not a valid number",
			})
		}
	}

	scope := db.BrowserActionScope(c.Query("scope", string(db.BrowserActionScopeGlobal)))
	if scope != db.BrowserActionScopeGlobal && scope != db.BrowserActionScopeWorkspace {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scope",
			Message: "Scope must be either 'global' or 'workspace'",
		})
	}

	storedBrowserActions := convertToBrowserActions(*input, workspaceID, scope)

	createdSBA, err := db.Connection.CreateStoredBrowserActions(&storedBrowserActions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: err.Error(),
		})
	}

	return c.Status(fiber.StatusCreated).JSON(createdSBA)
}

// UpdateStoredBrowserActions handles the API request for updating a StoredBrowserActions
// @Summary Update a StoredBrowserActions
// @Description Updates an existing StoredBrowserActions record
// @Tags Browser Actions
// @Accept json
// @Produce json
// @Param id path int true "StoredBrowserActions ID"
// @Param input body actions.BrowserActions true "BrowserActions object to update"
// @Param workspace_id query int false "Workspace ID"
// @Param scope query string false "Scope (global or workspace)" Enums(global, workspace)
// @Success 200 {object} db.StoredBrowserActions
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-actions/{id} [put]
func UpdateStoredBrowserActions(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid ID",
			Message: "The provided ID is not a valid number",
		})
	}

	input := new(actions.BrowserActions)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot parse JSON",
			Message: err.Error(),
		})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: buildValidationErrorMessage(err),
		})
	}

	var workspaceID *uint
	if workspaceIDStr := c.Query("workspace_id"); workspaceIDStr != "" {
		if id, err := strconv.ParseUint(workspaceIDStr, 10, 32); err == nil {
			uintID := uint(id)
			workspaceID = &uintID
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace_id",
				Message: "The provided workspace_id is not a valid number",
			})
		}
	}

	scope := db.BrowserActionScope(c.Query("scope", string(db.BrowserActionScopeGlobal)))
	if scope != db.BrowserActionScopeGlobal && scope != db.BrowserActionScopeWorkspace {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scope",
			Message: "Scope must be either 'global' or 'workspace'",
		})
	}

	storedBrowserActions := convertToBrowserActions(*input, workspaceID, scope)
	storedBrowserActions.ID = uint(id)

	updatedSBA, err := db.Connection.UpdateStoredBrowserActions(uint(id), &storedBrowserActions)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: err.Error(),
		})
	}

	return c.JSON(updatedSBA)
}

// GetStoredBrowserActions handles the API request for retrieving a StoredBrowserActions by ID
// @Summary Get a StoredBrowserActions by ID
// @Description Retrieves a StoredBrowserActions record by its ID
// @Tags Browser Actions
// @Accept json
// @Produce json
// @Param id path int true "StoredBrowserActions ID"
// @Success 200 {object} db.StoredBrowserActions
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-actions/{id} [get]
func GetStoredBrowserActions(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid ID",
			Message: "The provided ID is not a valid number",
		})
	}

	sba, err := db.Connection.GetStoredBrowserActionsByID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Not found",
			Message: "StoredBrowserActions not found",
		})
	}

	return c.JSON(sba)
}

// DeleteStoredBrowserActions handles the API request for deleting a StoredBrowserActions
// @Summary Delete a StoredBrowserActions
// @Description Deletes an existing StoredBrowserActions record
// @Tags Browser Actions
// @Accept json
// @Produce json
// @Param id path int true "StoredBrowserActions ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-actions/{id} [delete]
func DeleteStoredBrowserActions(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid ID",
			Message: "The provided ID is not a valid number",
		})
	}

	err = db.Connection.DeleteStoredBrowserActions(uint(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: err.Error(),
		})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// ListStoredBrowserActions handles the API request for listing StoredBrowserActions with filtering and sorting
// @Summary List StoredBrowserActions with filtering and sorting
// @Description Retrieves a list of StoredBrowserActions with optional filtering and sorting options
// @Tags Browser Actions
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(50)
// @Param query query string false "Search query for title"
// @Param scope query string false "Scope filter (global or workspace)"
// @Param workspace_id query int false "Workspace ID filter"
// @Success 200 {object} map[string]interface{} "Returns 'data' (array of StoredBrowserActions) and 'count' (total number of records)"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-actions [get]
func ListStoredBrowserActions(c *fiber.Ctx) error {
	filter := new(db.StoredBrowserActionsFilter)

	// Parse query parameters
	var err error
	filter.Pagination.Page, err = strconv.Atoi(c.Query("page", "1"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid page",
			Message: "The provided page number is not valid",
		})
	}

	filter.Pagination.PageSize, err = strconv.Atoi(c.Query("page_size", "50"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid page_size",
			Message: "The provided page size is not valid",
		})
	}

	filter.Query = c.Query("query")
	filter.Scope = db.BrowserActionScope(c.Query("scope"))

	if workspaceID, err := strconv.Atoi(c.Query("workspace_id")); err == nil {
		uintWorkspaceID := uint(workspaceID)
		filter.WorkspaceID = &uintWorkspaceID
	}

	validate := validator.New()
	if err := validate.Struct(filter); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: buildValidationErrorMessage(err),
		})
	}

	items, count, err := db.Connection.ListStoredBrowserActions(*filter)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{"data": items, "count": count})
}
