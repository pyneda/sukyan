package api

import (
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
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
// @Router /api/v1/tokens/jwts [post]
func JwtListHandler(c *fiber.Ctx) error {
	input := new(db.JwtFilters)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Cannot parse JSON",
		})
	}

	validate := validator.New()
	if err := validate.Struct(input); err != nil {
		errors := make(map[string]string)
		for _, err := range err.(validator.ValidationErrors) {
			errors[err.Field()] = fmt.Sprintf("Invalid value for %s", err.Field())
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Validation failed",
			"message": errors,
		})
	}

	jwts, err := db.Connection.ListJsonWebTokens(*input)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error":   "Database error",
			"message": err.Error(),
		})
	}

	return c.JSON(jwts)
}
