package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/scan/control"
	"github.com/pyneda/sukyan/pkg/scan/executor"
	"github.com/pyneda/sukyan/pkg/scan/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockingExecutor runs until its context is cancelled, standing in for a job
// that overruns its budget.
type blockingExecutor struct {
	jobType  db.ScanJobType
	started  chan struct{}
	returned chan error
}

func (e *blockingExecutor) Execute(ctx context.Context, job *db.ScanJob, ctrl *control.ScanControl) error {
	close(e.started)
	<-ctx.Done()
	err := ctx.Err()
	e.returned <- err
	return err
}

func (e *blockingExecutor) JobType() db.ScanJobType { return e.jobType }

type recordedFailure struct {
	jobID     uint
	errorType string
	message   string
}

// fakeQueue records terminal transitions so the test can assert how the worker
// classified the job.
type fakeQueue struct {
	failures  []recordedFailure
	cancelled []uint
	completed []uint
}

func (q *fakeQueue) Claim(context.Context, string) (*db.ScanJob, error) { return nil, nil }
func (q *fakeQueue) ClaimForScan(context.Context, string, uint) (*db.ScanJob, error) {
	return nil, nil
}
func (q *fakeQueue) Complete(_ context.Context, jobID uint, _ queue.JobResult) error {
	q.completed = append(q.completed, jobID)
	return nil
}
func (q *fakeQueue) Fail(_ context.Context, jobID uint, errorType, errorMsg string) error {
	q.failures = append(q.failures, recordedFailure{jobID, errorType, errorMsg})
	return nil
}
func (q *fakeQueue) Cancel(_ context.Context, jobID uint) error {
	q.cancelled = append(q.cancelled, jobID)
	return nil
}
func (q *fakeQueue) Enqueue(context.Context, *db.ScanJob) error        { return nil }
func (q *fakeQueue) EnqueueBatch(context.Context, []*db.ScanJob) error { return nil }
func (q *fakeQueue) Stats(context.Context, uint) (*queue.QueueStats, error) {
	return &queue.QueueStats{}, nil
}
func (q *fakeQueue) ResetStaleJobs(context.Context, string) (int64, error) { return 0, nil }

func setupJobFixture(t *testing.T, maxDuration time.Duration) (*db.ScanJob, func()) {
	t.Helper()

	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title:       "TestWorkspaceWorkerTimeout",
		Code:        "test-workspace-worker-timeout-" + uuid.New().String(),
		Description: "Test workspace for worker job timeout",
	})
	require.NoError(t, err)

	scan := &db.Scan{
		Title:       "worker-timeout-" + uuid.New().String(),
		WorkspaceID: workspace.ID,
		Status:      db.ScanStatusScanning,
		Phase:       db.ScanPhaseActiveScan,
		Isolated:    true,
	}
	require.NoError(t, db.Connection().DB().Create(scan).Error)

	job := &db.ScanJob{
		ScanID:      scan.ID,
		WorkspaceID: workspace.ID,
		JobType:     db.ScanJobTypeActiveScan,
		Status:      db.ScanJobStatusClaimed,
		MaxDuration: maxDuration,
	}
	created, err := db.Connection().CreateScanJob(job)
	require.NoError(t, err)
	job = created

	return job, func() {
		db.Connection().DB().Where("scan_id = ?", scan.ID).Delete(&db.ScanJob{})
		db.Connection().DB().Delete(scan)
		db.Connection().DeleteWorkspace(workspace.ID)
	}
}

func newTestWorker(q queue.JobQueue, exec executor.JobExecutor) *Worker {
	registry := executor.NewExecutorRegistry()
	registry.Register(exec)
	return New(Config{
		ID:               "test-worker",
		Queue:            q,
		Registry:         control.NewRegistry(db.Connection()),
		ExecutorRegistry: registry,
	})
}

// Without a deadline bound to MaxDuration the executor runs forever and its
// worker never returns to the pool, which silently stalls the whole scan.
func TestExecuteJob_StopsExecutorWhenMaxDurationExceeded(t *testing.T) {
	job, cleanup := setupJobFixture(t, 300*time.Millisecond)
	defer cleanup()

	exec := &blockingExecutor{
		jobType:  db.ScanJobTypeActiveScan,
		started:  make(chan struct{}),
		returned: make(chan error, 1),
	}
	q := &fakeQueue{}
	w := newTestWorker(q, exec)

	done := make(chan struct{})
	go func() {
		w.executeJob(job)
		close(done)
	}()

	select {
	case <-exec.started:
	case <-time.After(5 * time.Second):
		t.Fatal("executor never started")
	}

	select {
	case err := <-exec.returned:
		assert.ErrorIs(t, err, context.DeadlineExceeded, "executor must be stopped by the job deadline")
	case <-time.After(5 * time.Second):
		t.Fatal("executor was not cancelled when MaxDuration elapsed")
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("executeJob did not return after the deadline")
	}

	require.Len(t, q.failures, 1, "a timed-out job must be failed, not cancelled or completed")
	assert.Equal(t, job.ID, q.failures[0].jobID)
	assert.Equal(t, "timeout", q.failures[0].errorType)
	assert.Empty(t, q.cancelled)
	assert.Empty(t, q.completed)
}

// A job the queue already wrote off as failed must not keep its worker busy.
func TestIsJobNoLongerOurs_TreatsFailedLikeCancelled(t *testing.T) {
	job, cleanup := setupJobFixture(t, time.Minute)
	defer cleanup()

	w := newTestWorker(&fakeQueue{}, &blockingExecutor{jobType: db.ScanJobTypeActiveScan})

	assert.False(t, w.isJobNoLongerOurs(job.ID), "a claimed job is still ours")

	require.NoError(t, db.Connection().DB().Model(&db.ScanJob{}).
		Where("id = ?", job.ID).Update("status", db.ScanJobStatusFailed).Error)
	assert.True(t, w.isJobNoLongerOurs(job.ID), "a failed job has been written off by the queue")

	require.NoError(t, db.Connection().DB().Model(&db.ScanJob{}).
		Where("id = ?", job.ID).Update("status", db.ScanJobStatusCancelled).Error)
	assert.True(t, w.isJobNoLongerOurs(job.ID))
}
