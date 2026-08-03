package db

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedWorkspaceForDeletion builds a workspace carrying rows in every bulk table
// plus a few of the small ones left to the cascade.
func seedWorkspaceForDeletion(t *testing.T, code string, historyCount int) *Workspace {
	t.Helper()
	conn := Connection()

	workspace, err := conn.GetOrCreateWorkspace(&Workspace{
		Code:  code,
		Title: "Workspace delete test",
	})
	require.NoError(t, err)

	scan, err := conn.CreateScan(&Scan{
		WorkspaceID: workspace.ID,
		Status:      ScanStatusPending,
		Title:       "delete test scan",
	})
	require.NoError(t, err)

	var firstHistory *History
	for i := 0; i < historyCount; i++ {
		h := &History{
			URL:         fmt.Sprintf("http://example.com/delete-test/%d", i),
			StatusCode:  200,
			Method:      "GET",
			WorkspaceID: &workspace.ID,
			ScanID:      &scan.ID,
			Source:      SourceScanner,
		}
		created, err := conn.CreateHistory(h)
		require.NoError(t, err)
		if firstHistory == nil {
			firstHistory = created
		}
	}

	conn2 := &WebSocketConnection{
		URL:         "ws://example.com/ws",
		WorkspaceID: &workspace.ID,
		Source:      SourceScanner,
	}
	require.NoError(t, conn.CreateWebSocketConnection(conn2))

	require.NoError(t, conn.DB().Create(&WebSocketMessage{
		ConnectionID: conn2.ID,
		PayloadData:  "hello",
		Direction:    MessageSent,
	}).Error)

	issue, err := conn.CreateIssue(Issue{
		Code:        string(XssReflectedCode),
		Title:       "delete test issue",
		WorkspaceID: &workspace.ID,
		URL:         "http://example.com/delete-test/0",
	})
	require.NoError(t, err)

	oobTest, err := conn.CreateOOBTest(OOBTest{
		Code:              XssReflectedCode,
		TestName:          "delete test oob",
		Target:            "http://example.com",
		WorkspaceID:       &workspace.ID,
		HistoryID:         &firstHistory.ID,
		InteractionDomain: "example.oob",
		InteractionFullID: fmt.Sprintf("%s-oob", code),
	})
	require.NoError(t, err)

	_, err = conn.CreateInteraction(&OOBInteraction{
		OOBTestID:   &oobTest.ID,
		WorkspaceID: &workspace.ID,
		IssueID:     &issue.ID,
		Protocol:    "dns",
		FullID:      fmt.Sprintf("%s-int", code),
	})
	require.NoError(t, err)

	return workspace
}

func countWorkspaceRows(t *testing.T, workspaceID uint) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64)
	tables := []string{"histories", "web_socket_connections", "oob_tests", "oob_interactions", "issues", "scans", "workspaces"}
	for _, table := range tables {
		var n int64
		column := "workspace_id"
		if table == "workspaces" {
			column = "id"
		}
		require.NoError(t, Connection().DB().
			Raw(fmt.Sprintf("SELECT count(*) FROM %s WHERE %s = ?", table, column), workspaceID).
			Scan(&n).Error)
		counts[table] = n
	}
	return counts
}

func TestDeleteWorkspaceCascadeRemovesEverything(t *testing.T) {
	workspace := seedWorkspaceForDeletion(t, "ws-delete-full", 25)

	before := countWorkspaceRows(t, workspace.ID)
	require.EqualValues(t, 25, before["histories"])
	require.EqualValues(t, 1, before["web_socket_connections"])
	require.EqualValues(t, 1, before["oob_tests"])
	require.EqualValues(t, 1, before["oob_interactions"])
	require.EqualValues(t, 1, before["issues"])
	require.EqualValues(t, 1, before["workspaces"])

	result, err := Connection().DeleteWorkspaceCascade(context.Background(), workspace.ID, WorkspaceDeleteOptions{BatchSize: 7})
	require.NoError(t, err)

	after := countWorkspaceRows(t, workspace.ID)
	for table, n := range after {
		assert.Zero(t, n, "table %s still holds rows for the deleted workspace", table)
	}

	assert.EqualValues(t, 25, result.RowsByTable["histories"])
	assert.EqualValues(t, 1, result.RowsByTable["workspaces"])
	assert.Greater(t, result.TotalRows, int64(25))
	assert.EqualValues(t, 1, result.ScansCancelled, "the pending scan must be cancelled so workers stop writing into the workspace")

	var orphanMessages int64
	require.NoError(t, Connection().DB().Raw(`
		SELECT count(*) FROM web_socket_messages m
		LEFT JOIN web_socket_connections c ON c.id = m.connection_id
		WHERE c.id IS NULL`).Scan(&orphanMessages).Error)
	assert.Zero(t, orphanMessages, "websocket messages must not outlive their connection")
}

// A batch size smaller than the row count must still empty the table, proving
// the loop terminates rather than stopping after a single batch.
func TestDeleteWorkspaceCascadeBatchesUntilEmpty(t *testing.T) {
	workspace := seedWorkspaceForDeletion(t, "ws-delete-batched", 23)

	var batches int
	result, err := Connection().DeleteWorkspaceCascade(context.Background(), workspace.ID, WorkspaceDeleteOptions{
		BatchSize: 5,
		Progress: func(table string, deleted int64) {
			if table == "histories" {
				batches++
			}
		},
	})
	require.NoError(t, err)

	assert.EqualValues(t, 23, result.RowsByTable["histories"])
	assert.Equal(t, 5, batches, "23 rows at a batch size of 5 should take 5 batches")
	assert.Zero(t, countWorkspaceRows(t, workspace.ID)["histories"])
}

func TestDeleteWorkspaceCascadeHonoursCancellation(t *testing.T) {
	workspace := seedWorkspaceForDeletion(t, "ws-delete-cancel", 30)

	ctx, cancel := context.WithCancel(context.Background())
	_, err := Connection().DeleteWorkspaceCascade(ctx, workspace.ID, WorkspaceDeleteOptions{
		BatchSize: 5,
		Progress: func(table string, deleted int64) {
			if table == "histories" && deleted >= 10 {
				cancel()
			}
		},
	})
	require.ErrorIs(t, err, context.Canceled)

	after := countWorkspaceRows(t, workspace.ID)
	assert.EqualValues(t, 1, after["workspaces"], "cancelling must leave the workspace row in place")
	assert.Greater(t, after["histories"], int64(0), "cancelling must stop before draining the table")
	assert.Less(t, after["histories"], int64(30), "the batches completed before cancellation must have been committed")

	// Resumable: a second, uncancelled run finishes the job.
	_, err = Connection().DeleteWorkspaceCascade(context.Background(), workspace.ID, WorkspaceDeleteOptions{BatchSize: 5})
	require.NoError(t, err)
	for table, n := range countWorkspaceRows(t, workspace.ID) {
		assert.Zero(t, n, "table %s still holds rows after the resumed delete", table)
	}
}

// histories and scan_jobs reference each other with ON DELETE CASCADE, so
// deleting one history removes its scan job and every other history of that
// job. A batch that triggers that cascade is bounded in name only: it can take
// most of the workspace in a single transaction.
func TestDeleteWorkspaceCascadeBoundsTheHistoryScanJobCycle(t *testing.T) {
	conn := Connection()

	workspace, err := conn.GetOrCreateWorkspace(&Workspace{Code: "ws-delete-cycle", Title: "cycle"})
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.DeleteWorkspaceCascade(context.Background(), workspace.ID, WorkspaceDeleteOptions{})
	})

	scan, err := conn.CreateScan(&Scan{WorkspaceID: workspace.ID, Status: ScanStatusCompleted, Title: "cycle scan"})
	require.NoError(t, err)

	job := &ScanJob{
		ScanID:      scan.ID,
		WorkspaceID: workspace.ID,
		JobType:     ScanJobTypeActiveScan,
		Status:      ScanJobStatusCompleted,
	}
	require.NoError(t, conn.DB().Create(job).Error)

	const total = 40
	var first *History
	for i := 0; i < total; i++ {
		created, err := conn.CreateHistory(&History{
			URL:         fmt.Sprintf("http://example.com/cycle/%d", i),
			StatusCode:  200,
			Method:      "GET",
			WorkspaceID: &workspace.ID,
			ScanID:      &scan.ID,
			ScanJobID:   &job.ID,
			Source:      SourceScanner,
		})
		require.NoError(t, err)
		if first == nil {
			first = created
		}
	}
	// Closes the cycle: the job points back at one of its own histories.
	require.NoError(t, conn.DB().Model(job).Update("history_id", first.ID).Error)

	// Stop after the first histories batch and count what actually went.
	ctx, cancel := context.WithCancel(context.Background())
	_, err = conn.DeleteWorkspaceCascade(ctx, workspace.ID, WorkspaceDeleteOptions{
		BatchSize: 10,
		Progress: func(table string, deleted int64) {
			if table == "histories" {
				cancel()
			}
		},
	})
	require.ErrorIs(t, err, context.Canceled)

	var remaining int64
	require.NoError(t, conn.DB().
		Raw("SELECT count(*) FROM histories WHERE workspace_id = ?", workspace.ID).
		Scan(&remaining).Error)

	assert.EqualValues(t, total-10, remaining,
		"one batch of 10 should remove exactly 10 histories; %d of %d went, so the cascade is unbounded",
		total-remaining, total)
}

func TestDeleteWorkspaceCascadeRejectsZeroID(t *testing.T) {
	_, err := Connection().DeleteWorkspaceCascade(context.Background(), 0, WorkspaceDeleteOptions{})
	require.Error(t, err)
}
