package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// FindInteractions gets interactions with pagination and filtering options
// @Summary Get interactions
// @Description Get interactions with optional pagination and protocols filter
// @Tags Interactions
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param protocols query string false "Comma-separated list of protocols to filter by"
// @Router /api/v1/interactions [get]
func FindInteractions(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedProtocols := c.Query("protocols")
	var protocols []string
	log.Warn().Str("protocols", unparsedProtocols).Msg("protocols unparsed")

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
		for _, protocol := range strings.Split(unparsedProtocols, ",") {
			protocols = append(protocols, protocol)
		}
	}

	issues, count, err := db.Connection.ListInteractions(db.InteractionsFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		Protocols: protocols,
	})

	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": issues, "count": count})
}
