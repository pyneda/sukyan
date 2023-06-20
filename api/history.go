package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

func IsValidFilterHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "TRACE":
		return true
	default:
		return false
	}
}



// FindHistory gets history with pagination and filtering options
// @Summary Get history
// @Description Get history with optional pagination and filtering by status codes, HTTP methods, and sources
// @Tags History
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param status query string false "Comma-separated list of status codes to filter by"
// @Param methods query string false "Comma-separated list of HTTP methods to filter by"
// @Param sources query string false "Comma-separated list of sources to filter by"
// @Router /history [get]
func FindHistory(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedStatusCodes := c.Query("status")
	unparsedHttpMethods := c.Query("methods")
	unparsedSources := c.Query("sources")
	var statusCodes []int
	var httpMethods []string
	var sources []string
	log.Warn().Str("status", unparsedStatusCodes).Msg("status codes unparsed")

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

	if unparsedStatusCodes != "" {
		for _, status := range strings.Split(unparsedStatusCodes, ",") {
			statusInt, err := strconv.Atoi(status)
			if err != nil {
				log.Error().Err(err).Msg("Error parsing page status parameter query")
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid status parameter"})
			} else {
				statusCodes = append(statusCodes, statusInt)
			}
		}
	}

	if unparsedSources != "" {
		for _, source := range strings.Split(unparsedSources, ",") {
			if db.IsValidSource(source) {
				sources = append(sources, source)
			} else {
				log.Warn().Str("source", source).Msg("Invalid filter source provided")
			}
		}
	}

	if unparsedHttpMethods != "" {
		for _, method := range strings.Split(unparsedHttpMethods, ",") {
			if IsValidFilterHTTPMethod(method) {
				httpMethods = append(httpMethods, method)
			} else {
				log.Warn().Str("method", method).Msg("Invalid filter HTTP method provided")
			}
		}
	}
	items, count, err := db.Connection.ListHistory(db.HistoryFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		StatusCodes: statusCodes,
		Methods:     httpMethods,
		Sources:     sources,
	})

	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": items, "count": count})
}
