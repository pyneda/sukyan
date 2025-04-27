package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/browser/actions"
	"github.com/rs/zerolog/log"
)

// convertToBrowserActions converts BrowserActions to db.StoredBrowserActions
func convertToBrowserActions(ba BrowserActionsInput) db.StoredBrowserActions {
	return db.StoredBrowserActions{
		Title:       ba.Title,
		Actions:     ba.Actions,
		WorkspaceID: ba.WorkspaceID,
		Scope:       ba.Scope,
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

type BrowserActionsInput struct {
	actions.BrowserActions
	Scope       db.BrowserActionScope `json:"scope" validate:"required,oneof=global workspace"`
	WorkspaceID *uint                 `json:"workspace_id,omitempty" validate:"omitempty"`
}

// CreateStoredBrowserActions handles the API request for creating a new StoredBrowserActions
// @Summary Create a new StoredBrowserActions
// @Description Creates a new StoredBrowserActions record
// @Tags Browser Actions
// @Accept json
// @Produce json
// @Param input body BrowserActionsInput true "Browser actions input object to create"
// @Success 201 {object} db.StoredBrowserActions
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/browser-actions [post]
func CreateStoredBrowserActions(c *fiber.Ctx) error {
	input := new(BrowserActionsInput)

	if err := c.BodyParser(input); err != nil {
		log.Error().Err(err).Msg("Error parsing JSON")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot parse JSON",
			Message: "The provided JSON is invalid, check the syntax and logs for details",
		})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: buildValidationErrorMessage(err),
		})
	}

	if input.WorkspaceID == nil && input.Scope == db.BrowserActionScopeWorkspace {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Missing workspace_id",
			Message: "workspace_id is required when scope is 'workspace'",
		})
	}
	if input.WorkspaceID != nil {
		workspaceExists, _ := db.Connection().WorkspaceExists(*input.WorkspaceID)
		if !workspaceExists {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace_id",
				Message: "The provided workspace_id does not exist",
			})
		}
	}

	storedBrowserActions := convertToBrowserActions(*input)

	createdSBA, err := db.Connection().CreateStoredBrowserActions(&storedBrowserActions)
	if err != nil {
		log.Error().Err(err).Msg("Error creating StoredBrowserActions")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Check logs for details",
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
// @Param input body BrowserActionsInput true "BrowserActionsInput object to update"
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

	input := new(BrowserActionsInput)
	if err := c.BodyParser(input); err != nil {
		log.Error().Err(err).Msg("Error parsing JSON")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot parse JSON",
			Message: "The provided JSON is invalid, check the syntax and logs for details",
		})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: buildValidationErrorMessage(err),
		})
	}

	if input.WorkspaceID == nil && input.Scope == db.BrowserActionScopeWorkspace {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Missing workspace_id",
			Message: "workspace_id is required when scope is 'workspace'",
		})
	}

	if input.WorkspaceID != nil {
		workspaceExists, _ := db.Connection().WorkspaceExists(*input.WorkspaceID)
		if !workspaceExists {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace_id",
				Message: "The provided workspace_id does not exist",
			})
		}
	}

	storedBrowserActions := convertToBrowserActions(*input)
	storedBrowserActions.ID = uint(id)

	updatedSBA, err := db.Connection().UpdateStoredBrowserActions(uint(id), &storedBrowserActions)
	if err != nil {
		log.Error().Err(err).Msg("Error updating StoredBrowserActions")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Check logs for details",
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

	sba, err := db.Connection().GetStoredBrowserActionsByID(uint(id))
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

	err = db.Connection().DeleteStoredBrowserActions(uint(id))
	if err != nil {
		log.Error().Err(err).Msg("Error deleting StoredBrowserActions")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Check logs for details",
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

	items, count, err := db.Connection().ListStoredBrowserActions(*filter)
	if err != nil {
		log.Error().Err(err).Msg("Error listing StoredBrowserActions")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "Check logs for details",
		})
	}

	return c.JSON(fiber.Map{"data": items, "count": count})
}
