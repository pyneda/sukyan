package api

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// FindTasks godoc
// @Summary List tasks with pagination and filtering
// @Description Retrieves tasks based on pagination and status filters
// @Tags Tasks
// @Accept  json
// @Produce  json
// @Param page_size query int false "Number of items per page" default(50)
// @Param page query int false "Page number" default(1)
// @Param status query string false "Comma-separated list of statuses to filter"
// @Success 200 {array} db.Task
// @Failure 500 {object} ErrorResponse
// @Router /api/v1/tasks [get]
func FindTasks(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedStatuses := c.Query("status")
	var statuses []string

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

	tasks, count, err := db.Connection.ListTasks(db.TaskFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		Statuses: statuses,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": tasks, "count": count})
}
