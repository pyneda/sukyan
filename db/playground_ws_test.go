package db

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateAndGetPlaygroundWsSession(t *testing.T) {
	conn := Connection()
	require.NoError(t, conn.DB().AutoMigrate(&PlaygroundWsSession{}))
	ws := createTestWorkspace(t)
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
