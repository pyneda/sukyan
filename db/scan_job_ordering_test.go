package db

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOrderingTest(t *testing.T) (*Scan, func()) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceScanJobOrdering",
		Code:        "test-workspace-job-ordering-" + uuid.New().String(),
		Description: "Test workspace for scan job relevance ordering",
	})
	require.NoError(t, err)

	scan := &Scan{
		Title:       "job-ordering-" + uuid.New().String(),
		WorkspaceID: workspace.ID,
		Status:      ScanStatusScanning,
		Phase:       ScanPhaseActiveScan,
	}
	require.NoError(t, Connection().DB().Create(scan).Error)

	return scan, func() {
		Connection().DB().Where("scan_id = ?", scan.ID).Delete(&ScanJob{})
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&Scan{})
		Connection().DeleteWorkspace(workspace.ID)
	}
}

func createOrderingJob(t *testing.T, job *ScanJob) uint {
	t.Helper()
	if job.JobType == "" {
		job.JobType = ScanJobTypeActiveScan
	}
	require.NoError(t, Connection().DB().Create(job).Error)
	return job.ID
}

func TestListScanJobsRelevanceOrdersByAttention(t *testing.T) {
	scan, cleanup := setupOrderingTest(t)
	defer cleanup()

	now := time.Now()
	older := now.Add(-10 * time.Minute)
	recent := now.Add(-1 * time.Minute)
	middle := now.Add(-5 * time.Minute)

	runningOld := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusRunning, StartedAt: &older})
	runningNew := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusRunning, StartedAt: &recent})
	claimed := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusClaimed, ClaimedAt: &recent})
	failedOld := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusFailed, CompletedAt: &middle})
	failedNew := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusFailed, CompletedAt: &recent})
	completedWithIssues := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusCompleted, CompletedAt: &middle, IssuesFound: 3})
	completedClean := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusCompleted, CompletedAt: &recent})
	cancelled := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusCancelled})
	pendingLow := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusPending, Priority: 5})
	pendingHigh := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusPending, Priority: 10})

	jobs, count, err := Connection().ListScanJobs(ScanJobFilter{
		ScanID:     scan.ID,
		SortBy:     "relevance",
		Pagination: Pagination{Page: 1, PageSize: 50},
	})
	require.NoError(t, err)
	assert.Equal(t, int64(10), count)

	got := make([]uint, 0, len(jobs))
	for _, job := range jobs {
		got = append(got, job.ID)
	}

	want := []uint{
		runningOld,          // running, longest-running first
		runningNew,          // running
		claimed,             // claimed
		failedNew,           // failed, most recent first
		failedOld,           // failed
		completedWithIssues, // completed with findings
		completedClean,      // completed clean
		cancelled,           // cancelled
		pendingHigh,         // pending, priority desc
		pendingLow,          // pending
	}
	assert.Equal(t, want, got)
}

// Regression test for issues_found DESC being unguarded: it sat ahead of the
// guarded completed_at key, so a failed job with issues_found > 0 would
// displace the "most recent first" ordering even though only completed jobs
// are meant to rank by issues_found.
func TestListScanJobsRelevanceIgnoresIssuesFoundOutsideCompletedBucket(t *testing.T) {
	scan, cleanup := setupOrderingTest(t)
	defer cleanup()

	now := time.Now()
	older := now.Add(-10 * time.Minute)
	recent := now.Add(-1 * time.Minute)

	failedOldWithIssues := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusFailed, CompletedAt: &older, IssuesFound: 5})
	failedNewClean := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusFailed, CompletedAt: &recent})

	jobs, _, err := Connection().ListScanJobs(ScanJobFilter{
		ScanID:     scan.ID,
		SortBy:     "relevance",
		Pagination: Pagination{Page: 1, PageSize: 50},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, failedNewClean, jobs[0].ID, "most recent failed job must sort first regardless of issues_found")
	assert.Equal(t, failedOldWithIssues, jobs[1].ID)
}

func TestListScanJobsUnknownSortFallsBackToQueueOrder(t *testing.T) {
	scan, cleanup := setupOrderingTest(t)
	defer cleanup()

	low := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusPending, Priority: 1})
	high := createOrderingJob(t, &ScanJob{ScanID: scan.ID, Status: ScanJobStatusPending, Priority: 9})

	jobs, _, err := Connection().ListScanJobs(ScanJobFilter{
		ScanID:     scan.ID,
		SortBy:     "not_a_column",
		Pagination: Pagination{Page: 1, PageSize: 50},
	})
	require.NoError(t, err)
	require.Len(t, jobs, 2)
	assert.Equal(t, high, jobs[0].ID)
	assert.Equal(t, low, jobs[1].ID)
}
