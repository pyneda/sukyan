package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"

	"github.com/gofiber/fiber/v2"
)

// FindTasks godoc
// @Summary List tasks with pagination and filtering
// @Description Retrieves tasks based on pagination and status filters
// @Tags Tasks
// @Accept  json
// @Produce  json
// @Param query query string false "Query string to search for"
// @Param page_size query int false "Number of items per page" default(50)
// @Param page query int false "Page number" default(1)
// @Param workspace query int true "Workspace ID"
// @Param status query string false "Comma-separated list of statuses to filter"
// @Param playground_session query integer false "Playground session ID to filter by"
// @Success 200 {array} db.Task
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/tasks [get]
func FindTasks(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid workspace",
			"message": "The provided workspace ID does not seem valid",
		})
	}

	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedStatuses := c.Query("status")
	var statuses []string
	query := c.Query("query")

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

	if unparsedStatuses != "" {
		statuses = append(statuses, strings.Split(unparsedStatuses, ",")...)
	}
	playgroundSession, err := parsePlaygroundSessionID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid playground session",
			"message": "The provided playground session ID does not seem valid",
		})
	}
	tasks, count, err := db.Connection.ListTasks(db.TaskFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		Statuses:            statuses,
		WorkspaceID:         workspaceID,
		FetchStats:          true,
		Query:               query,
		PlaygroundSessionID: playgroundSession,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": DefaultInternalServerErrorMessage})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": tasks, "count": count})
}
