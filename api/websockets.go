package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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
// @Param query query string false "Case-insensitive substring match against the connection URL"
// @Param min_messages query integer false "Only return connections with at least this many messages"
// @Param sort_by query string false "Field to sort by" Enums(id,url,status_code,source,message_count,created_at,closed_at) default("id")
// @Param sort_order query string false "Sort order" Enums(asc,desc) default("desc")
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

	var proxyServiceID *uuid.UUID
	if proxyServiceIDStr := c.Query("proxy_service_id"); proxyServiceIDStr != "" {
		parsedID, err := parseUUID(proxyServiceIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Invalid proxy service ID",
				"message": "The provided proxy_service_id is not a valid UUID",
			})
		}
		proxyServiceID = &parsedID
	}

	if unparsedSources != "" {
		for _, source := range strings.Split(unparsedSources, ",") {
			if db.IsValidWebSocketSource(source) {
				sources = append(sources, source)
			} else {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"error":   "Invalid source",
					"message": "The provided sources filter contains an unknown source",
				})
			}
		}
	}

	minMessages := 0
	if unparsedMinMessages := c.Query("min_messages"); unparsedMinMessages != "" {
		parsed, err := strconv.Atoi(unparsedMinMessages)
		if err != nil || parsed < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":   "Invalid min messages",
				"message": "The provided min_messages must be a non-negative integer",
			})
		}
		minMessages = parsed
	}

	urlPrefixes := c.Context().QueryArgs().PeekMulti("url_prefixes")
	prefixes := make([]string, 0, len(urlPrefixes))
	for _, p := range urlPrefixes {
		if len(p) > 0 {
			prefixes = append(prefixes, string(p))
		}
	}

	connections, count, err := db.Connection().ListWebSocketConnections(db.WebSocketConnectionFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		WorkspaceID:    workspaceID,
		TaskID:         taskID,
		ScanID:         scanID,
		ScanJobID:      scanJobID,
		ProxyServiceID: proxyServiceID,
		Sources:        sources,
		Query:          c.Query("query"),
		MinMessages:    minMessages,
		URLPrefixes:    prefixes,
		SortBy:         c.Query("sort_by", "id"),
		SortOrder:      c.Query("sort_order", "desc"),
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
// @Param playground_session_id query int false "filter by playground session id"
// @Param is_binary query boolean false "Filter by binary messages (true) or text messages (false)"
// @Param direction query string false "Filter by message direction" Enums(sent,received)
// @Success 200 {array} db.WebSocketMessage
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/history/websocket/messages [get]
func FindWebSocketMessages(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedConnectionID := c.Query("connection_id")
	unparsedPlaygroundSessionID := c.Query("playground_session_id")
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
		unparsedUint, err := strconv.ParseUint(unparsedConnectionID, 10, 64)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing connection ID query parameter")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid connection ID parameter"})
		}
		connectionID = uint(unparsedUint)
	}

	var playgroundSessionID *uint
	if unparsedPlaygroundSessionID != "" {
		parsed, err := strconv.ParseUint(unparsedPlaygroundSessionID, 10, 64)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing playground_session_id query parameter")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid playground_session_id parameter"})
		}
		parsedUint := uint(parsed)
		playgroundSessionID = &parsedUint
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

	var direction db.MessageDirection
	switch unparsedDirection := c.Query("direction"); unparsedDirection {
	case "":
	case string(db.MessageSent), string(db.MessageReceived):
		direction = db.MessageDirection(unparsedDirection)
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid direction parameter, must be sent or received"})
	}

	messages, count, err := db.Connection().ListWebSocketMessages(db.WebSocketMessageFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		ConnectionID:        connectionID,
		PlaygroundSessionID: playgroundSessionID,
		IsBinary:            isBinary,
		Direction:           direction,
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

	connectionID, err := strconv.ParseUint(unparsedConnectionID, 10, 64)
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
