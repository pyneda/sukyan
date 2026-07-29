package api

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// WorkspaceStats retrieves statistics for a given workspace.
//
// @Summary Retrieves workspace statistics including counts of issues, history entries, JWTs,
// websocket connections, tasks, etc
// @Tags Stats
// @Accept json
// @Produce json
// @Param workspace_id path int true "Workspace ID"
// @Success 200 {object} db.WorkspaceStats "Successfully retrieved stats"
// @Failure 400 {object} ErrorResponse "Invalid workspace ID"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/stats/workspace [get]
func WorkspaceStats(c *fiber.Ctx) error {
	workspaceID, err := parseWorkspaceID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid workspace",
			Message: "The provided workspace ID does not seem valid. Please provide a valid workspace ID.",
		})
	}

	metrics, err := db.Connection().GetWorkspaceStats(workspaceID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve workspace statistics")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve workspace statistics",
			Message: "An unexpected error occurred while fetching workspace statistics. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(metrics)
}

// SystemStats retrieves overall system statistics.
//
// @Summary Retrieves system statistics such as the current database size.
// @Tags Stats
// @Accept json
// @Produce json
// @Success 200 {object} db.SystemStats "Successfully retrieved system stats"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/stats/system [get]
func SystemStats(c *fiber.Ctx) error {
	stats, err := db.Connection().GetSystemStats()
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve system statistics")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve system statistics",
			Message: "An unexpected error occurred while fetching system statistics. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(stats)
}

// WorkerNodesResponse contains the list of worker nodes and aggregate stats
type WorkerNodesResponse struct {
	Nodes []*db.WorkerNode    `json:"nodes"`
	Stats *db.WorkerNodeStats `json:"stats"`
}

// ListWorkerNodes retrieves all registered worker nodes.
//
// @Summary Retrieves the list of registered worker nodes with their status and statistics.
// @Description Returns all worker nodes that have registered with the system, including their
// heartbeat status, job counts, and whether they are currently active or stale.
// @Tags Stats
// @Accept json
// @Produce json
// @Success 200 {object} WorkerNodesResponse "Successfully retrieved worker nodes"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/stats/workers [get]
func ListWorkerNodes(c *fiber.Ctx) error {
	nodes, err := db.Connection().GetAllWorkerNodes()
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve worker nodes")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve worker nodes",
			Message: "An unexpected error occurred while fetching worker nodes. Please try again later.",
		})
	}

	stats, err := db.Connection().GetWorkerNodeStats()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to retrieve worker node stats")
		// Continue without stats
	}

	// Mark stale workers in the response
	heartbeatThreshold := 2 * time.Minute
	for _, node := range nodes {
		if node.Status == db.WorkerNodeStatusRunning && time.Since(node.LastSeenAt) > heartbeatThreshold {
			// Add stale indicator - we could add a computed field to the response
			// For now, the frontend can compute this from LastSeenAt
		}
	}

	return c.Status(http.StatusOK).JSON(WorkerNodesResponse{
		Nodes: nodes,
		Stats: stats,
	})
}

// CleanupStaleWorkers marks stale workers as stopped and resets their jobs.
//
// @Summary Cleanup stale worker nodes and reset their claimed jobs.
// @Description Identifies worker nodes that haven't sent a heartbeat within the threshold,
// marks them as stopped, and resets any jobs they had claimed back to pending status.
// @Tags Stats
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Cleanup results"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/stats/workers/cleanup [post]
func CleanupStaleWorkers(c *fiber.Ctx) error {
	heartbeatThreshold := 2 * time.Minute

	resetCount, affectedScanIDs, err := db.Connection().ResetJobsFromStaleWorkers(heartbeatThreshold)
	if err != nil {
		log.Error().Err(err).Msg("Failed to cleanup stale workers")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to cleanup stale workers",
			Message: "An unexpected error occurred during cleanup. Please try again later.",
		})
	}

	// Update job counts for affected scans
	for _, scanID := range affectedScanIDs {
		db.Connection().UpdateScanJobCounts(scanID)
	}

	return c.Status(http.StatusOK).JSON(fiber.Map{
		"jobs_reset":        resetCount,
		"affected_scan_ids": affectedScanIDs,
		"message":           "Stale workers cleaned up successfully",
	})
}

// WorkspaceRollupsResponse is the paginated response for cross-workspace rollups.
type WorkspaceRollupsResponse struct {
	Data  []db.WorkspaceRollup `json:"data"`
	Count int64                `json:"count"`
}

// ListWorkspaceRollupsHandler returns one summary row per workspace.
//
// @Summary Lists all workspaces with issue, history and scan rollups.
// @Description Returns a paginated, sortable summary of every workspace, used by
// the global workspaces view. Uses grouped queries rather than per-workspace counts.
// @Tags Stats
// @Accept json
// @Produce json
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Param query query string false "Filter by workspace title or code"
// @Param sort_by query string false "One of title, critical, high, issues, active_scans, last_activity"
// @Param sort_order query string false "asc or desc"
// @Success 200 {object} WorkspaceRollupsResponse "Successfully retrieved workspace rollups"
// @Failure 400 {object} ErrorResponse "Invalid sort parameter"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/stats/workspaces [get]
func ListWorkspaceRollupsHandler(c *fiber.Ctx) error {
	filter := db.WorkspaceRollupFilter{
		Query:     c.Query("query"),
		SortBy:    c.Query("sort_by"),
		SortOrder: c.Query("sort_order"),
		Pagination: db.Pagination{
			Page:     c.QueryInt("page", 1),
			PageSize: c.QueryInt("page_size", 25),
		},
	}

	if filter.SortBy != "" && !slices.Contains(db.WorkspaceRollupSortFields, filter.SortBy) {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid sort field",
			Message: "sort_by must be one of: " + strings.Join(db.WorkspaceRollupSortFields, ", "),
		})
	}
	if filter.SortOrder != "" && filter.SortOrder != "asc" && filter.SortOrder != "desc" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid sort order",
			Message: "sort_order must be either asc or desc",
		})
	}

	// db.Pagination.GetData() normalises Page == 0 to 1 but does not guard a
	// negative Page, and ListWorkspaceRollups slices its result with that raw
	// offset. A negative page therefore produces a negative slice offset and
	// panics. Reject non-positive page/page_size here, at the API boundary,
	// rather than touching the DB layer.
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

	rows, count, err := db.Connection().ListWorkspaceRollups(filter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve workspace rollups")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve workspace rollups",
			Message: "An unexpected error occurred while fetching workspace rollups. Please try again later.",
		})
	}

	return c.Status(http.StatusOK).JSON(WorkspaceRollupsResponse{Data: rows, Count: count})
}
