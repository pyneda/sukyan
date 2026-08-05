package api

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

type UsersResponse struct {
	Data    []db.User            `json:"data"`
	Count   int64                `json:"count"`
	Summary db.UserRosterSummary `json:"summary"`
}

// ListUsersHandler lists every account on the deployment.
//
// @Summary Lists all user accounts.
// @Description Returns one row per account with its role, status and sign-in
// activity, plus a deployment-wide roster summary. The summary is not affected by
// `query` — it describes the whole deployment, while `count` reflects the filter.
// Restricted to superusers and strictly read-only.
// @Tags Users
// @Accept json
// @Produce json
// @Param page query int false "Page number, 1-based (default 1)"
// @Param page_size query int false "Rows per page (default 25)"
// @Param query query string false "Case-insensitive email substring"
// @Param sort_by query string false "email, last_login, created_at, status or superuser"
// @Param sort_order query string false "asc or desc"
// @Success 200 {object} UsersResponse "Successfully retrieved users"
// @Failure 400 {object} ErrorResponse "Invalid parameters"
// @Failure 403 {object} ErrorResponse "Requires a superuser account"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/users [get]
func ListUsersHandler(c fiber.Ctx) error {
	filter := db.UserListFilter{
		Query:     c.Query("query"),
		SortBy:    c.Query("sort_by"),
		SortOrder: c.Query("sort_order"),
		Pagination: db.Pagination{
			Page:     fiber.Query[int](c, "page", 1),
			PageSize: fiber.Query[int](c, "page_size", 25),
		},
	}

	if filter.SortBy != "" && !slices.Contains(db.UserSortFields, filter.SortBy) {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid sort field",
			Message: "sort_by must be one of: " + strings.Join(db.UserSortFields, ", "),
		})
	}
	if filter.SortOrder != "" && filter.SortOrder != "asc" && filter.SortOrder != "desc" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid sort order",
			Message: "sort_order must be either asc or desc",
		})
	}
	// Pagination.GetData normalises Page == 0 but not a negative page, which
	// would reach the query as a negative offset. Reject it at the boundary.
	if filter.Pagination.Page < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid page",
			Message: "page must be 1 or greater",
		})
	}
	if filter.Pagination.PageSize < 1 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid page size",
			Message: "page_size must be 1 or greater",
		})
	}

	rows, count, summary, err := db.Connection().ListUsers(filter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve users")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve users",
			Message: "An unexpected error occurred while fetching users. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(UsersResponse{Data: rows, Count: count, Summary: summary})
}
