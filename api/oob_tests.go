package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// FindOOBTests gets OOB tests with pagination and filtering options
// @Summary Get OOB tests
// @Description Get OOB tests with optional pagination and filtering
// @Tags OOB Tests
// @Accept json
// @Produce json
// @Param filters body db.OOBTestsFilter true "OOB test filter options"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/oob-tests [post]
func FindOOBTests(c *fiber.Ctx) error {
	var filters db.OOBTestsFilter

	if err := c.BodyParser(&filters); err != nil {
		log.Error().Err(err).Msg("Failed to parse request body")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid request body",
			Message: "The request body could not be parsed. Please ensure it's valid JSON.",
		})
	}

	if filters.Pagination.Page == 0 {
		filters.Pagination.Page = 1
	}
	if filters.Pagination.PageSize == 0 {
		filters.Pagination.PageSize = 50
	}

	validate := validator.New()
	if err := validate.Struct(filters); err != nil {
		log.Error().Err(err).Msg("Validation failed for OOB tests filter")
		var sb strings.Builder
		for _, fieldErr := range err.(validator.ValidationErrors) {
			sb.WriteString(fmt.Sprintf("Validation failed on '%s' tag for field '%s'; ", fieldErr.Tag(), fieldErr.Field()))
		}
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Filter validation failed",
			Message: sb.String(),
		})
	}

	if filters.WorkspaceID > 0 {
		workspaceExists, err := db.Connection().WorkspaceExists(filters.WorkspaceID)
		if err != nil {
			log.Error().Err(err).Uint("workspace_id", filters.WorkspaceID).Msg("Failed to check workspace existence")
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal server error",
				Message: DefaultInternalServerErrorMessage,
			})
		}
		if !workspaceExists {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace",
				Message: "The provided workspace_id does not exist",
			})
		}
	}

	items, count, err := db.Connection().ListOOBTests(filters)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list OOB tests")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal server error",
			Message: DefaultInternalServerErrorMessage,
		})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"data":  items,
		"count": count,
	})
}

// GetOOBTestDetail fetches the details of a specific OOB Test by its ID
// @Summary Get OOB test detail
// @Description Fetch the detail of an OOB Test by its ID
// @Tags OOB Tests
// @Produce json
// @Param id path int true "OOB Test ID"
// @Success 200 {object} db.OOBTest
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse "OOB Test not found"
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/oob-tests/{id} [get]
func GetOOBTestDetail(c *fiber.Ctx) error {
	oobTestID, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid OOB test ID",
			Message: "The provided OOB test ID does not seem valid",
		})
	}

	var oobTest db.OOBTest
	result := db.Connection().DB().Preload("HistoryItem").First(&oobTest, uint(oobTestID))
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "OOB test not found",
				Message: "The requested OOB test does not exist",
			})
		}
		log.Error().Err(result.Error).Int("oob_test_id", oobTestID).Msg("Failed to get OOB test detail")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal server error",
			Message: DefaultInternalServerErrorMessage,
		})
	}

	return c.Status(http.StatusOK).JSON(oobTest)
}
