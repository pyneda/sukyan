package api

import (
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// @Success 200 {array} db.JsonWebToken

// JwtListHandler handles the API request for listing JWTs with filtering and sorting
// @Summary List JWTs with filtering and sorting
// @Description Retrieves a list of JWTs with optional filtering and sorting options
// @Tags JWT
// @Accept  json
// @Produce  json
// @Param input body db.JwtFilters true "Filtering and sorting options"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/tokens/jwts [post]
func JwtListHandler(c *fiber.Ctx) error {
	input := new(db.JwtFilters)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	if input.WorkspaceID > 0 {
		workspaceExists, _ := db.Connection().WorkspaceExists(input.WorkspaceID)
		if !workspaceExists {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace",
				Message: "The provided workspace_id does not exist",
			})
		}
	}
	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		var sb strings.Builder
		for _, err := range err.(validator.ValidationErrors) {
			sb.WriteString(fmt.Sprintf("Validation failed on '%s' tag for field '%s'\n", err.Tag(), err.Field()))
		}
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: sb.String(),
		})
	}

	jwts, err := db.Connection().ListJsonWebTokens(*input)
	if err != nil {
		log.Error().Err(err).Msg("Error fetching JWTs")

		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Database error",
			Message: "There was an error fetching the JWTs",
		})
	}

	return c.JSON(jwts)
}
