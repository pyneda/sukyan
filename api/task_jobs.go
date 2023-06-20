package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"time"
	"strconv"
	"strings"
	"github.com/rs/zerolog/log"

	"github.com/gofiber/fiber/v2"
)

// @Summary Search Task Jobs
// @Description Allows to filter and search task jobs
// @Tags tasks
// @Accept  json
// @Produce  json
// @Param id path int true "Task ID"
// @Router /tasks/ [get]
func FindTaskJobs(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedStatuses := c.Query("status")
	unparsedTitles := c.Query("title")
	unparsedCompletedAt := c.Query("completed_at")
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
		for _, status := range strings.Split(unparsedStatuses, ",") {
			statuses = append(statuses, status)
		}
	}

	if unparsedTitles != "" {
		for _, title := range strings.Split(unparsedTitles, ",") {
			titles = append(titles, title)
		}
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
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": taskJobs, "count": count})
}
