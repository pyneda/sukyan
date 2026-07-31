package db

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupJobStatsTest(t *testing.T) (*Scan, func()) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceScanJobStats",
		Code:        "test-workspace-job-stats-" + uuid.New().String(),
		Description: "Test workspace for batched scan job stats",
	})
	require.NoError(t, err)

	scan := &Scan{
		Title:       "job-stats-" + uuid.New().String(),
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

// The batched path replaces a per-row loop, so its only contract is that it
// agrees with the single-job function it replaced.
func TestGetScanJobStatsForJobsMatchesThePerJobPath(t *testing.T) {
	scan, cleanup := setupJobStatsTest(t)
	defer cleanup()

	withData := &ScanJob{ScanID: scan.ID, WorkspaceID: scan.WorkspaceID, Status: ScanJobStatusCompleted, JobType: ScanJobTypeActiveScan}
	require.NoError(t, Connection().DB().Create(withData).Error)

	empty := &ScanJob{ScanID: scan.ID, WorkspaceID: scan.WorkspaceID, Status: ScanJobStatusPending, JobType: ScanJobTypeActiveScan}
	require.NoError(t, Connection().DB().Create(empty).Error)

	for i := 0; i < 3; i++ {
		require.NoError(t, Connection().DB().Create(&History{
			WorkspaceID: &scan.WorkspaceID,
			ScanJobID:   &withData.ID,
			URL:         "https://example.com/stats-" + uuid.New().String(),
			Method:      "GET",
		}).Error)
	}

	for _, sev := range []severity{High, High, Medium} {
		require.NoError(t, Connection().DB().Create(&Issue{
			WorkspaceID: &scan.WorkspaceID,
			ScanJobID:   &withData.ID,
			Title:       "stats-issue-" + uuid.New().String(),
			Severity:    sev,
		}).Error)
	}

	batched, err := Connection().GetScanJobStatsForJobs([]uint{withData.ID, empty.ID})
	require.NoError(t, err)
	require.Len(t, batched, 2)

	single, err := Connection().GetScanJobStatsForJob(withData.ID)
	require.NoError(t, err)
	assert.Equal(t, single, batched[withData.ID], "batched stats must match the per-job path")

	assert.Equal(t, int64(3), batched[withData.ID].Requests)
	assert.Equal(t, int64(2), batched[withData.ID].Issues.High)
	assert.Equal(t, int64(1), batched[withData.ID].Issues.Medium)

	assert.Equal(t, ScanJobStatsResponse{}, batched[empty.ID], "a job with no rows still gets a zero-valued entry")
}

func TestGetScanJobStatsForJobsHandlesAnEmptyPage(t *testing.T) {
	stats, err := Connection().GetScanJobStatsForJobs(nil)
	require.NoError(t, err)
	assert.Empty(t, stats)
}
