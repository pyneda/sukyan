package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"net/http"
	"strconv"
	"strings"
)

// @Summary Search Task Jobs
// @Description Allows to filter and search task jobs
// @Tags Tasks
// @Accept  json
// @Produce  json
// @Param task query int true "Task ID"
// @Param page_size query int false "Number of items per page" default(50)
// @Param page query int false "Page number" default(1)
// @Param status query string false "Comma-separated list of statuses to filter"
// @Param title query string false "Comma-separated list of titles to filter"
// @Param status_codes query string false "Comma-separated list of status codes to filter"
// @Param methods query string false "Comma-separated list of methods to filter"
// @Param sort_by query string false "Field to sort by" Enums(id, history_method, history_url, history_status, history_parameters_count, title, status, started_at, completed_at, created_at, updated_at)
// @Param sort_order query string false "Sort order" Enums(asc, desc)
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/tasks/jobs [get]
func FindTaskJobs(c *fiber.Ctx) error {
	unparsedPageSize := c.Query("page_size", "50")
	unparsedPage := c.Query("page", "1")
	unparsedStatuses := c.Query("status")
	unparsedTitles := c.Query("title")
	unparsedStatusCodes := c.Query("status_codes")
	unparsedMethods := c.Query("methods")
	unparsedSortBy := c.Query("sort_by")
	unparsedSortOrder := c.Query("sort_order")

	taskID, err := parseTaskID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":   "Invalid task",
			"message": "The provided task ID does not seem valid",
		})
	}

	var statuses []string
	var titles []string
	var statusCodes []int
	var methods []string

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

	if unparsedStatusCodes != "" {
		for _, s := range strings.Split(unparsedStatusCodes, ",") {
			statusCode, err := strconv.Atoi(s)
			if err != nil {
				log.Error().Err(err).Msg("Error parsing status_codes parameter query")
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Invalid status_codes parameter"})
			}
			statusCodes = append(statusCodes, statusCode)
		}
	}

	if unparsedMethods != "" {
		for _, method := range strings.Split(unparsedMethods, ",") {
			if IsValidFilterHTTPMethod(method) {
				methods = append(methods, method)
			} else {
				log.Warn().Str("method", method).Msg("Invalid filter HTTP method provided")
			}
		}
	}

	taskJobs, count, err := db.Connection.ListTaskJobs(db.TaskJobFilter{
		Pagination: db.Pagination{
			Page:     page,
			PageSize: pageSize,
		},
		Statuses:    statuses,
		Titles:      titles,
		TaskID:      taskID,
		StatusCodes: statusCodes,
		Methods:     methods,
		SortBy:      unparsedSortBy,
		SortOrder:   unparsedSortOrder,
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(http.StatusOK).JSON(fiber.Map{"data": taskJobs, "count": count})

}
