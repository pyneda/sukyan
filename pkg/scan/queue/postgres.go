package queue

import (
	"context"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// PostgresQueue implements JobQueue using PostgreSQL with FOR UPDATE SKIP LOCKED
type PostgresQueue struct {
	dbConn *db.DatabaseConnection
}

// NewPostgresQueue creates a new PostgreSQL-backed job queue
func NewPostgresQueue(dbConn *db.DatabaseConnection) *PostgresQueue {
	return &PostgresQueue{dbConn: dbConn}
}

// Claim atomically claims the next available job for a worker.
// Uses FOR UPDATE SKIP LOCKED for atomic claiming without blocking.
func (q *PostgresQueue) Claim(ctx context.Context, workerID string) (*db.ScanJob, error) {
	// Check context before attempting to claim
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	job, err := q.dbConn.ClaimScanJob(workerID)
	if err != nil {
		log.Error().Err(err).Str("worker_id", workerID).Msg("Failed to claim job")
		return nil, err
	}

	if job != nil {
		log.Debug().
			Uint("job_id", job.ID).
			Uint("scan_id", job.ScanID).
			Str("worker_id", workerID).
			Str("job_type", string(job.JobType)).
			Msg("Job claimed")
	}

	return job, nil
}

// Complete marks a job as successfully completed
func (q *PostgresQueue) Complete(ctx context.Context, jobID uint, result JobResult) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := q.dbConn.MarkScanJobCompleted(jobID, result.IssuesFound)
	if err != nil {
		log.Error().Err(err).Uint("job_id", jobID).Msg("Failed to mark job as completed")
		return err
	}

	// Update HTTP status if provided
	if result.HTTPStatus != nil {
		q.dbConn.DB().Model(&db.ScanJob{}).Where("id = ?", jobID).Update("http_status", result.HTTPStatus)
	}

	log.Debug().
		Uint("job_id", jobID).
		Int("issues_found", result.IssuesFound).
		Msg("Job completed")

	return nil
}

// Fail marks a job as failed
func (q *PostgresQueue) Fail(ctx context.Context, jobID uint, errorType, errorMsg string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// First check if the job can be retried
	job, err := q.dbConn.GetScanJobByID(jobID)
	if err != nil {
		return err
	}

	if job.CanRetry() {
		// Reset to pending for retry
		now := time.Now()
		return q.dbConn.DB().Model(&db.ScanJob{}).Where("id = ?", jobID).Updates(map[string]interface{}{
			"status":        db.ScanJobStatusPending,
			"worker_id":     nil,
			"claimed_at":    nil,
			"error_type":    errorType,
			"error_message": errorMsg,
			"completed_at":  now,
		}).Error
	}

	// No more retries, mark as failed
	err = q.dbConn.MarkScanJobFailed(jobID, errorType, errorMsg)
	if err != nil {
		log.Error().Err(err).Uint("job_id", jobID).Msg("Failed to mark job as failed")
		return err
	}

	log.Warn().
		Uint("job_id", jobID).
		Str("error_type", errorType).
		Str("error_msg", errorMsg).
		Msg("Job failed")

	return nil
}

// Cancel cancels a job
func (q *PostgresQueue) Cancel(ctx context.Context, jobID uint) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	err := q.dbConn.SetScanJobStatus(jobID, db.ScanJobStatusCancelled)
	if err != nil {
		log.Error().Err(err).Uint("job_id", jobID).Msg("Failed to cancel job")
		return err
	}

	log.Debug().Uint("job_id", jobID).Msg("Job cancelled")
	return nil
}

// Enqueue adds a new job to the queue
func (q *PostgresQueue) Enqueue(ctx context.Context, job *db.ScanJob) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	_, err := q.dbConn.CreateScanJob(job)
	if err != nil {
		log.Error().Err(err).Interface("job", job).Msg("Failed to enqueue job")
		return err
	}

	log.Debug().
		Uint("job_id", job.ID).
		Uint("scan_id", job.ScanID).
		Str("job_type", string(job.JobType)).
		Msg("Job enqueued")

	return nil
}

// EnqueueBatch adds multiple jobs to the queue
func (q *PostgresQueue) EnqueueBatch(ctx context.Context, jobs []*db.ScanJob) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if len(jobs) == 0 {
		return nil
	}

	err := q.dbConn.CreateScanJobs(jobs)
	if err != nil {
		log.Error().Err(err).Int("count", len(jobs)).Msg("Failed to enqueue batch of jobs")
		return err
	}

	log.Debug().
		Int("count", len(jobs)).
		Uint("scan_id", jobs[0].ScanID).
		Msg("Jobs enqueued in batch")

	return nil
}

// Stats returns queue statistics for a scan
func (q *PostgresQueue) Stats(ctx context.Context, scanID uint) (*QueueStats, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	statusCounts, err := q.dbConn.GetScanJobStats(scanID)
	if err != nil {
		return nil, err
	}

	stats := &QueueStats{
		PendingCount:   statusCounts[db.ScanJobStatusPending],
		ClaimedCount:   statusCounts[db.ScanJobStatusClaimed],
		RunningCount:   statusCounts[db.ScanJobStatusRunning],
		CompletedCount: statusCounts[db.ScanJobStatusCompleted],
		FailedCount:    statusCounts[db.ScanJobStatusFailed],
		CancelledCount: statusCounts[db.ScanJobStatusCancelled],
	}

	stats.TotalCount = stats.PendingCount + stats.ClaimedCount + stats.RunningCount +
		stats.CompletedCount + stats.FailedCount + stats.CancelledCount

	return stats, nil
}

// ResetStaleJobs resets jobs that were claimed by a specific worker but never completed.
// This is used for recovery when a worker crashes or restarts.
func (q *PostgresQueue) ResetStaleJobs(ctx context.Context, workerID string) (int64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	result := q.dbConn.DB().Model(&db.ScanJob{}).
		Where("status = ? AND worker_id = ?", db.ScanJobStatusClaimed, workerID).
		Updates(map[string]interface{}{
			"status":     db.ScanJobStatusPending,
			"worker_id":  nil,
			"claimed_at": nil,
		})

	if result.Error != nil {
		log.Error().Err(result.Error).Str("worker_id", workerID).Msg("Failed to reset stale jobs")
		return 0, result.Error
	}

	if result.RowsAffected > 0 {
		log.Info().
			Int64("count", result.RowsAffected).
			Str("worker_id", workerID).
			Msg("Reset stale jobs")
	}

	return result.RowsAffected, nil
}

// ResetAllStaleJobs resets all jobs that have been claimed for longer than the threshold.
// This is used during startup recovery.
func (q *PostgresQueue) ResetAllStaleJobs(ctx context.Context, staleThreshold time.Duration) (int64, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	threshold := time.Now().Add(-staleThreshold)
	count, err := q.dbConn.ResetStaleClaimedJobs(threshold)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reset stale jobs during recovery")
		return 0, err
	}

	if count > 0 {
		log.Info().Int64("count", count).Msg("Reset stale jobs during recovery")
	}

	return count, nil
}

// UpdateScanJobCounts updates the job counts for a scan.
// This should be called periodically to keep progress tracking accurate.
func (q *PostgresQueue) UpdateScanJobCounts(ctx context.Context, scanID uint) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return q.dbConn.UpdateScanJobCounts(scanID)
}
