package api

import (
	"github.com/pyneda/sukyan/db"
	"net/http"
	"strconv"
	"strings"
	"github.com/rs/zerolog/log"

	"github.com/gofiber/fiber/v2"
)

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
		for _, status := range strings.Split(unparsedStatuses, ",") {
			statuses = append(statuses, status)
		}
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
