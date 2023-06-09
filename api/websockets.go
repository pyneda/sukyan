package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"net/http"
	"strconv"
)

// @Summary Get WebSocket connections
// @Description Get WebSocket connections with optional pagination
// @Tags History WebSocket
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/history/websocket/connections [get]
func FindWebSocketConnections(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")

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

	connections, count, err := db.Connection.ListWebSocketConnections(db.WebSocketConnectionFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": connections, "count": count})
}

// @Summary Get WebSocket messages
// @Description Get WebSocket messages with optional pagination and filtering by connection id
// @Tags History WebSocket
// @Produce json
// @Param page_size query integer false "Size of each page" default(50)
// @Param page query integer false "Page number" default(1)
// @Param connection_id query string false "Filter messages by WebSocket connection ID"
// @Success 200 {array} db.WebSocketMessage
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/history/websocket/messages [get]
func FindWebSocketMessages(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedConnectionID := c.Query("connection_id")

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

	messages, count, err := db.Connection.ListWebSocketMessages(db.WebSocketMessageFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		ConnectionID: connectionID,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": messages, "count": count})
}
