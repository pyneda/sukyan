package executor

import (
	"context"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/control"
)

// JobExecutor defines the interface for executing different types of scan jobs.
// Implementations should respect the ScanControl for pause/resume/cancel operations.
type JobExecutor interface {
	// Execute runs the scan job and returns any error encountered.
	// The implementation should:
	// - Periodically call ctrl.Checkpoint() to allow pause/resume
	// - Check for cancellation via ctx.Done() or ctrl.State() == control.StateCancelled
	// - Update job progress if applicable
	Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error

	// JobType returns the type of jobs this executor handles
	JobType() db.ScanJobType
}

// ExecutorRegistry maps job types to their executors
type ExecutorRegistry struct {
	executors map[db.ScanJobType]JobExecutor
}

// NewExecutorRegistry creates a new executor registry
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make(map[db.ScanJobType]JobExecutor),
	}
}

// Register adds an executor for a specific job type
func (r *ExecutorRegistry) Register(executor JobExecutor) {
	r.executors[executor.JobType()] = executor
}

// Get retrieves the executor for a job type
func (r *ExecutorRegistry) Get(jobType db.ScanJobType) (JobExecutor, bool) {
	executor, ok := r.executors[jobType]
	return executor, ok
}

// DefaultRegistry is the global executor registry
var DefaultRegistry = NewExecutorRegistry()

// RegisterExecutor registers an executor in the default registry
func RegisterExecutor(executor JobExecutor) {
	DefaultRegistry.Register(executor)
}

// GetExecutor retrieves an executor from the default registry
func GetExecutor(jobType db.ScanJobType) (JobExecutor, bool) {
	return DefaultRegistry.Get(jobType)
}
