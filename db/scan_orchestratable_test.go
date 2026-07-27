package db

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupOrchestratableScanTest(t *testing.T) (*Workspace, func()) {
	workspace, err := Connection().GetOrCreateWorkspace(&Workspace{
		Title:       "TestWorkspaceOrchestratableScans",
		Code:        "test-workspace-orchestratable-" + uuid.New().String(),
		Description: "Test workspace for orchestratable scan queries",
	})
	require.NoError(t, err)

	return workspace, func() {
		Connection().DB().Where("workspace_id = ?", workspace.ID).Delete(&Scan{})
		Connection().DeleteWorkspace(workspace.ID)
	}
}

func createScanForTest(t *testing.T, workspaceID uint, status ScanStatus, isolated bool) *Scan {
	scan := &Scan{
		Title:       "orchestratable-" + uuid.New().String(),
		WorkspaceID: workspaceID,
		Status:      status,
		Phase:       ScanPhaseCrawl,
		Isolated:    isolated,
	}
	require.NoError(t, Connection().DB().Create(scan).Error)
	return scan
}

func scanIDs(scans []*Scan) map[uint]bool {
	ids := make(map[uint]bool, len(scans))
	for _, s := range scans {
		ids[s.ID] = true
	}
	return ids
}

// An isolated scan is driven by the process that created it. A shared
// orchestrator picking it up would schedule jobs no worker of its own will claim
// and would run synchronous phases on another process's targets.
func TestGetOrchestratableScans_SharedOrchestratorSkipsIsolatedScans(t *testing.T) {
	workspace, cleanup := setupOrchestratableScanTest(t)
	defer cleanup()

	isolated := createScanForTest(t, workspace.ID, ScanStatusScanning, true)
	shared := createScanForTest(t, workspace.ID, ScanStatusScanning, false)

	scans, err := Connection().GetOrchestratableScans(nil)
	require.NoError(t, err)

	ids := scanIDs(scans)
	assert.True(t, ids[shared.ID], "shared orchestrator must drive non-isolated scans")
	assert.False(t, ids[isolated.ID], "shared orchestrator must not drive isolated scans")
}

func TestGetOrchestratableScans_FilteredToOwnScan(t *testing.T) {
	workspace, cleanup := setupOrchestratableScanTest(t)
	defer cleanup()

	own := createScanForTest(t, workspace.ID, ScanStatusScanning, true)
	otherIsolated := createScanForTest(t, workspace.ID, ScanStatusScanning, true)
	otherShared := createScanForTest(t, workspace.ID, ScanStatusCrawling, false)

	scans, err := Connection().GetOrchestratableScans(&own.ID)
	require.NoError(t, err)

	require.Len(t, scans, 1)
	assert.Equal(t, own.ID, scans[0].ID)

	ids := scanIDs(scans)
	assert.False(t, ids[otherIsolated.ID])
	assert.False(t, ids[otherShared.ID])
}

func TestGetOrchestratableScans_OnlyRunningStatuses(t *testing.T) {
	workspace, cleanup := setupOrchestratableScanTest(t)
	defer cleanup()

	crawling := createScanForTest(t, workspace.ID, ScanStatusCrawling, false)
	scanning := createScanForTest(t, workspace.ID, ScanStatusScanning, false)
	completed := createScanForTest(t, workspace.ID, ScanStatusCompleted, false)
	paused := createScanForTest(t, workspace.ID, ScanStatusPaused, false)

	scans, err := Connection().GetOrchestratableScans(nil)
	require.NoError(t, err)

	ids := scanIDs(scans)
	assert.True(t, ids[crawling.ID])
	assert.True(t, ids[scanning.ID])
	assert.False(t, ids[completed.ID])
	assert.False(t, ids[paused.ID])
}

// A finished scan must not be resurrected by its own orchestrator's filter.
func TestGetOrchestratableScans_FilteredScanRespectsStatus(t *testing.T) {
	workspace, cleanup := setupOrchestratableScanTest(t)
	defer cleanup()

	completed := createScanForTest(t, workspace.ID, ScanStatusCompleted, true)

	scans, err := Connection().GetOrchestratableScans(&completed.ID)
	require.NoError(t, err)
	assert.Empty(t, scans)
}
