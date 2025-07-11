package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/pyneda/sukyan/db"

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

// FindHistoryPost handles POST requests for fetching history with pagination and filtering options
// @Summary Get history (POST)
// @Description Get history with optional pagination and filtering using POST request
// @Tags History
// @Accept json
// @Produce json
// @Param filters body db.HistoryFilter true "History filter options"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history [post]
func FindHistoryPost(c *fiber.Ctx) error {
	var filters db.HistoryFilter
	if err := c.BodyParser(&filters); err != nil {
		log.Error().Err(err).Msg("Error parsing history filter")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid request body",
			"message": "There was an error parsing the request body",
		})
	}

	if filters.WorkspaceID > 0 {
		workspaceExists, _ := db.Connection().WorkspaceExists(filters.WorkspaceID)
		if !workspaceExists {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace",
				Message: "The provided workspace_id does not exist",
			})
		}
	}

	validate := validator.New()
	if err := validate.Struct(filters); err != nil {
		var sb strings.Builder
		for _, err := range err.(validator.ValidationErrors) {
			sb.WriteString(fmt.Sprintf("Validation failed on '%s' tag for field '%s'\n", err.Tag(), err.Field()))
		}
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Filters validation failed",
			Message: sb.String(),
		})
	}

	// Set default values if not provided
	if filters.Pagination.Page == 0 {
		filters.Pagination.Page = 1
	}
	if filters.Pagination.PageSize == 0 {
		filters.Pagination.PageSize = 50
	}
	if filters.SortBy == "" {
		filters.SortBy = "id"
	}
	if filters.SortOrder == "" {
		filters.SortOrder = "desc"
	}

	items, count, err := db.Connection().ListHistory(filters)
	if err != nil {
		log.Error().Err(err).Interface("filters", filters).Msg("Error fetching history")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal server error",
			Message: "An error occurred while fetching history",
		})
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"data":  items,
		"count": count,
	})
}

// FindHistory gets history with pagination and filtering options
// @Summary Get history
// @Description Get history with optional pagination and filtering by status codes, HTTP methods, and sources
// @Deprecated
// @Tags History
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param status query string false "Comma-separated list of status codes to filter by"
// @Param methods query string false "Comma-separated list of HTTP methods to filter by"
// @Param sources query string false "Comma-separated list of sources to filter by"
// @Param ids query string false "Comma-separated list of history IDs to filter by"
// @Param workspace query integer true "Workspace ID to filter by"
// @Param playground_session query integer false "Playground session ID to filter by"
// @Param task query integer false "Task ID"
// @Param sort_by query string false "Field to sort by" Enums(id,created_at,updated_at,status_code,request_body_size,url,response_body_size,parameters_count,method) default("id")
// @Param sort_order query string false "Sort order" Enums(asc, desc) default("desc")
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history [get]
func FindHistory(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedStatusCodes := c.Query("status")
	unparsedHttpMethods := c.Query("methods")
	unparsedSources := c.Query("sources")
	unparsedIDs := c.Query("ids", "")
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}
	taskID, err := parseTaskID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}

	playgroundSession, err := parsePlaygroundSessionID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid playground session",
			"message": "The provided playground session ID does not seem valid",
		})
	}

	var statusCodes []int
	var httpMethods []string
	var sources []string

	filterIDs, err := stringToUintSlice(unparsedIDs, []uint{}, false)
	if err != nil {
		log.Error().Err(err).Str("unparsed", unparsedIDs).Msg("Error parsing filter IDs parameter query")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid filter IDs parameter"})
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
	filters := db.HistoryFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		StatusCodes:         statusCodes,
		Methods:             httpMethods,
		Sources:             sources,
		WorkspaceID:         workspaceID,
		SortBy:              c.Query("sort_by", "id"),
		SortOrder:           c.Query("sort_order", "desc"),
		TaskID:              taskID,
		IDs:                 filterIDs,
		PlaygroundSessionID: playgroundSession,
	}
	validate := validator.New()
	if err := validate.Struct(filters); err != nil {
		errors := make(map[string]string)
		for _, err := range err.(validator.ValidationErrors) {
			errors[err.Field()] = "Invalid value"
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Filters validation failed",
			"message": errors,
		})
	}
	items, count, err := db.Connection().ListHistory(filters)

	if err != nil {
		// Should handle this better
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
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
// @Failure 400,404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history/{id}/children [get]
func GetChildren(c *fiber.Ctx) error {
	// get history id from path
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID"})
	}

	// retrieve the parent history item
	parent, err := db.Connection().GetHistoryByID(uint(id))
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "History not found"})
	}

	// retrieve all the children history items
	children, err := db.Connection().GetChildrenHistories(parent)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
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

type RootNode struct {
	ID    uint   `json:"id"`
	Depth int    `json:"depth"`
	URL   string `json:"url"`
}

// @Summary Gets all root history nodes
// @Description Get all the root history items
// @Tags History
// @Accept  json
// @Produce  json
// @Param workspace query integer true "Workspace ID to filter by"
// @Success 200 {array} RootNode
// @Failure 400,404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history/root-nodes [get]
func GetRootNodes(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}
	children, err := db.Connection().GetRootHistoryNodes(workspaceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}

	nodes := make([]RootNode, len(children))
	for i, child := range children {
		nodes[i] = RootNode{
			ID:    child.ID,
			Depth: child.Depth,
			URL:   child.URL,
		}
	}

	// return the response
	return c.Status(fiber.StatusOK).JSON(nodes)
}

// GetHistoryDetail fetches the details of a specific History item by its ID
// @Summary Get history detail
// @Description Fetch the detail of a History item by its ID
// @Tags History
// @Produce json
// @Param id path int true "History ID"
// @Success 200 {object} db.History
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse "History not found"
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history/{id} [get]
func GetHistoryDetail(c *fiber.Ctx) error {
	historyID, err := c.ParamsInt("id")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid history ID",
			Message: "The provided history ID does not seem valid",
		})
	}

	history, err := db.Connection().GetHistoryByID(uint(historyID))
	if err != nil {
		if err.Error() == "record not found" {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "History not found",
				Message: "The requested history item does not exist",
			})
		}
		log.Error().Err(err).Int("history_id", historyID).Msg("Failed to get history detail")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal server error",
			Message: DefaultInternalServerErrorMessage,
		})
	}

	return c.Status(http.StatusOK).JSON(history)
}
