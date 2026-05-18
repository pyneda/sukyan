package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ensureFuzzRunTable creates the playground_fuzz_runs table if missing.
// AutoMigrate from scratch fails for this model because History already
// declares a foreign key to it; CreateTable up-front sidesteps that ordering.
func ensureFuzzRunTable(t *testing.T) {
	t.Helper()
	m := Connection().DB().Migrator()
	if !m.HasTable(&PlaygroundFuzzRun{}) {
		require.NoError(t, m.CreateTable(&PlaygroundFuzzRun{}))
	}
}

func TestCreateAndGetPlaygroundFuzzRun(t *testing.T) {
	conn := Connection()
	ensureFuzzRunTable(t)
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: FuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))

	run := &PlaygroundFuzzRun{
		PlaygroundSessionID: sess.ID,
		WorkspaceID:         ws.ID,
		ConfigSnapshot:      []byte(`{"mode":"paired"}`),
		Status:              FuzzRunPending,
		PlannedRequestCount: 100,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))
	require.NotZero(t, run.ID)

	got, err := conn.GetPlaygroundFuzzRun(run.ID)
	require.NoError(t, err)
	require.Equal(t, sess.ID, got.PlaygroundSessionID)
	require.Equal(t, ws.ID, got.WorkspaceID)
	require.Equal(t, FuzzRunPending, got.Status)
	require.Equal(t, 100, got.PlannedRequestCount)
	require.JSONEq(t, `{"mode":"paired"}`, string(got.ConfigSnapshot))
}

func TestUpdatePlaygroundFuzzRunStatus(t *testing.T) {
	conn := Connection()
	ensureFuzzRunTable(t)
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: FuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))

	run := &PlaygroundFuzzRun{
		PlaygroundSessionID: sess.ID,
		WorkspaceID:         ws.ID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              FuzzRunPending,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(run))

	now := time.Now()
	run.Status = FuzzRunRunning
	run.StartedAt = &now
	run.SentRequestCount = 5
	require.NoError(t, conn.UpdatePlaygroundFuzzRun(run))

	got, err := conn.GetPlaygroundFuzzRun(run.ID)
	require.NoError(t, err)
	require.Equal(t, FuzzRunRunning, got.Status)
	require.Equal(t, 5, got.SentRequestCount)
	require.NotNil(t, got.StartedAt)
}

func TestListPlaygroundFuzzRunsOrdersNewestFirst(t *testing.T) {
	conn := Connection()
	ensureFuzzRunTable(t)
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: FuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))

	for i := 0; i < 3; i++ {
		run := &PlaygroundFuzzRun{
			PlaygroundSessionID: sess.ID,
			WorkspaceID:         ws.ID,
			ConfigSnapshot:      []byte(`{}`),
			Status:              FuzzRunPending,
		}
		require.NoError(t, conn.CreatePlaygroundFuzzRun(run))
	}

	runs, count, err := conn.ListPlaygroundFuzzRuns(sess.ID, 1, 10)
	require.NoError(t, err)
	require.Equal(t, int64(3), count)
	require.Len(t, runs, 3)
	require.True(t, runs[0].ID > runs[1].ID && runs[1].ID > runs[2].ID, "expected newest-first order")
}

func TestMarkOrphanedFuzzRunsAborted(t *testing.T) {
	conn := Connection()
	ensureFuzzRunTable(t)
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: FuzzType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))

	startedAt := time.Now().Add(-2 * time.Minute)
	// One in each "active" status — all should be swept.
	for _, status := range []PlaygroundFuzzRunStatus{FuzzRunPending, FuzzRunCalibrating, FuzzRunRunning, FuzzRunPaused} {
		run := &PlaygroundFuzzRun{
			PlaygroundSessionID: sess.ID,
			WorkspaceID:         ws.ID,
			ConfigSnapshot:      []byte(`{}`),
			Status:              status,
			StartedAt:           &startedAt,
		}
		require.NoError(t, conn.CreatePlaygroundFuzzRun(run))
	}

	// One terminal — must NOT be swept.
	finishedAt := time.Now().Add(-1 * time.Minute).Truncate(time.Microsecond)
	succeeded := &PlaygroundFuzzRun{
		PlaygroundSessionID: sess.ID,
		WorkspaceID:         ws.ID,
		ConfigSnapshot:      []byte(`{}`),
		Status:              FuzzRunSucceeded,
		StartedAt:           &startedAt,
		FinishedAt:          &finishedAt,
	}
	require.NoError(t, conn.CreatePlaygroundFuzzRun(succeeded))

	require.NoError(t, conn.MarkOrphanedFuzzRunsAborted())

	runs, _, err := conn.ListPlaygroundFuzzRuns(sess.ID, 0, 0)
	require.NoError(t, err)
	require.Len(t, runs, 5)

	aborted := 0
	for _, r := range runs {
		if r.ID == succeeded.ID {
			require.Equal(t, FuzzRunSucceeded, r.Status, "terminal run must not be swept")
			require.NotNil(t, r.FinishedAt)
			require.Equal(t, finishedAt.UTC(), r.FinishedAt.UTC(), "terminal run finished_at must be untouched")
			continue
		}
		require.Equal(t, FuzzRunAbortedServerRestart, r.Status)
		require.NotNil(t, r.FinishedAt)
		require.NotNil(t, r.FailureReason)
		require.Equal(t, "server restarted while run was in progress", *r.FailureReason)
		aborted++
	}
	require.Equal(t, 4, aborted)
}

func TestPlaygroundFuzzRunStatusIsTerminal(t *testing.T) {
	cases := map[PlaygroundFuzzRunStatus]bool{
		FuzzRunPending:              false,
		FuzzRunCalibrating:          false,
		FuzzRunRunning:              false,
		FuzzRunPaused:               false,
		FuzzRunSucceeded:            true,
		FuzzRunFailed:               true,
		FuzzRunCancelled:            true,
		FuzzRunStoppedErrorRate:     true,
		FuzzRunStoppedMaxDuration:   true,
		FuzzRunAbortedServerRestart: true,
	}
	for status, want := range cases {
		require.Equal(t, want, status.IsTerminal(), "IsTerminal(%s)", status)
	}
}
