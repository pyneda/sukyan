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
// @Router /api/v1/history [get]
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

type HistorySummary struct {
	ID              uint   `json:"id"`
	Depth           int    `json:"depth"`
	URL             string `json:"url"`
	StatusCode      int    `json:"status_code"`
	Method          string `json:"method"`
	ParametersCount int    `json:"parameters_count"`
}

// @Summary Get children history
// @Description Get all the other history items that have the same depth or more than the provided history ID and that start with the same URL
// @Tags History
// @Accept  json
// @Produce  json
// @Param id path int true "History ID"
// @Success 200 {array} HistorySummary
// @Failure 400,404 {object} string
// @Router /api/v1/history/{id}/children [get]
func GetChildren(c *fiber.Ctx) error {
	// get history id from path
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID"})
	}

	// retrieve the parent history item
	parent, err := db.Connection.GetHistoryByID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "History not found"})
	}

	// retrieve all the children history items
	children, err := db.Connection.GetChildrenHistories(parent)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// map to the HistorySummary type
	childrenSummaries := make([]HistorySummary, len(children))
	for i, child := range children {
		childrenSummaries[i] = HistorySummary{
			ID:              child.ID,
			Depth:           child.Depth,
			URL:             child.URL,
			StatusCode:      child.StatusCode,
			Method:          child.Method,
			ParametersCount: child.ParametersCount,
		}
	}

	// return the response
	return c.Status(fiber.StatusOK).JSON(childrenSummaries)
}

// @Summary Gets all root history nodes
// @Description Get all the root history items
// @Tags History
// @Accept  json
// @Produce  json
// @Success 200 {array} HistorySummary
// @Failure 400,404 {object} string
// @Router /api/v1/history/root-nodes [get]
func GetRootNodes(c *fiber.Ctx) error {
	// retrieve all the children history items
	children, err := db.Connection.GetRootHistoryNodes()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// map to the HistorySummary type
	childrenSummaries := make([]HistorySummary, len(children))
	for i, child := range children {
		childrenSummaries[i] = HistorySummary{
			ID:              child.ID,
			Depth:           child.Depth,
			URL:             child.URL,
			StatusCode:      child.StatusCode,
			Method:          child.Method,
			ParametersCount: child.ParametersCount,
		}
	}

	// return the response
	return c.Status(fiber.StatusOK).JSON(childrenSummaries)
}
