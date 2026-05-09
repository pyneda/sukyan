package wsreplay

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/stretchr/testify/require"
)

func TestDBPersisterCreatesAndCloses(t *testing.T) {
	conn := db.Connection()
	require.NoError(t, conn.DB().AutoMigrate(&db.WebSocketConnection{}, &db.WebSocketMessage{}))
	ws := createTestWorkspaceForPersister(t, conn)
	t.Cleanup(func() { conn.DeleteWorkspace(ws.ID) })

	p := NewDBPersister(conn)
	pid := uint(1) // placeholder playground_session_id; not validated in this test path
	id, err := p.CreateConnection("wss://example.com/ws", []HeaderSpec{{Key: "X", Value: "Y", Enabled: true}}, 101, "playground", &pid)
	require.NoError(t, err)
	require.NotZero(t, id)

	mid, err := p.RecordMessage(id, 1, "hello", "sent")
	require.NoError(t, err)
	require.NotZero(t, mid)

	require.NoError(t, p.CloseConnection(id))

	// Verify closed_at is set.
	got, err := conn.GetWebSocketConnection(id)
	require.NoError(t, err)
	require.NotNil(t, got.ClosedAt, "expected ClosedAt to be set")
}

// createTestWorkspaceForPersister mirrors db.createTestWorkspace but is local to this
// package (the db helper is unexported). Keeps the test self-contained.
func createTestWorkspaceForPersister(t *testing.T, conn *db.DatabaseConnection) *db.Workspace {
	t.Helper()
	ws := &db.Workspace{
		Code:        "wsreplay-persister-" + lib.GenerateRandomLowercaseString(8),
		Title:       "wsreplay-test",
		Description: "wsreplay persister test",
	}
	created, err := conn.CreateWorkspace(ws)
	require.NoError(t, err)
	return created
}
