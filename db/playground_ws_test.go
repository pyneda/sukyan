package db

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateAndGetPlaygroundWsSession(t *testing.T) {
	conn := Connection()
	require.NoError(t, conn.DB().AutoMigrate(&PlaygroundWsSession{}))
	ws := createTestWorkspace(t)
	t.Cleanup(func() { Connection().DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	if err := conn.CreatePlaygroundCollection(coll); err != nil {
		t.Fatal(err)
	}
	sess := &PlaygroundSession{Name: "s", Type: WsManualType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	if err := conn.CreatePlaygroundSession(sess); err != nil {
		t.Fatal(err)
	}
	headers, _ := json.Marshal([]map[string]any{{"key": "X", "value": "Y", "enabled": true}})
	wsSess := &PlaygroundWsSession{
		PlaygroundSessionID: sess.ID,
		TargetURL:           "wss://example.com/ws",
		RequestHeaders:      headers,
		Script:              json.RawMessage(`[]`),
		Options:             json.RawMessage(`{}`),
	}
	if err := conn.CreatePlaygroundWsSession(wsSess); err != nil {
		t.Fatal(err)
	}
	got, err := conn.GetPlaygroundWsSessionBySessionID(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetURL != "wss://example.com/ws" {
		t.Fatalf("unexpected target_url: %q", got.TargetURL)
	}
	require.Equal(t, sess.ID, got.PlaygroundSessionID)
}

func TestUpdatePlaygroundWsSession(t *testing.T) {
	conn := Connection()
	require.NoError(t, conn.DB().AutoMigrate(&PlaygroundWsSession{}))
	ws := createTestWorkspace(t)
	t.Cleanup(func() { Connection().DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	if err := conn.CreatePlaygroundCollection(coll); err != nil {
		t.Fatal(err)
	}
	sess := &PlaygroundSession{Name: "s", Type: WsManualType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	if err := conn.CreatePlaygroundSession(sess); err != nil {
		t.Fatal(err)
	}
	wsSess := &PlaygroundWsSession{PlaygroundSessionID: sess.ID, TargetURL: "wss://a"}
	if err := conn.CreatePlaygroundWsSession(wsSess); err != nil {
		t.Fatal(err)
	}
	wsSess.TargetURL = "wss://b"
	if err := conn.UpdatePlaygroundWsSession(wsSess); err != nil {
		t.Fatal(err)
	}
	got, err := conn.GetPlaygroundWsSessionBySessionID(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TargetURL != "wss://b" {
		t.Fatalf("update did not persist: %q", got.TargetURL)
	}
}

func TestCreateAndListPlaygroundWsRuns(t *testing.T) {
	conn := Connection()
	require.NoError(t, conn.DB().AutoMigrate(&PlaygroundWsSession{}, &PlaygroundWsRun{}))
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: WsManualType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	wsSess := &PlaygroundWsSession{PlaygroundSessionID: sess.ID, TargetURL: "wss://a"}
	require.NoError(t, conn.CreatePlaygroundWsSession(wsSess))

	for i := 0; i < 3; i++ {
		run := &PlaygroundWsRun{
			PlaygroundWsSessionID: wsSess.ID,
			Status:                WsRunPending,
			ScriptSnapshot:        []byte("[]"),
			OptionsSnapshot:       []byte("{}"),
		}
		require.NoError(t, conn.CreatePlaygroundWsRun(run))
	}
	runs, count, err := conn.ListPlaygroundWsRuns(wsSess.ID, 1, 10)
	require.NoError(t, err)
	if count != 3 || len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d (count=%d)", len(runs), count)
	}
}

func TestRecoveryMarkOrphanedRuns(t *testing.T) {
	conn := Connection()
	require.NoError(t, conn.DB().AutoMigrate(&PlaygroundWsSession{}, &PlaygroundWsRun{}))
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: WsManualType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	wsSess := &PlaygroundWsSession{PlaygroundSessionID: sess.ID, TargetURL: "wss://a"}
	require.NoError(t, conn.CreatePlaygroundWsSession(wsSess))
	now := time.Now()
	stuck := &PlaygroundWsRun{
		PlaygroundWsSessionID: wsSess.ID,
		Status:                WsRunRunning,
		StartedAt:             &now,
		ScriptSnapshot:        []byte("[]"),
		OptionsSnapshot:       []byte("{}"),
	}
	require.NoError(t, conn.CreatePlaygroundWsRun(stuck))
	require.NoError(t, conn.MarkOrphanedWsRunsAborted())
	got, err := conn.GetPlaygroundWsRun(stuck.ID)
	require.NoError(t, err)
	if got.Status != WsRunAbortedServerRestart {
		t.Fatalf("expected aborted_server_restart, got %s", got.Status)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected finished_at to be set")
	}
}

func TestRecoveryCloseOrphanedConnections(t *testing.T) {
	conn := Connection()
	require.NoError(t, conn.DB().AutoMigrate(&PlaygroundWsSession{}, &WebSocketConnection{}))
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: WsManualType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	wsSess := &PlaygroundWsSession{PlaygroundSessionID: sess.ID, TargetURL: "wss://a"}
	require.NoError(t, conn.CreatePlaygroundWsSession(wsSess))

	wsConn := &WebSocketConnection{
		URL:                 "wss://example.com/ws",
		WorkspaceID:         &ws.ID,
		Source:              "playground",
		PlaygroundSessionID: &sess.ID,
		// ClosedAt intentionally left at zero value
	}
	require.NoError(t, conn.CreateWebSocketConnection(wsConn))

	require.NoError(t, conn.CloseOrphanedPlaygroundConnections())

	var got WebSocketConnection
	require.NoError(t, conn.DB().First(&got, wsConn.ID).Error)
	if got.ClosedAt == nil || got.ClosedAt.IsZero() {
		closed := "<nil>"
		if got.ClosedAt != nil {
			closed = got.ClosedAt.Format(time.RFC3339Nano)
		}
		t.Fatalf("expected closed_at to be stamped by recovery sweep, got %s", closed)
	}
}

func TestMarkOrphanedWsRunsAbortedLeavesNonOrphansUntouched(t *testing.T) {
	conn := Connection()
	require.NoError(t, conn.DB().AutoMigrate(&PlaygroundWsSession{}, &PlaygroundWsRun{}))
	ws := createTestWorkspace(t)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })
	coll := &PlaygroundCollection{Name: "c", WorkspaceID: ws.ID}
	require.NoError(t, conn.CreatePlaygroundCollection(coll))
	sess := &PlaygroundSession{Name: "s", Type: WsManualType, WorkspaceID: ws.ID, CollectionID: coll.ID}
	require.NoError(t, conn.CreatePlaygroundSession(sess))
	wsSess := &PlaygroundWsSession{PlaygroundSessionID: sess.ID, TargetURL: "wss://a"}
	require.NoError(t, conn.CreatePlaygroundWsSession(wsSess))

	startedAt := time.Now().Add(-2 * time.Minute)
	running := &PlaygroundWsRun{
		PlaygroundWsSessionID: wsSess.ID,
		Status:                WsRunRunning,
		StartedAt:             &startedAt,
		ScriptSnapshot:        []byte("[]"),
		OptionsSnapshot:       []byte("{}"),
	}
	require.NoError(t, conn.CreatePlaygroundWsRun(running))

	// Truncate to microsecond — postgres TIMESTAMPTZ has microsecond precision,
	// so the round-tripped value drops nanoseconds. We want the equality assertion
	// below to test "the sweep didn't change finished_at", not driver precision.
	finishedAt := time.Now().Add(-1 * time.Minute).Truncate(time.Microsecond)
	succeeded := &PlaygroundWsRun{
		PlaygroundWsSessionID: wsSess.ID,
		Status:                WsRunSucceeded,
		StartedAt:             &startedAt,
		FinishedAt:            &finishedAt,
		ScriptSnapshot:        []byte("[]"),
		OptionsSnapshot:       []byte("{}"),
	}
	require.NoError(t, conn.CreatePlaygroundWsRun(succeeded))

	require.NoError(t, conn.MarkOrphanedWsRunsAborted())

	gotRunning, err := conn.GetPlaygroundWsRun(running.ID)
	require.NoError(t, err)
	if gotRunning.Status != WsRunAbortedServerRestart {
		t.Fatalf("expected running run status %s, got %s", WsRunAbortedServerRestart, gotRunning.Status)
	}
	if gotRunning.FinishedAt == nil {
		t.Fatal("expected running run finished_at to be set")
	}
	if gotRunning.FailureReason == nil || *gotRunning.FailureReason != "server restarted while run was in progress" {
		t.Fatalf("expected failure_reason to be set; got %v", gotRunning.FailureReason)
	}

	gotSucceeded, err := conn.GetPlaygroundWsRun(succeeded.ID)
	require.NoError(t, err)
	if gotSucceeded.Status != WsRunSucceeded {
		t.Fatalf("expected succeeded run to remain %s, got %s", WsRunSucceeded, gotSucceeded.Status)
	}
	if gotSucceeded.FinishedAt == nil {
		t.Fatal("expected succeeded run finished_at to remain set")
	}
	if !gotSucceeded.FinishedAt.Equal(finishedAt) {
		t.Fatalf("expected succeeded run finished_at unchanged (%s), got %s",
			finishedAt.Format(time.RFC3339Nano),
			gotSucceeded.FinishedAt.Format(time.RFC3339Nano))
	}
	if gotSucceeded.FailureReason != nil {
		t.Fatalf("expected succeeded run failure_reason to remain nil, got %q", *gotSucceeded.FailureReason)
	}
}
