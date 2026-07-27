package db

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupJobCountsTest(t *testing.T) (*Scan, func()) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceScanJobCounts",
		Code:        "test-workspace-job-counts-" + uuid.New().String(),
		Description: "Test workspace for scan job count refresh",
	})
	require.NoError(t, err)

	scan := &Scan{
		Title:       "job-counts-" + uuid.New().String(),
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

func createJobsForScan(t *testing.T, scanID uint, status ScanJobStatus, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		job := &ScanJob{ScanID: scanID, Status: status, JobType: ScanJobTypeActiveScan}
		require.NoError(t, Connection().DB().Create(job).Error)
	}
}

// RefreshScanJobCounts must update the caller's struct as well as the row. UpdateScan
// persists the whole struct, so a caller that recounts and then saves would otherwise
// write the stale in-memory counts straight back over the fresh ones.
func TestRefreshScanJobCountsUpdatesTheStructAndTheRow(t *testing.T) {
	scan, cleanup := setupJobCountsTest(t)
	defer cleanup()

	createJobsForScan(t, scan.ID, ScanJobStatusCompleted, 5)
	createJobsForScan(t, scan.ID, ScanJobStatusPending, 2)
	createJobsForScan(t, scan.ID, ScanJobStatusFailed, 1)

	require.NoError(t, Connection().RefreshScanJobCounts(scan))

	assert.Equal(t, 5, scan.CompletedJobsCount, "in-memory completed count")
	assert.Equal(t, 2, scan.PendingJobsCount, "in-memory pending count")
	assert.Equal(t, 1, scan.FailedJobsCount, "in-memory failed count")
	assert.Equal(t, 8, scan.TotalJobsCount, "in-memory total count")

	stored, err := Connection().GetScanByID(scan.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, stored.CompletedJobsCount, "persisted completed count")
	assert.Equal(t, 2, stored.PendingJobsCount, "persisted pending count")
}

// The defect this guards: a scan whose terminal status is published by UpdateScan while
// the struct still holds early counts reports e.g. 7 completed / 82 pending forever, so
// anything watching for completion reads a truncated-looking scan.
func TestRefreshScanJobCountsSurvivesASubsequentUpdateScan(t *testing.T) {
	scan, cleanup := setupJobCountsTest(t)
	defer cleanup()

	createJobsForScan(t, scan.ID, ScanJobStatusPending, 9)
	require.NoError(t, Connection().RefreshScanJobCounts(scan))

	// Every job finishes, then the scan is marked complete the way completeScan does it.
	require.NoError(t, Connection().DB().Model(&ScanJob{}).Where("scan_id = ?", scan.ID).
		Update("status", ScanJobStatusCompleted).Error)
	require.NoError(t, Connection().RefreshScanJobCounts(scan))
	scan.Status = ScanStatusCompleted
	_, err := Connection().UpdateScan(scan)
	require.NoError(t, err)

	stored, err := Connection().GetScanByID(scan.ID)
	require.NoError(t, err)
	assert.Equal(t, ScanStatusCompleted, stored.Status)
	assert.Equal(t, 9, stored.CompletedJobsCount, "completed count must be published with the terminal status")
	assert.Equal(t, 0, stored.PendingJobsCount, "no jobs may look pending once the scan is complete")
}
