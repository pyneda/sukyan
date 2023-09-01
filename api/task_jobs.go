package api

import (
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// @Summary Search Task Jobs
// @Description Allows to filter and search task jobs
// @Tags Tasks
// @Accept  json
// @Produce  json
// @Param query path int true "Task ID"
// @Param page_size query int false "Number of items per page" default(50)
// @Param page query int false "Page number" default(1)
// @Param status query string false "Comma-separated list of statuses to filter"
// @Param title query string false "Comma-separated list of titles to filter"
// @Param completed_at query string false "Completed at date to filter"
// @Success 200 {array} db.TaskJob
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/tasks/jobs [get]
func FindTaskJobs(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedStatuses := c.Query("status")
	unparsedTitles := c.Query("title")
	unparsedCompletedAt := c.Query("completed_at")
	taskID, err := parseTaskID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}
	var statuses []string
	var titles []string
	var completedAt time.Time

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

	if unparsedTitles != "" {
		titles = append(titles, strings.Split(unparsedTitles, ",")...)
	}

	if unparsedCompletedAt != "" {
		completedAt, err = time.Parse(time.RFC3339, unparsedCompletedAt)
		if err != nil {
			log.Error().Err(err).Msg("Error parsing completed_at parameter query")
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid completed_at parameter"})
		}
	}

	taskJobs, count, err := db.Connection.ListTaskJobs(db.TaskJobFilter{
		Pagination: db.Pagination{
			Page: page, PageSize: pageSize,
		},
		Statuses:    statuses,
		Titles:      titles,
		CompletedAt: &completedAt,
		TaskID:      taskID,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": taskJobs, "count": count})
}
