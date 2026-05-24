package api

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/playground/fuzz"
	"gorm.io/gorm"
)

// MatcherPresetInput is the shape accepted by POST and PUT. The workspace_id
// is taken from the path so callers cannot create or edit presets in a
// different workspace by manipulating the body.
type MatcherPresetInput struct {
	Name       string              `json:"name" validate:"required,min=1,max=128"`
	Domain     db.MatcherPresetDomain `json:"domain" validate:"required,oneof=http_fuzz ws_fuzz"`
	MatcherSet fuzz.MatcherSet     `json:"matcher_set" validate:"required"`
}

type matcherPresetUpdateInput struct {
	Name       string          `json:"name" validate:"required,min=1,max=128"`
	MatcherSet fuzz.MatcherSet `json:"matcher_set" validate:"required"`
}

// ListMatcherPresets godoc
// @Summary List matcher presets for a workspace
// @Tags Playground
// @Param workspace_id path int true "Workspace ID"
// @Param domain query string false "http_fuzz or ws_fuzz"
// @Success 200 {array} db.MatcherPreset
// @Failure 400 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{workspace_id}/matcher-presets [get]
func ListMatcherPresets(c *fiber.Ctx) error {
	wsID, err := c.ParamsInt("workspace_id")
	if err != nil || wsID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid workspace id"})
	}
	domain := db.MatcherPresetDomain(c.Query("domain"))
	if domain != "" && domain != db.MatcherPresetDomainHTTPFuzz && domain != db.MatcherPresetDomainWsFuzz {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid domain",
			Message: "domain must be http_fuzz or ws_fuzz",
		})
	}
	presets, err := db.Connection().ListMatcherPresets(db.MatcherPresetFilters{
		WorkspaceID: uint(wsID),
		Domain:      domain,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "List failed",
			Message: err.Error(),
		})
	}
	return c.JSON(presets)
}

// CreateMatcherPreset godoc
// @Summary Create a matcher preset
// @Tags Playground
// @Accept json
// @Param workspace_id path int true "Workspace ID"
// @Param input body MatcherPresetInput true "Preset"
// @Success 201 {object} db.MatcherPreset
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{workspace_id}/matcher-presets [post]
func CreateMatcherPreset(c *fiber.Ctx) error {
	wsID, err := c.ParamsInt("workspace_id")
	if err != nil || wsID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid workspace id"})
	}
	input := new(MatcherPresetInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid JSON",
			Message: err.Error(),
		})
	}
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid input",
			Message: err.Error(),
		})
	}
	raw, err := json.Marshal(input.MatcherSet)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot marshal matcher_set",
			Message: err.Error(),
		})
	}
	preset := &db.MatcherPreset{
		WorkspaceID: uint(wsID),
		Domain:      input.Domain,
		Name:        input.Name,
		MatcherSet:  raw,
	}
	if err := db.Connection().CreateMatcherPreset(preset); err != nil {
		if errors.Is(err, db.ErrMatcherPresetNameTaken) {
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error:   "Name taken",
				Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Create failed",
			Message: err.Error(),
		})
	}
	return c.Status(fiber.StatusCreated).JSON(preset)
}

// UpdateMatcherPreset godoc
// @Summary Rename a preset or replace its matcher_set
// @Tags Playground
// @Accept json
// @Param workspace_id path int true "Workspace ID"
// @Param id path int true "Preset ID"
// @Param input body matcherPresetUpdateInput true "Updated preset"
// @Success 200 {object} db.MatcherPreset
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{workspace_id}/matcher-presets/{id} [put]
func UpdateMatcherPreset(c *fiber.Ctx) error {
	wsID, err := c.ParamsInt("workspace_id")
	if err != nil || wsID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid workspace id"})
	}
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid preset id"})
	}
	// Authorise: the preset must belong to the workspace in the path.
	existing, err := db.Connection().GetMatcherPreset(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Preset not found"})
	}
	if existing.WorkspaceID != uint(wsID) {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Preset not found"})
	}
	input := new(matcherPresetUpdateInput)
	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid JSON",
			Message: err.Error(),
		})
	}
	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid input",
			Message: err.Error(),
		})
	}
	raw, err := json.Marshal(input.MatcherSet)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot marshal matcher_set",
			Message: err.Error(),
		})
	}
	if err := db.Connection().UpdateMatcherPreset(uint(id), input.Name, raw); err != nil {
		if errors.Is(err, db.ErrMatcherPresetNameTaken) {
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{Error: "Name taken", Message: err.Error()})
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Preset not found"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Update failed",
			Message: err.Error(),
		})
	}
	updated, err := db.Connection().GetMatcherPreset(uint(id))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Reload failed"})
	}
	return c.JSON(updated)
}

// DeleteMatcherPreset godoc
// @Summary Delete a matcher preset
// @Tags Playground
// @Param workspace_id path int true "Workspace ID"
// @Param id path int true "Preset ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/workspaces/{workspace_id}/matcher-presets/{id} [delete]
func DeleteMatcherPreset(c *fiber.Ctx) error {
	wsID, err := c.ParamsInt("workspace_id")
	if err != nil || wsID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid workspace id"})
	}
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Invalid preset id"})
	}
	existing, err := db.Connection().GetMatcherPreset(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Preset not found"})
	}
	if existing.WorkspaceID != uint(wsID) {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Preset not found"})
	}
	if err := db.Connection().DeleteMatcherPreset(uint(id)); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Delete failed",
			Message: err.Error(),
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
