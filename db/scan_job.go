package db

import (
	"fmt"
	"time"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// MaxJobAttempts is the maximum number of times a job can be retried before being marked as permanently failed.
const MaxJobAttempts = 3

// DefaultJobTimeout returns the default timeout duration for a job type.
func DefaultJobTimeout(jobType ScanJobType) time.Duration {
	switch jobType {
	case ScanJobTypeCrawl:
		return 1 * time.Hour
	case ScanJobTypeDiscovery:
		return 5 * time.Minute
	case ScanJobTypeActiveScan:
		return 30 * time.Minute
	case ScanJobTypeNuclei:
		return 20 * time.Minute
	case ScanJobTypeWebSocketScan:
		return 15 * time.Minute
	case ScanJobTypeSiteBehavior:
		return 2 * time.Minute
	default:
		return 30 * time.Minute // Default fallback
	}
}

// ScanJobStatus represents the status of a scan job
type ScanJobStatus string

const (
	ScanJobStatusPending   ScanJobStatus = "pending"
	ScanJobStatusClaimed   ScanJobStatus = "claimed"
	ScanJobStatusRunning   ScanJobStatus = "running"
	ScanJobStatusCompleted ScanJobStatus = "completed"
	ScanJobStatusFailed    ScanJobStatus = "failed"
	ScanJobStatusCancelled ScanJobStatus = "cancelled"
)

// ScanJobType represents the type of scan job
type ScanJobType string

const (
	ScanJobTypeActiveScan    ScanJobType = "active_scan"
	ScanJobTypeWebSocketScan ScanJobType = "websocket_scan"
	ScanJobTypeDiscovery     ScanJobType = "discovery"
	ScanJobTypeNuclei        ScanJobType = "nuclei"
	ScanJobTypeCrawl         ScanJobType = "crawl"
	ScanJobTypeSiteBehavior  ScanJobType = "site_behavior"
)

// AuditType identifies audit modules for checkpoint tracking
type AuditType string

const (
	AuditTypeTemplateScanner AuditType = "template_scanner"
	AuditTypeXSS             AuditType = "xss"
	AuditTypeHostHeaderSSRF  AuditType = "host_header_ssrf"
	AuditTypeLog4Shell       AuditType = "log4shell"
	AuditTypeCRLF            AuditType = "crlf"
	AuditTypeOpenRedirect    AuditType = "open_redirect"
	AuditTypePathTraversal   AuditType = "path_traversal"
	AuditTypeCommandInj      AuditType = "command_injection"
	AuditTypeBypass          AuditType = "bypass"
)

// ScanJobCheckpoint stores job-level state for resume within a job
type ScanJobCheckpoint struct {
	// For active_scan jobs
	CompletedAudits          []string `json:"completed_audits,omitempty"`
	CurrentAudit             string   `json:"current_audit,omitempty"`
	CurrentInsertionPointIdx int      `json:"current_insertion_point_idx,omitempty"`
	LastPayloadIndex         int      `json:"last_payload_index,omitempty"`

	// For websocket_scan jobs
	MessagesProcessed int `json:"messages_processed,omitempty"`

	// For discovery jobs
	CompletedChecks []string `json:"completed_checks,omitempty"`
}

// ScanJob represents a single unit of scannable work
type ScanJob struct {
	BaseModel

	// Core fields
	ScanID      uint          `json:"scan_id" gorm:"index;not null"`
	Scan        Scan          `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Status      ScanJobStatus `json:"status" gorm:"index;size:50;not null;default:'pending'"`
	JobType     ScanJobType   `json:"job_type" gorm:"index;size:50;not null"`
	Priority    int           `json:"priority" gorm:"index;default:0"`
	WorkspaceID uint          `json:"workspace_id" gorm:"index;not null"`

	// Worker tracking
	WorkerID  string     `json:"worker_id,omitempty" gorm:"index;size:255"`
	ClaimedAt *time.Time `json:"claimed_at,omitempty"`

	// Target information
	TargetHost            string               `json:"target_host" gorm:"index;size:255"`
	URL                   string               `json:"url" gorm:"type:text"`
	Method                string               `json:"method" gorm:"size:10"`
	HistoryID             *uint                `json:"history_id,omitempty" gorm:"index"`
	History               *History             `json:"-" gorm:"foreignKey:HistoryID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WebSocketConnectionID *uint                `json:"websocket_connection_id,omitempty" gorm:"index"`
	WebSocketConnection   *WebSocketConnection `json:"-" gorm:"foreignKey:WebSocketConnectionID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	// Payload stores job-specific configuration (JSON)
	Payload []byte `json:"payload,omitempty" gorm:"type:jsonb"`

	// Execution tracking
	Attempts    int           `json:"attempts" gorm:"default:0"`
	MaxAttempts int           `json:"max_attempts" gorm:"default:3"`
	StartedAt   *time.Time    `json:"started_at,omitempty"`
	CompletedAt *time.Time    `json:"completed_at,omitempty"`
	MaxDuration time.Duration `json:"max_duration,omitempty" gorm:"default:1800000000000"` // Default 30 minutes in nanoseconds

	// Result tracking
	ErrorType    *string `json:"error_type,omitempty" gorm:"size:100"`
	ErrorMessage *string `json:"error_message,omitempty" gorm:"type:text"`
	HTTPStatus   *int    `json:"http_status,omitempty"`
	IssuesFound  int     `json:"issues_found" gorm:"default:0"`

	// Checkpoint for resume
	Checkpoint *ScanJobCheckpoint `json:"checkpoint,omitempty" gorm:"serializer:json"`
}

// IsTerminal returns true if the job is in a terminal state
func (j *ScanJob) IsTerminal() bool {
	return j.Status == ScanJobStatusCompleted ||
		j.Status == ScanJobStatusFailed ||
		j.Status == ScanJobStatusCancelled
}

// CanRetry returns true if the job can be retried
func (j *ScanJob) CanRetry() bool {
	return j.Attempts < j.MaxAttempts && j.Status == ScanJobStatusFailed
}

// TableHeaders returns table headers for CLI output
func (j ScanJob) TableHeaders() []string {
	return []string{"ID", "Scan ID", "Type", "Status", "URL", "Method", "Priority", "Attempts", "Issues"}
}

// TableRow returns table row for CLI output
func (j ScanJob) TableRow() []string {
	formattedURL := j.URL
	if len(j.URL) > PrintMaxURLLength {
		formattedURL = j.URL[0:PrintMaxURLLength] + "..."
	}
	return []string{
		fmt.Sprintf("%d", j.ID),
		fmt.Sprintf("%d", j.ScanID),
		string(j.JobType),
		string(j.Status),
		formattedURL,
		j.Method,
		fmt.Sprintf("%d", j.Priority),
		fmt.Sprintf("%d/%d", j.Attempts, j.MaxAttempts),
		fmt.Sprintf("%d", j.IssuesFound),
	}
}

// String provides a basic textual representation
func (j ScanJob) String() string {
	return fmt.Sprintf("ID: %d, ScanID: %d, Type: %s, Status: %s, URL: %s",
		j.ID, j.ScanID, j.JobType, j.Status, j.URL)
}

// Pretty provides a formatted representation
func (j ScanJob) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sScan ID:%s %d\n%sType:%s %s\n%sStatus:%s %s\n%sURL:%s %s\n%sMethod:%s %s\n%sPriority:%s %d\n%sAttempts:%s %d/%d\n%sIssues Found:%s %d\n",
		lib.Blue, lib.ResetColor, j.ID,
		lib.Blue, lib.ResetColor, j.ScanID,
		lib.Blue, lib.ResetColor, j.JobType,
		lib.Blue, lib.ResetColor, j.Status,
		lib.Blue, lib.ResetColor, j.URL,
		lib.Blue, lib.ResetColor, j.Method,
		lib.Blue, lib.ResetColor, j.Priority,
		lib.Blue, lib.ResetColor, j.Attempts, j.MaxAttempts,
		lib.Blue, lib.ResetColor, j.IssuesFound,
	)
}

// ScanJobFilter represents available scan job filters
type ScanJobFilter struct {
	Query      string          `json:"query" validate:"omitempty,ascii"`
	ScanID     uint            `json:"scan_id" validate:"omitempty,numeric"`
	Statuses   []ScanJobStatus `json:"statuses" validate:"omitempty"`
	JobTypes   []ScanJobType   `json:"job_types" validate:"omitempty"`
	TargetHost string          `json:"target_host" validate:"omitempty"`
	WorkerID   string          `json:"worker_id" validate:"omitempty"`
	Pagination Pagination      `json:"pagination"`
	SortBy     string          `json:"sort_by" validate:"omitempty,oneof=id created_at updated_at status priority"`
	SortOrder  string          `json:"sort_order" validate:"omitempty,oneof=asc desc"`
}

// DiscoveryJobExistsForURL checks if a discovery job already exists for the given scan and URL.
// This is used for deduplication in distributed environments where multiple orchestrators may run.
// Returns true if a non-cancelled discovery job exists for the scan+URL combination.
func (d *DatabaseConnection) DiscoveryJobExistsForURL(scanID uint, url string) (bool, error) {
	var count int64
	err := d.db.Model(&ScanJob{}).
		Where("scan_id = ? AND job_type = ? AND url = ? AND status != ?",
			scanID, ScanJobTypeDiscovery, url, ScanJobStatusCancelled).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CreateScanJob creates a new scan job
func (d *DatabaseConnection) CreateScanJob(job *ScanJob) (*ScanJob, error) {
	// Set default max_duration based on job type if not already set
	if job.MaxDuration == 0 {
		job.MaxDuration = DefaultJobTimeout(job.JobType)
	}
	result := d.db.Create(job)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("job", job).Msg("ScanJob creation failed")
	}
	return job, result.Error
}

// CreateScanJobs creates multiple scan jobs in a batch
func (d *DatabaseConnection) CreateScanJobs(jobs []*ScanJob) error {
	if len(jobs) == 0 {
		return nil
	}
	// Set default max_duration for each job based on job type if not already set
	for _, job := range jobs {
		if job.MaxDuration == 0 {
			job.MaxDuration = DefaultJobTimeout(job.JobType)
		}
	}
	result := d.db.Create(jobs)
	if result.Error != nil {
		log.Error().Err(result.Error).Int("count", len(jobs)).Msg("Batch ScanJob creation failed")
	}
	return result.Error
}

// GetScanJobByID retrieves a scan job by ID
func (d *DatabaseConnection) GetScanJobByID(id uint) (*ScanJob, error) {
	var job ScanJob
	err := d.db.Where("id = ?", id).First(&job).Error
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// UpdateScanJob updates a scan job
func (d *DatabaseConnection) UpdateScanJob(job *ScanJob) (*ScanJob, error) {
	result := d.db.Save(job)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("job", job).Msg("ScanJob update failed")
	}
	return job, result.Error
}

// ListScanJobs lists scan jobs with filters
func (d *DatabaseConnection) ListScanJobs(filter ScanJobFilter) (items []*ScanJob, count int64, err error) {
	query := d.db.Model(&ScanJob{})

	if filter.Query != "" {
		likeQuery := "%" + filter.Query + "%"
		query = query.Where("url ILIKE ? OR error_message ILIKE ?", likeQuery, likeQuery)
	}

	if filter.ScanID > 0 {
		query = query.Where("scan_id = ?", filter.ScanID)
	}

	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}

	if len(filter.JobTypes) > 0 {
		query = query.Where("job_type IN ?", filter.JobTypes)
	}

	if filter.TargetHost != "" {
		query = query.Where("target_host = ?", filter.TargetHost)
	}

	if filter.WorkerID != "" {
		query = query.Where("worker_id = ?", filter.WorkerID)
	}

	if err := query.Count(&count).Error; err != nil {
		return nil, 0, err
	}

	// Sorting
	validSortBy := map[string]bool{
		"id":         true,
		"created_at": true,
		"updated_at": true,
		"status":     true,
		"priority":   true,
	}

	order := "priority desc, id asc"
	if validSortBy[filter.SortBy] {
		sortOrder := "asc"
		if filter.SortOrder == "desc" {
			sortOrder = "desc"
		}
		order = filter.SortBy + " " + sortOrder
	}

	err = query.Scopes(Paginate(&filter.Pagination)).Order(order).Find(&items).Error
	return items, count, err
}

// ClaimScanJob atomically claims the next available job for a worker.
// This is used for distributed workers that can process jobs from any non-isolated scan.
// Uses FOR UPDATE SKIP LOCKED for atomic claiming.
func (d *DatabaseConnection) ClaimScanJob(workerID string) (*ScanJob, error) {
	var job ScanJob
	now := time.Now()

	// This query:
	// 1. Joins with scans to check scan status
	// 2. Only claims jobs from active (not paused/cancelled) scans
	// 3. Excludes isolated scans (those are reserved for specific workers)
	// 4. Respects per-scan concurrency limits
	// 5. Uses FOR UPDATE SKIP LOCKED for atomic claiming
	err := d.db.Raw(`
		UPDATE scan_jobs
		SET status = ?, worker_id = ?, claimed_at = ?
		WHERE id = (
			SELECT sj.id FROM scan_jobs sj
			JOIN scans s ON sj.scan_id = s.id
			WHERE sj.status = ?
			  AND s.status IN (?, ?)
			  AND s.isolated = false
			  AND (s.throttled_until IS NULL OR s.throttled_until < ?)
			  AND (
				s.max_concurrent_jobs IS NULL
				OR (SELECT COUNT(*) FROM scan_jobs
					WHERE scan_id = sj.scan_id AND status IN (?, ?)) < s.max_concurrent_jobs
			  )
			ORDER BY sj.priority DESC, sj.created_at ASC
			LIMIT 1
			FOR UPDATE OF sj SKIP LOCKED
		)
		RETURNING *
	`,
		ScanJobStatusClaimed, workerID, now,
		ScanJobStatusPending,
		ScanStatusCrawling, ScanStatusScanning,
		now,
		ScanJobStatusClaimed, ScanJobStatusRunning,
	).Scan(&job).Error

	if err != nil {
		return nil, err
	}

	if job.ID == 0 {
		return nil, nil // No job available
	}

	return &job, nil
}

// ClaimScanJobForScan atomically claims the next available job for a specific scan.
// This is used for isolated scanning where workers should only process jobs for one scan.
// Uses FOR UPDATE SKIP LOCKED for atomic claiming.
func (d *DatabaseConnection) ClaimScanJobForScan(workerID string, scanID uint) (*ScanJob, error) {
	var job ScanJob
	now := time.Now()

	// This query is similar to ClaimScanJob but filters to a specific scan_id
	err := d.db.Raw(`
		UPDATE scan_jobs 
		SET status = ?, worker_id = ?, claimed_at = ?
		WHERE id = (
			SELECT sj.id FROM scan_jobs sj
			JOIN scans s ON sj.scan_id = s.id
			WHERE sj.status = ?
			  AND sj.scan_id = ?
			  AND s.status IN (?, ?)
			  AND (s.throttled_until IS NULL OR s.throttled_until < ?)
			  AND (
				s.max_concurrent_jobs IS NULL
				OR (SELECT COUNT(*) FROM scan_jobs 
					WHERE scan_id = sj.scan_id AND status IN (?, ?)) < s.max_concurrent_jobs
			  )
			ORDER BY sj.priority DESC, sj.created_at ASC
			LIMIT 1
			FOR UPDATE OF sj SKIP LOCKED
		)
		RETURNING *
	`,
		ScanJobStatusClaimed, workerID, now,
		ScanJobStatusPending,
		scanID,
		ScanStatusCrawling, ScanStatusScanning,
		now,
		ScanJobStatusClaimed, ScanJobStatusRunning,
	).Scan(&job).Error

	if err != nil {
		return nil, err
	}

	if job.ID == 0 {
		return nil, nil // No job available for this scan
	}

	return &job, nil
}

// SetScanJobStatus updates a job's status
func (d *DatabaseConnection) SetScanJobStatus(jobID uint, status ScanJobStatus) error {
	updates := map[string]interface{}{
		"status": status,
	}

	if status == ScanJobStatusRunning {
		now := time.Now()
		updates["started_at"] = now
	}

	if status == ScanJobStatusCompleted || status == ScanJobStatusFailed || status == ScanJobStatusCancelled {
		now := time.Now()
		updates["completed_at"] = now
	}

	return d.db.Model(&ScanJob{}).Where("id = ?", jobID).Updates(updates).Error
}

// MarkScanJobRunning marks a job as running
func (d *DatabaseConnection) MarkScanJobRunning(jobID uint) error {
	now := time.Now()
	return d.db.Model(&ScanJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":     ScanJobStatusRunning,
		"started_at": now,
		"attempts":   gorm.Expr("attempts + 1"),
	}).Error
}

// MarkScanJobCompleted marks a job as completed
func (d *DatabaseConnection) MarkScanJobCompleted(jobID uint, issuesFound int) error {
	now := time.Now()
	return d.db.Model(&ScanJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":       ScanJobStatusCompleted,
		"completed_at": now,
		"issues_found": issuesFound,
	}).Error
}

// MarkScanJobFailed marks a job as failed
func (d *DatabaseConnection) MarkScanJobFailed(jobID uint, errorType, errorMsg string) error {
	now := time.Now()
	return d.db.Model(&ScanJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
		"status":        ScanJobStatusFailed,
		"completed_at":  now,
		"error_type":    errorType,
		"error_message": errorMsg,
	}).Error
}

// CancelScanJobs cancels jobs matching the filter
func (d *DatabaseConnection) CancelScanJobs(scanID uint, filter ScanJobFilter) (int64, error) {
	query := d.db.Model(&ScanJob{}).Where("scan_id = ?", scanID)

	// Only cancel pending or claimed jobs
	query = query.Where("status IN ?", []ScanJobStatus{ScanJobStatusPending, ScanJobStatusClaimed})

	if filter.TargetHost != "" {
		query = query.Where("target_host = ?", filter.TargetHost)
	}

	if len(filter.JobTypes) > 0 {
		query = query.Where("job_type IN ?", filter.JobTypes)
	}

	result := query.Update("status", ScanJobStatusCancelled)
	return result.RowsAffected, result.Error
}

// ResetStaleClaimedJobs resets jobs that were claimed but never completed
// (e.g., worker crashed). Called during startup recovery.
func (d *DatabaseConnection) ResetStaleClaimedJobs(staleThreshold time.Time) (int64, error) {
	result := d.db.Model(&ScanJob{}).
		Where("status = ? AND claimed_at < ?", ScanJobStatusClaimed, staleThreshold).
		Updates(map[string]interface{}{
			"status":     ScanJobStatusPending,
			"worker_id":  nil,
			"claimed_at": nil,
		})
	return result.RowsAffected, result.Error
}

// GetScanJobStats returns job statistics for a scan
func (d *DatabaseConnection) GetScanJobStats(scanID uint) (map[ScanJobStatus]int64, error) {
	var results []struct {
		Status ScanJobStatus
		Count  int64
	}

	err := d.db.Model(&ScanJob{}).
		Select("status, COUNT(*) as count").
		Where("scan_id = ?", scanID).
		Group("status").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	stats := make(map[ScanJobStatus]int64)
	for _, r := range results {
		stats[r.Status] = r.Count
	}

	return stats, nil
}

// ScanJobExists checks if a scan job exists
func (d *DatabaseConnection) ScanJobExists(id uint) (bool, error) {
	var count int64
	err := d.db.Model(&ScanJob{}).Where("id = ?", id).Count(&count).Error
	return count > 0, err
}

// GetPendingScanJobsCount returns the count of pending jobs for a scan
func (d *DatabaseConnection) GetPendingScanJobsCount(scanID uint) (int64, error) {
	var count int64
	err := d.db.Model(&ScanJob{}).
		Where("scan_id = ? AND status = ?", scanID, ScanJobStatusPending).
		Count(&count).Error
	return count, err
}

// ScanHasPendingJobs checks if a scan has pending or running jobs
func (d *DatabaseConnection) ScanHasPendingJobs(scanID uint) (bool, error) {
	var count int64
	err := d.db.Model(&ScanJob{}).
		Where("scan_id = ? AND status IN ?", scanID, []ScanJobStatus{ScanJobStatusPending, ScanJobStatusClaimed, ScanJobStatusRunning}).
		Count(&count).Error
	return count > 0, err
}

// UpdateScanJobCheckpoint updates the checkpoint for a job
func (d *DatabaseConnection) UpdateScanJobCheckpoint(jobID uint, checkpoint *ScanJobCheckpoint) error {
	return d.db.Model(&ScanJob{}).Where("id = ?", jobID).Update("checkpoint", checkpoint).Error
}

// ScanJobStatsResponse contains statistics for a specific scan job
type ScanJobStatsResponse struct {
	Requests int64       `json:"requests"`
	Issues   IssuesStats `json:"issues"`
	OOBTests int64       `json:"oob_tests"`
}

// GetScanJobStatsForJob retrieves issue and OOB test statistics for a specific scan job
func (d *DatabaseConnection) GetScanJobStatsForJob(scanJobID uint) (ScanJobStatsResponse, error) {
	var stats ScanJobStatsResponse

	// Get request count (history items) for this scan job
	if err := d.db.Model(&History{}).Where("scan_job_id = ?", scanJobID).Count(&stats.Requests).Error; err != nil {
		return stats, err
	}

	// Get issue counts by severity for this scan job
	issueCounts := map[severity]int64{}
	rows, err := d.db.Model(&Issue{}).
		Select("severity, COUNT(*) as count").
		Where("scan_job_id = ?", scanJobID).
		Group("severity").Rows()
	if err != nil {
		return stats, err
	}
	for rows.Next() {
		var sev severity
		var count int64
		rows.Scan(&sev, &count)
		issueCounts[sev] = count
	}
	rows.Close()

	stats.Issues = IssuesStats{
		Unknown:  issueCounts[Unknown],
		Info:     issueCounts[Info],
		Low:      issueCounts[Low],
		Medium:   issueCounts[Medium],
		High:     issueCounts[High],
		Critical: issueCounts[Critical],
	}

	// Get OOB tests count for this scan job
	if err := d.db.Model(&OOBTest{}).Where("scan_job_id = ?", scanJobID).Count(&stats.OOBTests).Error; err != nil {
		return stats, err
	}

	return stats, nil
}

// ResetTimedOutJobs marks jobs that have exceeded their max_duration as failed.
// Timed-out jobs are NOT retried
// Returns the number of jobs marked as failed and affected scan IDs.
func (d *DatabaseConnection) ResetTimedOutJobs() (failed int64, affectedScanIDs []uint, err error) {
	now := time.Now()

	// max_duration is stored in nanoseconds (Go's time.Duration), convert to seconds for PostgreSQL
	// PostgreSQL: started_at + (max_duration / 1000000000) * interval '1 second'
	timeoutCondition := "started_at + make_interval(secs => max_duration / 1000000000) < ?"

	// Get the scan IDs that will be affected (for updating counts later)
	var scanIDs []uint
	d.db.Model(&ScanJob{}).
		Select("DISTINCT scan_id").
		Where("status IN ? AND started_at IS NOT NULL",
			[]ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning}).
		Where(timeoutCondition, now).
		Pluck("scan_id", &scanIDs)
	affectedScanIDs = scanIDs

	// Mark ALL timed-out jobs as failed (no retry for timeouts)
	result := d.db.Model(&ScanJob{}).
		Where("status IN ? AND started_at IS NOT NULL",
			[]ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning}).
		Where(timeoutCondition, now).
		Updates(map[string]interface{}{
			"status":        ScanJobStatusFailed,
			"completed_at":  now,
			"error_type":    "timeout",
			"error_message": "Job exceeded maximum duration (timeouts are not retried)",
			"updated_at":    now,
		})

	if result.Error != nil {
		return 0, affectedScanIDs, fmt.Errorf("failed to mark timed out jobs as failed: %w", result.Error)
	}
	failed = result.RowsAffected

	if failed > 0 {
		log.Warn().
			Int64("count", failed).
			Msg("Timed-out jobs marked as failed (timeouts are not retried)")
	}

	return failed, affectedScanIDs, nil
}

// ReleaseJobsByWorker releases all jobs claimed by a specific worker back to pending status.
// This is called during graceful shutdown to ensure jobs are not stuck.
// Returns the count of released jobs and affected scan IDs.
func (d *DatabaseConnection) ReleaseJobsByWorker(workerID string) (int64, []uint, error) {
	now := time.Now()

	// Get the scan IDs that will be affected (for updating counts later)
	var affectedScanIDs []uint
	d.db.Model(&ScanJob{}).
		Select("DISTINCT scan_id").
		Where("worker_id = ? AND status IN ?",
			workerID,
			[]ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning}).
		Pluck("scan_id", &affectedScanIDs)

	result := d.db.Model(&ScanJob{}).
		Where("worker_id = ? AND status IN ?",
			workerID,
			[]ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning}).
		Updates(map[string]interface{}{
			"status":     ScanJobStatusPending,
			"worker_id":  nil,
			"claimed_at": nil,
			"started_at": nil,
			"updated_at": now,
		})

	if result.Error != nil {
		return 0, affectedScanIDs, fmt.Errorf("failed to release jobs for worker %s: %w", workerID, result.Error)
	}

	if result.RowsAffected > 0 {
		log.Info().
			Str("worker_id", workerID).
			Int64("count", result.RowsAffected).
			Msg("Released jobs during graceful shutdown")
	}

	return result.RowsAffected, affectedScanIDs, nil
}

// ReleaseJobsByWorkerNode releases all jobs claimed by workers belonging to a specific node.
// This is called during graceful shutdown of a worker pool.
// Returns the count of released jobs and affected scan IDs.
func (d *DatabaseConnection) ReleaseJobsByWorkerNode(nodeID string) (int64, []uint, error) {
	now := time.Now()

	// Worker IDs are prefixed with nodeID, e.g., "node-hostname-123-0", "node-hostname-123-1"
	// Get the scan IDs that will be affected (for updating counts later)
	var affectedScanIDs []uint
	d.db.Model(&ScanJob{}).
		Select("DISTINCT scan_id").
		Where("worker_id LIKE ? AND status IN ?",
			nodeID+"-%",
			[]ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning}).
		Pluck("scan_id", &affectedScanIDs)

	result := d.db.Model(&ScanJob{}).
		Where("worker_id LIKE ? AND status IN ?",
			nodeID+"-%",
			[]ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning}).
		Updates(map[string]interface{}{
			"status":     ScanJobStatusPending,
			"worker_id":  nil,
			"claimed_at": nil,
			"started_at": nil,
			"updated_at": now,
		})

	if result.Error != nil {
		return 0, affectedScanIDs, fmt.Errorf("failed to release jobs for worker node %s: %w", nodeID, result.Error)
	}

	if result.RowsAffected > 0 {
		log.Info().
			Str("node_id", nodeID).
			Int64("count", result.RowsAffected).
			Msg("Released jobs during worker node graceful shutdown")
	}

	return result.RowsAffected, affectedScanIDs, nil
}

// ThroughputStats holds job throughput metrics
type ThroughputStats struct {
	// Jobs completed in the last minute
	LastMinute int64 `json:"last_minute"`
	// Jobs completed in the last 5 minutes
	Last5Minutes int64 `json:"last_5_minutes"`
	// Jobs completed in the last hour
	LastHour int64 `json:"last_hour"`
	// Jobs per minute (calculated from last 5 minutes)
	JobsPerMinute float64 `json:"jobs_per_minute"`
	// Success rate (completed / (completed + failed)) over last hour
	SuccessRate float64 `json:"success_rate"`
	// Current pending queue depth
	QueueDepth int64 `json:"queue_depth"`
	// Jobs currently being processed
	InFlight int64 `json:"in_flight"`
}

// GetJobThroughputStats retrieves job throughput metrics
func (d *DatabaseConnection) GetJobThroughputStats() (*ThroughputStats, error) {
	stats := &ThroughputStats{}
	now := time.Now()

	// Jobs completed in last minute
	oneMinAgo := now.Add(-1 * time.Minute)
	d.db.Model(&ScanJob{}).
		Where("status = ? AND completed_at > ?", ScanJobStatusCompleted, oneMinAgo).
		Count(&stats.LastMinute)

	// Jobs completed in last 5 minutes
	fiveMinAgo := now.Add(-5 * time.Minute)
	d.db.Model(&ScanJob{}).
		Where("status = ? AND completed_at > ?", ScanJobStatusCompleted, fiveMinAgo).
		Count(&stats.Last5Minutes)

	// Jobs completed in last hour
	oneHourAgo := now.Add(-1 * time.Hour)
	d.db.Model(&ScanJob{}).
		Where("status = ? AND completed_at > ?", ScanJobStatusCompleted, oneHourAgo).
		Count(&stats.LastHour)

	// Calculate jobs per minute from last 5 minutes
	if stats.Last5Minutes > 0 {
		stats.JobsPerMinute = float64(stats.Last5Minutes) / 5.0
	}

	// Success rate over last hour
	var failedLastHour int64
	d.db.Model(&ScanJob{}).
		Where("status = ? AND completed_at > ?", ScanJobStatusFailed, oneHourAgo).
		Count(&failedLastHour)

	totalProcessed := stats.LastHour + failedLastHour
	if totalProcessed > 0 {
		stats.SuccessRate = float64(stats.LastHour) / float64(totalProcessed) * 100
	} else {
		stats.SuccessRate = 100 // No failures if nothing processed
	}

	// Current queue depth (pending jobs)
	d.db.Model(&ScanJob{}).
		Where("status = ?", ScanJobStatusPending).
		Count(&stats.QueueDepth)

	// In-flight jobs (claimed + running)
	d.db.Model(&ScanJob{}).
		Where("status IN ?", []ScanJobStatus{ScanJobStatusClaimed, ScanJobStatusRunning}).
		Count(&stats.InFlight)

	return stats, nil
}

// JobDurationStats holds job duration metrics for a specific job type
type JobDurationStats struct {
	JobType     string  `json:"job_type"`
	Count       int64   `json:"count"`
	AvgDuration float64 `json:"avg_duration_ms"`
	MinDuration float64 `json:"min_duration_ms"`
	MaxDuration float64 `json:"max_duration_ms"`
	P50Duration float64 `json:"p50_duration_ms"`
	P95Duration float64 `json:"p95_duration_ms"`
	P99Duration float64 `json:"p99_duration_ms"`
}

// GetJobDurationStats retrieves job duration statistics by job type
// Uses jobs completed in the last hour for relevant metrics
func (d *DatabaseConnection) GetJobDurationStats() ([]JobDurationStats, error) {
	oneHourAgo := time.Now().Add(-1 * time.Hour)

	// Query for basic stats (avg, min, max, count) grouped by job type
	var basicStats []struct {
		JobType     string
		Count       int64
		AvgDuration float64
		MinDuration float64
		MaxDuration float64
	}

	err := d.db.Model(&ScanJob{}).
		Select(`
			job_type,
			COUNT(*) as count,
			AVG(EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000) as avg_duration,
			MIN(EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000) as min_duration,
			MAX(EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000) as max_duration
		`).
		Where("status = ? AND completed_at > ? AND started_at IS NOT NULL", ScanJobStatusCompleted, oneHourAgo).
		Group("job_type").
		Scan(&basicStats).Error

	if err != nil {
		return nil, err
	}

	result := make([]JobDurationStats, 0, len(basicStats))

	for _, bs := range basicStats {
		stats := JobDurationStats{
			JobType:     bs.JobType,
			Count:       bs.Count,
			AvgDuration: bs.AvgDuration,
			MinDuration: bs.MinDuration,
			MaxDuration: bs.MaxDuration,
		}

		// Calculate percentiles using PostgreSQL percentile_cont
		var percentiles struct {
			P50 float64
			P95 float64
			P99 float64
		}

		d.db.Raw(`
			SELECT 
				COALESCE(percentile_cont(0.50) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000), 0) as p50,
				COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000), 0) as p95,
				COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY EXTRACT(EPOCH FROM (completed_at - started_at)) * 1000), 0) as p99
			FROM scan_jobs
			WHERE status = ? AND completed_at > ? AND started_at IS NOT NULL AND job_type = ?
		`, ScanJobStatusCompleted, oneHourAgo, bs.JobType).Scan(&percentiles)

		stats.P50Duration = percentiles.P50
		stats.P95Duration = percentiles.P95
		stats.P99Duration = percentiles.P99

		result = append(result, stats)
	}

	return result, nil
}
