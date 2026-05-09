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
