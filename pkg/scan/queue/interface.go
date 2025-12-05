// Package queue provides job queue interfaces and implementations for the scan engine.
package queue

import (
	"context"

	"github.com/pyneda/sukyan/db"
)

// JobResult contains the result of a job execution
type JobResult struct {
	IssuesFound int
	HTTPStatus  *int
	ErrorType   string
	ErrorMsg    string
}

// QueueStats contains statistics about the job queue for a scan
type QueueStats struct {
	PendingCount   int64
	ClaimedCount   int64
	RunningCount   int64
	CompletedCount int64
	FailedCount    int64
	CancelledCount int64
	TotalCount     int64
}

// JobQueue defines the interface for job queue operations.
// Implementations must be safe for concurrent use.
type JobQueue interface {
	// Claim atomically claims and returns the next available job.
	// Returns nil if no job is available.
	// The job status will be set to "claimed" with the worker ID.
	Claim(ctx context.Context, workerID string) (*db.ScanJob, error)

	// Complete marks a job as successfully completed.
	Complete(ctx context.Context, jobID uint, result JobResult) error

	// Fail marks a job as failed with error information.
	// The job may be retried if attempts < maxAttempts.
	Fail(ctx context.Context, jobID uint, errorType, errorMsg string) error

	// Cancel cancels a job (must be pending or claimed).
	Cancel(ctx context.Context, jobID uint) error

	// Enqueue adds a new job to the queue.
	Enqueue(ctx context.Context, job *db.ScanJob) error

	// EnqueueBatch adds multiple jobs to the queue.
	EnqueueBatch(ctx context.Context, jobs []*db.ScanJob) error

	// Stats returns queue statistics for a scan.
	Stats(ctx context.Context, scanID uint) (*QueueStats, error)

	// ResetStaleJobs resets jobs that were claimed but never completed.
	// This is used for recovery after worker crashes.
	ResetStaleJobs(ctx context.Context, workerID string) (int64, error)
}

// JobFilter defines criteria for filtering jobs
type JobFilter struct {
	ScanID     uint
	Statuses   []db.ScanJobStatus
	JobTypes   []db.ScanJobType
	TargetHost string
	URLPattern string
}
