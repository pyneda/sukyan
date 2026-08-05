package api

import (
	"net/http"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

const (
	// The overview renders 24 hours at 15-minute resolution. Both are fixed
	// server-side: the chart's geometry assumes this series length, and letting
	// callers pick arbitrary values would let one request group over the whole
	// scan_jobs table.
	pulseBucketSeconds = 900
	pulseBucketCount   = 96

	// Enough rows to fill the feed on a tall screen without paging.
	maxDeploymentFindings = 50
)

// GetDeploymentPulseHandler returns deployment-wide activity over the last 24 hours.
//
// @Summary Retrieves finished jobs and recorded issues bucketed over the last 24 hours.
// @Description Returns a fixed-length series of 96 fifteen-minute buckets, each holding
// the jobs that completed or failed in it and the issues recorded in it grouped by
// severity. Drives the activity chart on the global overview.
// @Tags Stats
// @Accept json
// @Produce json
// @Success 200 {object} db.DeploymentPulse "Successfully retrieved deployment pulse"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/stats/deployment/pulse [get]
func GetDeploymentPulseHandler(c fiber.Ctx) error {
	pulse, err := db.Connection().GetDeploymentPulse(pulseBucketSeconds, pulseBucketCount)
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve deployment pulse")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve deployment pulse",
			Message: "An unexpected error occurred while bucketing deployment activity. Please try again later.",
		})
	}
	return c.Status(http.StatusOK).JSON(pulse)
}

// GetDeploymentFindingsHandler returns the newest issues across every workspace.
//
// @Summary Lists the newest findings across all workspaces.
// @Description Returns the newest issues recorded anywhere in the deployment, plus
// severity totals for the period starting at `since`. Unlike /api/v1/issues this is not
// scoped to a single workspace. The listed rows are always the newest ones so a quiet
// deployment still shows what it last found; only the totals honour `since`. False
// positives and informational severities are excluded.
// @Tags Stats
// @Accept json
// @Produce json
// @Param since query string false "RFC3339 timestamp bounding the totals. Does not filter the listed rows."
// @Param limit query int false "Maximum rows to list, 1-50 (default 12)."
// @Success 200 {object} db.DeploymentFindings "Successfully retrieved deployment findings"
// @Failure 400 {object} ErrorResponse "Invalid since or limit"
// @Failure 500 {object} ErrorResponse "Internal server error"
// @Security ApiKeyAuth
// @Router /api/v1/stats/deployment/findings [get]
func GetDeploymentFindingsHandler(c fiber.Ctx) error {
	var since *time.Time
	if raw := c.Query("since"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid since",
				Message: "since must be an RFC3339 timestamp, for example 2026-08-03T04:00:00Z",
			})
		}
		since = &parsed
	}

	limit := fiber.Query[int](c, "limit", 12)
	if limit < 1 || limit > maxDeploymentFindings {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid limit",
			Message: "limit must be between 1 and 50",
		})
	}

	findings, err := db.Connection().GetDeploymentFindings(since, limit)
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve deployment findings")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to retrieve deployment findings",
			Message: "An unexpected error occurred while fetching findings. Please try again later.",
		})
	}
	return c.Status(http.StatusOK).JSON(findings)
}
