package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// FindInteractions gets interactions with pagination and filtering options
// @Summary Get interactions
// @Description Get interactions with optional pagination and protocols filter
// @Tags Interactions
// @Produce json
// @Param workspace query int true "Workspace ID"
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param protocols query string false "Comma-separated list of protocols to filter by"
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/interactions [get]
func FindInteractions(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedProtocols := c.Query("protocols")
	var protocols []string
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}
	pageSize, err := strconv.Atoi(unparsedPageSize)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page size parameter query")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid page size parameter"})
	}

	page, err := strconv.Atoi(unparsedPage)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing page parameter query")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid page parameter"})
	}

	if unparsedProtocols != "" {
		protocols = append(protocols, strings.Split(unparsedProtocols, ",")...)
	}

	issues, count, err := db.Connection.ListInteractions(db.InteractionsFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		Protocols:   protocols,
		WorkspaceID: workspaceID,
	})

	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues, "count": count})
}

// GetInteractionDetail fetches the details of a specific OOB Interaction by its ID.
// @Summary Get interaction detail
// @Description Fetch the detail of an OOB Interaction by its ID
// @Tags Interactions
// @Produce json
// @Param id path int true "Interaction ID"
// @Success 200 {object} db.OOBInteraction
// @Failure 404 {object} ErrorResponse "Interaction not found"
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/interactions/{id} [get]
func GetInteractionDetail(c *fiber.Ctx) error {
	interactionID, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid interaction ID",
			"message": "The provided interaction ID does not seem valid",
		})
	}

	interaction, err := db.Connection.GetInteraction(uint(interactionID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":   "Interaction not found",
				"message": "The requested interaction does not exist",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(http.StatusOK).JSON(interaction)
}
