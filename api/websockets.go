package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// @Summary Get WebSocket connections
// @Description Get WebSocket connections with optional pagination
// @Tags History
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param workspace query int true "Workspace ID"
// @Param task query int false "Task ID"
// @Param scan_id query int false "Scan ID"
// @Param scan_job_id query int false "Scan Job ID"
// @Param sources query string false "Comma-separated list of sources to filter by"
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history/websocket/connections [get]
func FindWebSocketConnections(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedSources := c.Query("sources")
	var sources []string

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

	scanID, err := parseScanID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid scan",
			"message": "The provided scan ID does not seem valid",
		})
	}

	scanJobID, err := parseScanJobID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid scan job",
			"message": "The provided scan job ID does not seem valid",
		})
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
	connections, count, err := db.Connection().ListWebSocketConnections(db.WebSocketConnectionFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		WorkspaceID: workspaceID,
		TaskID:      taskID,
		ScanID:      scanID,
		ScanJobID:   scanJobID,
		Sources:     sources,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": connections, "count": count})
}

// @Summary Get WebSocket messages
// @Description Get WebSocket messages with optional pagination and filtering by connection id
// @Tags History
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param connection_id query string false "Filter messages by WebSocket connection ID"
// @Param is_binary query boolean false "Filter by binary messages (true) or text messages (false)"
// @Success 200 {array} db.WebSocketMessage
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history/websocket/messages [get]
func FindWebSocketMessages(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedConnectionID := c.Query("connection_id")
	unparsedIsBinary := c.Query("is_binary")

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

	var connectionID uint
	if unparsedConnectionID != "" {
		unparsedUint, err := strconv.ParseUint(unparsedConnectionID, 10, 32)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing connection ID query parameter")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid connection ID parameter"})
		}
		connectionID = uint(unparsedUint)
	}

	var isBinary *bool
	if unparsedIsBinary != "" {
		parsed, err := strconv.ParseBool(unparsedIsBinary)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing is_binary query parameter")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid is_binary parameter, must be true or false"})
		}
		isBinary = &parsed
	}

	messages, count, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		ConnectionID: connectionID,
		IsBinary:     isBinary,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": messages, "count": count})
}

// @Summary Get WebSocket connection details
// @Description Get details of a specific WebSocket connection by its ID, including its associated messages
// @Tags History
// @Produce json
// @Param id path int true "WebSocket connection ID"
// @Success 200 {object} db.WebSocketConnection
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history/websocket/connections/{id} [get]
func FindWebSocketConnectionByID(c *fiber.Ctx) error {
	unparsedConnectionID := c.Params("id")

	connectionID, err := strconv.ParseUint(unparsedConnectionID, 10, 32)
	if err != nil {
		log.Error().Err(err).Msg("Error parsing connection ID parameter")
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid connection ID",
			Message: "The provided connection ID is not valid",
		})
	}

	connection, err := db.Connection().GetWebSocketConnection(uint(connectionID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Connection not found",
				Message: "No WebSocket connection found with the provided ID",
			})
		}
		log.Error().Err(err).Msg("Error fetching WebSocket connection details")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Error fetching WebSocket connection details",
			Message: "An unexpected error occurred while fetching the WebSocket connection details. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(connection)
}
