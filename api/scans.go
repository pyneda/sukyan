// Package api provides REST API handlers for the scan engine.
package api

import (
	"fmt"
	"strconv"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/manager"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// CreateScanInput represents the input for creating a new scan
type CreateScanInput struct {
	Title       string   `json:"title" validate:"required,min=1,max=255"`
	WorkspaceID uint     `json:"workspace_id" validate:"required,numeric"`
	StartURLs   []string `json:"start_urls" validate:"required,min=1,dive,url"`

	// Optional scan configuration
	MaxDepth        int  `json:"max_depth" validate:"omitempty,min=1,max=100"`
	MaxPagesToCrawl int  `json:"max_pages_to_crawl" validate:"omitempty,min=1"`
	PoolSize        int  `json:"pool_size" validate:"omitempty,min=1,max=50"`
	CrawlEnabled    bool `json:"crawl_enabled"`

	// Scan mode options
	Mode string `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`

	// UseOrchestrator enables the new scan engine with full phase management
	// When true, the scan will go through: crawl → fingerprint → discovery → nuclei → active_scan → websocket
	UseOrchestrator bool `json:"use_orchestrator"`
}

// UpdateScanInput represents the input for updating a scan
type UpdateScanInput struct {
	Title             *string `json:"title" validate:"omitempty,min=1,max=255"`
	MaxConcurrentJobs *int    `json:"max_concurrent_jobs" validate:"omitempty,min=1"`
}

// ScanStats contains statistics for a scan
type ScanStats struct {
	Requests db.RequestsStats `json:"requests"`
	Issues   db.IssuesStats   `json:"issues"`
}

// Summary returns a formatted summary of the scan stats
func (s ScanStats) Summary() string {
	return fmt.Sprintf("Requests:\n - Crawler: %d\n - Scanner: %d\n\nIssues:\n - Unknown: %d\n - Info: %d\n - Low: %d\n - Medium: %d\n - High: %d\n - Critical: %d\n",
		s.Requests.Crawler, s.Requests.Scanner,
		s.Issues.Unknown, s.Issues.Info, s.Issues.Low, s.Issues.Medium, s.Issues.High, s.Issues.Critical)
}

// ScanJobStats contains statistics for a scan job
type ScanJobStats struct {
	Requests int64          `json:"requests"`
	Issues   db.IssuesStats `json:"issues"`
	OOBTests int64          `json:"oob_tests"`
}

// ScanResponse represents a scan with its statistics for API responses
type ScanResponse struct {
	// Base fields
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Core fields
	WorkspaceID    uint                    `json:"workspace_id"`
	Title          string                  `json:"title"`
	Status         db.ScanStatus           `json:"status"`
	Phase          db.ScanPhase            `json:"phase"`
	PreviousStatus db.ScanStatus           `json:"previous_status,omitempty"`
	Options        options.FullScanOptions `json:"options"`

	// Rate limiting and circuit breaker fields
	MaxRPS              *int       `json:"max_rps,omitempty"`
	MaxConcurrentJobs   *int       `json:"max_concurrent_jobs,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastFailureAt       *time.Time `json:"last_failure_at,omitempty"`
	ThrottledUntil      *time.Time `json:"throttled_until,omitempty"`

	// Job counters for progress tracking
	TotalJobsCount     int `json:"total_jobs_count"`
	PendingJobsCount   int `json:"pending_jobs_count"`
	RunningJobsCount   int `json:"running_jobs_count"`
	CompletedJobsCount int `json:"completed_jobs_count"`
	FailedJobsCount    int `json:"failed_jobs_count"`

	// Timing fields
	StartedAt   *time.Time `json:"started_at,omitempty"`
	PausedAt    *time.Time `json:"paused_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Checkpoint for restart recovery
	Checkpoint *db.ScanCheckpoint `json:"checkpoint,omitempty"`

	// Computed fields
	Progress float64   `json:"progress"`
	Stats    ScanStats `json:"stats"`
}

// ScanResponseFromDBScan creates a ScanResponse from a db.Scan and its stats
func ScanResponseFromDBScan(scan *db.Scan, stats ScanStats) ScanResponse {
	return ScanResponse{
		ID:                  scan.ID,
		CreatedAt:           scan.CreatedAt,
		UpdatedAt:           scan.UpdatedAt,
		WorkspaceID:         scan.WorkspaceID,
		Title:               scan.Title,
		Status:              scan.Status,
		Phase:               scan.Phase,
		PreviousStatus:      scan.PreviousStatus,
		Options:             scan.Options,
		MaxRPS:              scan.MaxRPS,
		MaxConcurrentJobs:   scan.MaxConcurrentJobs,
		ConsecutiveFailures: scan.ConsecutiveFailures,
		LastFailureAt:       scan.LastFailureAt,
		ThrottledUntil:      scan.ThrottledUntil,
		TotalJobsCount:      scan.TotalJobsCount,
		PendingJobsCount:    scan.PendingJobsCount,
		RunningJobsCount:    scan.RunningJobsCount,
		CompletedJobsCount:  scan.CompletedJobsCount,
		FailedJobsCount:     scan.FailedJobsCount,
		StartedAt:           scan.StartedAt,
		PausedAt:            scan.PausedAt,
		CompletedAt:         scan.CompletedAt,
		Checkpoint:          scan.Checkpoint,
		Progress:            scan.Progress(),
		Stats:               stats,
	}
}

// ScanListResponse represents a paginated list of scans
type ScanListResponse struct {
	Scans []ScanResponse `json:"scans"`
	Count int64          `json:"count"`
}

// ScanJobResponse represents a scan job for API responses
type ScanJobResponse struct {
	// Base fields
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Core fields
	ScanID      uint             `json:"scan_id"`
	Status      db.ScanJobStatus `json:"status"`
	JobType     db.ScanJobType   `json:"job_type"`
	Priority    int              `json:"priority"`
	WorkspaceID uint             `json:"workspace_id"`

	// Worker tracking
	WorkerID  string     `json:"worker_id,omitempty"`
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`

	// Target information
	TargetHost            string `json:"target_host"`
	URL                   string `json:"url"`
	Method                string `json:"method"`
	HistoryID             *uint  `json:"history_id,omitempty"`
	WebSocketConnectionID *uint  `json:"websocket_connection_id,omitempty"`

	// Execution tracking
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Result tracking
	ErrorType    *string `json:"error_type,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`
	HTTPStatus   *int    `json:"http_status,omitempty"`
	IssuesFound  int     `json:"issues_found"`

	// Checkpoint for resume
	Checkpoint *db.ScanJobCheckpoint `json:"checkpoint,omitempty"`

	// Statistics
	Stats ScanJobStats `json:"stats"`
}

// ScanJobResponseFromDBScanJob creates a ScanJobResponse from a db.ScanJob
func ScanJobResponseFromDBScanJob(job *db.ScanJob, stats ScanJobStats) ScanJobResponse {
	return ScanJobResponse{
		ID:                    job.ID,
		CreatedAt:             job.CreatedAt,
		UpdatedAt:             job.UpdatedAt,
		ScanID:                job.ScanID,
		Status:                job.Status,
		JobType:               job.JobType,
		Priority:              job.Priority,
		WorkspaceID:           job.WorkspaceID,
		WorkerID:              job.WorkerID,
		ClaimedAt:             job.ClaimedAt,
		TargetHost:            job.TargetHost,
		URL:                   job.URL,
		Method:                job.Method,
		HistoryID:             job.HistoryID,
		WebSocketConnectionID: job.WebSocketConnectionID,
		Attempts:              job.Attempts,
		MaxAttempts:           job.MaxAttempts,
		StartedAt:             job.StartedAt,
		CompletedAt:           job.CompletedAt,
		ErrorType:             job.ErrorType,
		ErrorMessage:          job.ErrorMessage,
		HTTPStatus:            job.HTTPStatus,
		IssuesFound:           job.IssuesFound,
		Checkpoint:            job.Checkpoint,
		Stats:                 stats,
	}
}

// ScanJobsListResponse represents a paginated list of scan jobs
type ScanJobsListResponse struct {
	Jobs  []ScanJobResponse `json:"jobs"`
	Count int64             `json:"count"`
}

// ScanStatsResponse represents scan statistics
type ScanStatsResponse struct {
	JobStats map[db.ScanJobStatus]int64 `json:"job_stats"`
}

// scanManager holds the singleton ScanManager instance.
// It's initialized by SetScanManager during server startup.
var scanManager *manager.ScanManager

// SetScanManager sets the global scan manager instance.
// Called during API server initialization.
func SetScanManager(sm *manager.ScanManager) {
	scanManager = sm
}

// GetScanManager returns the global scan manager instance.
func GetScanManager() *manager.ScanManager {
	return scanManager
}

// ListScansHandler lists all scans with optional filtering
// @Summary List scans
// @Description Lists all scans with optional filtering and pagination
// @Tags Scans
// @Accept json
// @Produce json
// @Param query query string false "Search by scan title"
// @Param workspace query int false "Filter by workspace ID"
// @Param status query string false "Filter by status (comma-separated)"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} ScanListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans [get]
func ListScansHandler(c *fiber.Ctx) error {
	filter := db.ScanFilter{
		Pagination: db.Pagination{
			Page:     1,
			PageSize: 20,
		},
		SortBy:    "id",
		SortOrder: "desc",
	}

	// Parse query parameter for searching scan titles
	if query := c.Query("query"); query != "" {
		filter.Query = query
	}

	// Parse workspace ID
	if workspaceStr := c.Query("workspace"); workspaceStr != "" {
		workspaceID, err := parseUint(workspaceStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid workspace parameter",
				Message: err.Error(),
			})
		}
		filter.WorkspaceID = workspaceID
	}

	// Parse status filter
	if statusStr := c.Query("status"); statusStr != "" {
		statuses := parseCommaSeparatedStrings(statusStr)
		for _, s := range statuses {
			filter.Statuses = append(filter.Statuses, db.ScanStatus(s))
		}
	}

	// Parse pagination
	if pageStr := c.Query("page"); pageStr != "" {
		page, err := parseInt(pageStr)
		if err == nil && page > 0 {
			filter.Pagination.Page = page
		}
	}

	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		pageSize, err := parseInt(pageSizeStr)
		if err == nil && pageSize > 0 && pageSize <= 100 {
			filter.Pagination.PageSize = pageSize
		}
	}

	// Parse sorting
	if sortBy := c.Query("sort_by"); sortBy != "" {
		filter.SortBy = sortBy
	}
	if sortOrder := c.Query("sort_order"); sortOrder != "" {
		filter.SortOrder = sortOrder
	}

	// Query database
	scans, count, err := db.Connection().ListScans(filter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list scans")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to list scans",
			Message: err.Error(),
		})
	}

	// Build response with stats
	scanResponses := make([]ScanResponse, 0, len(scans))
	for _, scan := range scans {
		stats, _ := db.Connection().GetScanStats(scan.ID)
		scanResponses = append(scanResponses, ScanResponseFromDBScan(scan, ScanStats{
			Requests: stats.Requests,
			Issues:   stats.Issues,
		}))
	}

	return c.JSON(ScanListResponse{
		Scans: scanResponses,
		Count: count,
	})
}

// GetScanHandler retrieves a scan by ID
// @Summary Get scan
// @Description Retrieves a scan by ID
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Success 200 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id} [get]
func GetScanHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	scan, err := db.Connection().GetScanByID(scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: err.Error(),
		})
	}

	stats, _ := db.Connection().GetScanStats(scanID)
	return c.JSON(ScanResponseFromDBScan(scan, ScanStats{
		Requests: stats.Requests,
		Issues:   stats.Issues,
	}))
}

// UpdateScanHandler updates scan settings (title and max concurrent jobs)
// @Summary Update scan
// @Description Updates scan title and/or max concurrent jobs
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Param input body UpdateScanInput true "Update scan input"
// @Success 200 {object} ScanResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id} [patch]
func UpdateScanHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	// Parse and validate input
	validate := validator.New()
	input := new(UpdateScanInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid request body",
			Message: err.Error(),
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: err.Error(),
		})
	}

	// Fetch the scan
	scan, err := db.Connection().GetScanByID(scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: err.Error(),
		})
	}

	// Check if scan is in a terminal state - updates not allowed
	if scan.Status == db.ScanStatusCompleted || scan.Status == db.ScanStatusCancelled || scan.Status == db.ScanStatusFailed {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Cannot update scan",
			Message: fmt.Sprintf("Scan is in %s state and cannot be updated", scan.Status),
		})
	}

	// Update fields if provided
	if input.Title != nil {
		scan.Title = *input.Title
	}
	if input.MaxConcurrentJobs != nil {
		scan.MaxConcurrentJobs = input.MaxConcurrentJobs
	}

	// Save changes to database
	updatedScan, err := db.Connection().UpdateScan(scan)
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to update scan")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to update scan",
			Message: err.Error(),
		})
	}

	// Return updated scan with stats
	stats, _ := db.Connection().GetScanStats(scanID)
	return c.JSON(ScanResponseFromDBScan(updatedScan, ScanStats{
		Requests: stats.Requests,
		Issues:   stats.Issues,
	}))
}

// PauseScanHandler pauses a running scan
// @Summary Pause scan
// @Description Pauses a running scan
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Success 200 {object} db.Scan
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id}/pause [post]
func PauseScanHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	if scanManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Error:   "Scan service unavailable",
			Message: "Scan manager is not initialized",
		})
	}

	if err := scanManager.PauseScan(scanID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Failed to pause scan",
			Message: err.Error(),
		})
	}

	// Return updated scan
	scan, err := scanManager.GetScan(scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: err.Error(),
		})
	}

	return c.JSON(scan)
}

// ResumeScanHandler resumes a paused scan
// @Summary Resume scan
// @Description Resumes a paused scan
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Success 200 {object} db.Scan
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id}/resume [post]
func ResumeScanHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	if scanManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Error:   "Scan service unavailable",
			Message: "Scan manager is not initialized",
		})
	}

	if err := scanManager.ResumeScan(scanID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Failed to resume scan",
			Message: err.Error(),
		})
	}

	// Return updated scan
	scan, err := scanManager.GetScan(scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: err.Error(),
		})
	}

	return c.JSON(scan)
}

// CancelScanHandler cancels a scan
// @Summary Cancel scan
// @Description Cancels a scan and all its pending jobs
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Success 200 {object} db.Scan
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id}/cancel [post]
func CancelScanHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	if scanManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Error:   "Scan service unavailable",
			Message: "Scan manager is not initialized",
		})
	}

	if err := scanManager.CancelScan(scanID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Failed to cancel scan",
			Message: err.Error(),
		})
	}

	// Return updated scan
	scan, err := scanManager.GetScan(scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: err.Error(),
		})
	}

	return c.JSON(scan)
}

// DeleteScanHandler deletes a scan
// @Summary Delete scan
// @Description Deletes a scan and all its jobs
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id} [delete]
func DeleteScanHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	// Check scan exists
	exists, _ := db.Connection().ScanExists(scanID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: "The specified scan does not exist",
		})
	}

	if err := db.Connection().DeleteScan(scanID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to delete scan",
			Message: err.Error(),
		})
	}

	return c.JSON(ActionResponse{
		Message: "Scan deleted successfully",
	})
}

// GetScanJobHandler retrieves a scan job by ID
// @Summary Get scan job
// @Description Retrieves a scan job by ID with its statistics
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan Job ID"
// @Success 200 {object} ScanJobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scan-jobs/{id} [get]
func GetScanJobHandler(c *fiber.Ctx) error {
	jobID, err := parseScanJobIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan job ID",
			Message: err.Error(),
		})
	}

	job, err := db.Connection().GetScanJobByID(jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan job not found",
			Message: err.Error(),
		})
	}

	stats, _ := db.Connection().GetScanJobStatsForJob(jobID)
	return c.JSON(ScanJobResponseFromDBScanJob(job, ScanJobStats{
		Requests: stats.Requests,
		Issues:   stats.Issues,
		OOBTests: stats.OOBTests,
	}))
}

// parseScanJobIDParam parses the scan job ID from the URL path parameter
func parseScanJobIDParam(c *fiber.Ctx) (uint, error) {
	idStr := c.Params("id")
	id64, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id64), nil
}

// CancelScanJobHandler cancels a specific scan job
// @Summary Cancel scan job
// @Description Cancels a specific scan job. If the job is running, it will be stopped.
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Param job_id path int true "Job ID"
// @Success 200 {object} ScanJobResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id}/jobs/{job_id}/cancel [post]
func CancelScanJobHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	jobIDStr := c.Params("job_id")
	jobID64, err := strconv.ParseUint(jobIDStr, 10, 32)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid job ID",
			Message: err.Error(),
		})
	}
	jobID := uint(jobID64)

	// Get the job and verify it belongs to the scan
	job, err := db.Connection().GetScanJobByID(jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan job not found",
			Message: err.Error(),
		})
	}

	if job.ScanID != scanID {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Job does not belong to scan",
			Message: "The specified job does not belong to the specified scan",
		})
	}

	// Check if job can be cancelled
	if job.Status == db.ScanJobStatusCompleted || job.Status == db.ScanJobStatusCancelled {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Job cannot be cancelled",
			Message: "Job is already " + string(job.Status),
		})
	}

	// Cancel the job
	if err := db.Connection().SetScanJobStatus(jobID, db.ScanJobStatusCancelled); err != nil {
		log.Error().Err(err).Uint("job_id", jobID).Msg("Failed to cancel job")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to cancel job",
			Message: err.Error(),
		})
	}

	// Fetch updated job
	job, _ = db.Connection().GetScanJobByID(jobID)
	stats, _ := db.Connection().GetScanJobStatsForJob(jobID)

	log.Info().Uint("job_id", jobID).Uint("scan_id", scanID).Msg("Scan job cancelled")

	return c.JSON(ScanJobResponseFromDBScanJob(job, ScanJobStats{
		Requests: stats.Requests,
		Issues:   stats.Issues,
		OOBTests: stats.OOBTests,
	}))
}

// GetScanJobsHandler lists jobs for a scan
// @Summary Get scan jobs
// @Description Lists all jobs for a scan with optional filtering
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Param query query string false "Search by URL or error message"
// @Param status query string false "Filter by status (comma-separated)"
// @Param job_type query string false "Filter by job type (comma-separated)"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} ScanJobsListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id}/jobs [get]
func GetScanJobsHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	// Check scan exists
	exists, _ := db.Connection().ScanExists(scanID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: "The specified scan does not exist",
		})
	}

	filter := db.ScanJobFilter{
		ScanID: scanID,
		Pagination: db.Pagination{
			Page:     1,
			PageSize: 50,
		},
		SortBy:    "id",
		SortOrder: "desc",
	}

	// Parse query parameter for searching URL and error message
	if query := c.Query("query"); query != "" {
		filter.Query = query
	}

	// Parse status filter
	if statusStr := c.Query("status"); statusStr != "" {
		statuses := parseCommaSeparatedStrings(statusStr)
		for _, s := range statuses {
			filter.Statuses = append(filter.Statuses, db.ScanJobStatus(s))
		}
	}

	// Parse job type filter
	if jobTypeStr := c.Query("job_type"); jobTypeStr != "" {
		jobTypes := parseCommaSeparatedStrings(jobTypeStr)
		for _, jt := range jobTypes {
			filter.JobTypes = append(filter.JobTypes, db.ScanJobType(jt))
		}
	}

	// Parse pagination
	if pageStr := c.Query("page"); pageStr != "" {
		page, err := parseInt(pageStr)
		if err == nil && page > 0 {
			filter.Pagination.Page = page
		}
	}

	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		pageSize, err := parseInt(pageSizeStr)
		if err == nil && pageSize > 0 && pageSize <= 100 {
			filter.Pagination.PageSize = pageSize
		}
	}

	// Parse sorting
	if sortBy := c.Query("sort_by"); sortBy != "" {
		filter.SortBy = sortBy
	}
	if sortOrder := c.Query("sort_order"); sortOrder != "" {
		filter.SortOrder = sortOrder
	}

	// Query database
	jobs, count, err := db.Connection().ListScanJobs(filter)
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to list scan jobs")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to list scan jobs",
			Message: err.Error(),
		})
	}

	// Build response with stats
	jobResponses := make([]ScanJobResponse, 0, len(jobs))
	for _, job := range jobs {
		stats, _ := db.Connection().GetScanJobStatsForJob(job.ID)
		jobResponses = append(jobResponses, ScanJobResponseFromDBScanJob(job, ScanJobStats{
			Requests: stats.Requests,
			Issues:   stats.Issues,
			OOBTests: stats.OOBTests,
		}))
	}

	return c.JSON(ScanJobsListResponse{
		Jobs:  jobResponses,
		Count: count,
	})
}

// GetScanStatsHandler returns statistics for a scan
// @Summary Get scan statistics
// @Description Returns job statistics for a scan
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Success 200 {object} ScanStatsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id}/stats [get]
func GetScanStatsHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	// Check scan exists
	exists, _ := db.Connection().ScanExists(scanID)
	if !exists {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: "The specified scan does not exist",
		})
	}

	stats, err := db.Connection().GetScanJobStats(scanID)
	if err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to get scan stats")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to get scan statistics",
			Message: err.Error(),
		})
	}

	return c.JSON(ScanStatsResponse{
		JobStats: stats,
	})
}

// ScheduleScanHistoryItemsInput represents input for scheduling history item scans
type ScheduleScanHistoryItemsInput struct {
	HistoryIDs []uint `json:"history_ids" validate:"required,min=1"`
	// Scan mode: fast, smart, fuzz
	Mode string `json:"mode" validate:"omitempty,oneof=fast smart fuzz"`
}

// ScheduleHistoryItemScansHandler schedules active scans for history items
// @Summary Schedule history item scans
// @Description Schedules active scans for the specified history items
// @Tags Scans
// @Accept json
// @Produce json
// @Param id path int true "Scan ID"
// @Param input body ScheduleScanHistoryItemsInput true "History items to scan"
// @Success 200 {object} ActionResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Security ApiKeyAuth
// @Router /api/v1/scans/{id}/schedule-items [post]
func ScheduleHistoryItemScansHandler(c *fiber.Ctx) error {
	scanID, err := parseScanIDParam(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid scan ID",
			Message: err.Error(),
		})
	}

	validate := validator.New()
	input := new(ScheduleScanHistoryItemsInput)

	if err := c.BodyParser(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid request body",
			Message: err.Error(),
		})
	}

	if err := validate.Struct(input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Validation failed",
			Message: err.Error(),
		})
	}

	// Get scan
	scan, err := db.Connection().GetScanByID(scanID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Scan not found",
			Message: err.Error(),
		})
	}

	if scanManager == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(ErrorResponse{
			Error:   "Scan service unavailable",
			Message: "Scan manager is not initialized",
		})
	}

	// Fetch history items
	var historyItems []*db.History
	for _, historyID := range input.HistoryIDs {
		history, err := db.Connection().GetHistory(historyID)
		if err != nil {
			continue // Skip invalid history IDs
		}
		historyItems = append(historyItems, &history)
	}

	if len(historyItems) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "No valid history items",
			Message: "None of the provided history IDs are valid",
		})
	}

	// Create scan options
	scanMode := options.ScanModeSmart
	switch input.Mode {
	case "fast":
		scanMode = options.ScanModeFast
	case "fuzz":
		scanMode = options.ScanModeFuzz
	}

	opts := options.HistoryItemScanOptions{
		Mode: scanMode,
	}

	// Schedule the scans
	if err := scanManager.ScheduleHistoryItemScan(scanID, scan.WorkspaceID, historyItems, opts); err != nil {
		log.Error().Err(err).Uint("scan_id", scanID).Msg("Failed to schedule history item scans")
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Failed to schedule scans",
			Message: err.Error(),
		})
	}

	return c.JSON(ActionResponse{
		Message: "Scheduled " + strconv.Itoa(len(historyItems)) + " history item scans",
	})
}

// parseScanIDParam parses the scan ID from the URL path parameter
func parseScanIDParam(c *fiber.Ctx) (uint, error) {
	idStr := c.Params("id")
	id64, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id64), nil
}

// InitScanManager initializes and starts the global scan manager.
// This should be called during server startup after the database is ready.
func InitScanManager(interactionsManager *integrations.InteractionsManager, payloadGenerators []*generation.PayloadGenerator) error {
	cfg := manager.DefaultConfig()
	cfg.WorkerCount = viper.GetInt("scan.workers")
	sm := manager.New(cfg, db.Connection(), interactionsManager, payloadGenerators)

	if err := sm.Start(); err != nil {
		return err
	}

	SetScanManager(sm)
	log.Info().Msg("Scan manager initialized and started")
	return nil
}
