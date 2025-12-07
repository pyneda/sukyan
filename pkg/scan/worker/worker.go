// Package worker provides the worker implementation for executing scan jobs.
package worker

import (
	"context"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/pyneda/sukyan/pkg/scan/executor"
	"github.com/pyneda/sukyan/pkg/scan/queue"
	"github.com/rs/zerolog/log"
)

const (
	// JobCancellationPollInterval is how often to check if a running job was cancelled
	JobCancellationPollInterval = 3 * time.Second
)

// Worker runs in a goroutine, polling for and executing jobs.
type Worker struct {
	id               string
	queue            queue.JobQueue
	registry         *control.Registry
	executorRegistry *executor.ExecutorRegistry
	pollInterval     time.Duration
	scanID           *uint // If set, worker only claims jobs for this scan (isolated mode)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Config holds worker configuration.
type Config struct {
	ID               string
	Queue            queue.JobQueue
	Registry         *control.Registry
	ExecutorRegistry *executor.ExecutorRegistry
	PollInterval     time.Duration
	ScanID           *uint // If set, worker only claims jobs for this scan (isolated mode)
}

// New creates a new worker.
func New(cfg Config) *Worker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 100 * time.Millisecond
	}
	if cfg.ExecutorRegistry == nil {
		cfg.ExecutorRegistry = executor.DefaultRegistry
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Worker{
		id:               cfg.ID,
		queue:            cfg.Queue,
		registry:         cfg.Registry,
		executorRegistry: cfg.ExecutorRegistry,
		pollInterval:     cfg.PollInterval,
		scanID:           cfg.ScanID,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start begins the worker's main loop.
func (w *Worker) Start() {
	w.wg.Add(1)
	go w.run()
	log.Info().Str("worker_id", w.id).Msg("Worker started")
}

// Stop gracefully stops the worker.
func (w *Worker) Stop() {
	log.Info().Str("worker_id", w.id).Msg("Worker stopping")
	w.cancel()
	w.wg.Wait()
	log.Info().Str("worker_id", w.id).Msg("Worker stopped")
}

// SetScanID sets the scan ID filter for isolated mode.
// When set, the worker will only claim jobs for this specific scan.
func (w *Worker) SetScanID(scanID uint) {
	w.scanID = &scanID
}

// ID returns the worker's ID.
func (w *Worker) ID() string {
	return w.id
}

func (w *Worker) run() {
	defer w.wg.Done()

	log.Debug().Str("worker_id", w.id).Msg("Worker run loop started")

	for {
		select {
		case <-w.ctx.Done():
			log.Debug().Str("worker_id", w.id).Msg("Worker context cancelled, exiting")
			return
		default:
		}

		// Try to claim a job
		var job *db.ScanJob
		var err error
		if w.scanID != nil {
			// Isolated mode: only claim jobs for this specific scan
			job, err = w.queue.ClaimForScan(w.ctx, w.id, *w.scanID)
		} else {
			// Distributed mode: claim any available job
			job, err = w.queue.Claim(w.ctx, w.id)
		}
		if err != nil {
			if w.ctx.Err() != nil {
				return // Context cancelled
			}
			log.Error().Err(err).Str("worker_id", w.id).Msg("Error claiming job")
			w.sleep()
			continue
		}

		if job == nil {
			// No jobs available, sleep and retry
			w.sleep()
			continue
		}

		// Execute the job
		w.executeJob(job)
	}
}

func (w *Worker) sleep() {
	select {
	case <-w.ctx.Done():
	case <-time.After(w.pollInterval):
	}
}

func (w *Worker) executeJob(job *db.ScanJob) {
	log := log.With().
		Str("worker_id", w.id).
		Uint("job_id", job.ID).
		Uint("scan_id", job.ScanID).
		Str("job_type", string(job.JobType)).
		Logger()

	log.Debug().Msg("Executing job")

	// Get or create scan control for checkpoint operations
	// In distributed mode, the control may not exist locally yet
	ctrl := w.registry.GetOrCreate(job.ScanID)

	// Check if scan is cancelled before starting
	if ctrl.IsCancelled() {
		log.Debug().Msg("Scan is cancelled, skipping job")
		_ = w.queue.Cancel(w.ctx, job.ID)
		return
	}

	// Check if this specific job was cancelled before starting
	if w.isJobCancelled(job.ID) {
		log.Debug().Msg("Job is cancelled, skipping")
		return
	}

	// Get the executor for this job type
	jobExecutor, ok := w.executorRegistry.Get(job.JobType)
	if !ok {
		log.Warn().Str("job_type", string(job.JobType)).Msg("No executor registered for job type")
		_ = w.queue.Fail(w.ctx, job.ID, "no_executor", "No executor registered for job type: "+string(job.JobType))
		return
	}

	// Mark job as running
	if err := db.Connection().MarkScanJobRunning(job.ID); err != nil {
		log.Error().Err(err).Msg("Failed to mark job as running")
		return
	}

	// Create job context that combines worker context and scan control context
	jobCtx, jobCancel := context.WithCancel(w.ctx)
	defer jobCancel()

	// Watch scan control context for cancellation
	go func() {
		select {
		case <-jobCtx.Done():
			return
		case <-ctrl.Context().Done():
			jobCancel()
		}
	}()

	// Watch for individual job cancellation (polls DB)
	go func() {
		ticker := time.NewTicker(JobCancellationPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				if w.isJobCancelled(job.ID) {
					log.Debug().Msg("Job cancelled via DB poll, cancelling context")
					jobCancel()
					return
				}
			}
		}
	}()

	startTime := time.Now()

	// Execute the job
	execErr := jobExecutor.Execute(jobCtx, job, ctrl)

	duration := time.Since(startTime)

	// Check if we were cancelled during execution (scan-level or job-level)
	if ctrl.IsCancelled() || jobCtx.Err() != nil || w.isJobCancelled(job.ID) {
		log.Debug().Dur("duration", duration).Msg("Job cancelled during execution")
		_ = w.queue.Cancel(w.ctx, job.ID)
		return
	}

	// Handle result
	if execErr != nil {
		errorType := "execution_error"
		if jobCtx.Err() != nil {
			errorType = "context_cancelled"
		}
		log.Warn().Err(execErr).Dur("duration", duration).Msg("Job failed")
		_ = w.queue.Fail(w.ctx, job.ID, errorType, execErr.Error())
	} else {
		log.Info().
			Dur("duration", duration).
			Msg("Job completed")
		_ = w.queue.Complete(w.ctx, job.ID, queue.JobResult{})
	}
}

// isJobCancelled checks if a specific job has been cancelled in the database
func (w *Worker) isJobCancelled(jobID uint) bool {
	job, err := db.Connection().GetScanJobByID(jobID)
	if err != nil {
		log.Warn().Err(err).Uint("job_id", jobID).Msg("Failed to check job cancellation status")
		return false
	}
	return job.Status == db.ScanJobStatusCancelled
}
