package db

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// ensureWsFuzzTables creates the two new tables if missing — mirrors the
// ensureFuzzRunTable helper because the WebSocketConnection FK on
// PlaygroundWsFuzzIteration complicates AutoMigrate.
func ensureWsFuzzTables(t *testing.T) {
	t.Helper()
	m := Connection().DB().Migrator()
	if !m.HasTable(&PlaygroundWsFuzzRun{}) {
		require.NoError(t, m.CreateTable(&PlaygroundWsFuzzRun{}))
	}
	if !m.HasTable(&PlaygroundWsFuzzIteration{}) {
		require.NoError(t, m.CreateTable(&PlaygroundWsFuzzIteration{}))
	}
}

func mkWsFuzzSession(t *testing.T) (workspace uint, sessionID uint) {
	t.Helper()
	conn := Connection()
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: WsFuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	return ws.ID, sess.ID
}

func TestCreatePlaygroundWsFuzzRun(t *testing.T) {
	conn := Connection()
	ensureWsFuzzTables(t)
	_, sessionID := mkWsFuzzSession(t)

	run := &PlaygroundWsFuzzRun{
		SessionID:      sessionID,
		ConfigSnapshot: datatypes.JSON([]byte(`{"target_url":"ws://x/ws"}`)),
		Status:         "pending",
		IterationCount: 100,
	}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	require.NotZero(t, run.ID)

	got, err := conn.GetPlaygroundWsFuzzRun(run.ID)
	require.NoError(t, err)
	require.Equal(t, run.SessionID, got.SessionID)
	require.Equal(t, "pending", got.Status)
}

func TestPlaygroundWsFuzzIteration_CascadeDelete(t *testing.T) {
	conn := Connection()
	ensureWsFuzzTables(t)
	_, sessionID := mkWsFuzzSession(t)

	run := &PlaygroundWsFuzzRun{SessionID: sessionID, Status: "running"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))

	iter := &PlaygroundWsFuzzIteration{RunID: run.ID, IterationIndex: 0, Status: "completed"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzIteration(iter))

	require.NoError(t, conn.DeletePlaygroundWsFuzzRun(run.ID))

	_, err := conn.GetPlaygroundWsFuzzRun(run.ID)
	require.Error(t, err)

	var count int64
	require.NoError(t, conn.DB().Model(&PlaygroundWsFuzzIteration{}).Where("run_id = ?", run.ID).Count(&count).Error)
	require.Equal(t, int64(0), count, "iterations must cascade-delete with their run")
}

func TestRecoverOrphanedWsFuzzRuns(t *testing.T) {
	conn := Connection()
	ensureWsFuzzTables(t)
	_, sessionID := mkWsFuzzSession(t)

	for _, status := range []string{"pending", "calibrating", "running", "paused", "pausing"} {
		run := &PlaygroundWsFuzzRun{SessionID: sessionID, Status: status}
		require.NoError(t, conn.CreatePlaygroundWsFuzzRun(run))
	}
	// terminal — should NOT be touched
	doneRun := &PlaygroundWsFuzzRun{SessionID: sessionID, Status: "succeeded"}
	require.NoError(t, conn.CreatePlaygroundWsFuzzRun(doneRun))

	// Snapshot existing aborted-server-restart rows so the test is hermetic across reruns.
	var before int64
	require.NoError(t, conn.DB().Model(&PlaygroundWsFuzzRun{}).Where("status = ?", "aborted_server_restart").Count(&before).Error)

	n, err := conn.RecoverOrphanedWsFuzzRuns()
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, int64(5), "recovery sweep must mark at least our 5 non-terminal rows")

	got, err := conn.GetPlaygroundWsFuzzRun(doneRun.ID)
	require.NoError(t, err)
	require.Equal(t, "succeeded", got.Status, "terminal runs must NOT be re-stamped")

	var afterAborted int64
	require.NoError(t, conn.DB().Model(&PlaygroundWsFuzzRun{}).Where("status = ?", "aborted_server_restart").Count(&afterAborted).Error)
	require.GreaterOrEqual(t, afterAborted-before, int64(5))
}
