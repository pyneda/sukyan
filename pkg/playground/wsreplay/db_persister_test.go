package wsreplay

import (
	"testing"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/require"
)

func TestDBPersisterCreatesAndCloses(t *testing.T) {
	conn := db.Connection()
	require.NoError(t, conn.DB().AutoMigrate(&db.WebSocketConnection{}, &db.WebSocketMessage{}))

	p := NewDBPersister(conn)
	pid := uint(0) // no playground session linked; FK is nullable.
	var pidPtr *uint
	if pid != 0 {
		pidPtr = &pid
	}
	id, err := p.CreateConnection("wss://example.com/ws", []HeaderSpec{{Key: "X", Value: "Y", Enabled: true}}, 101, "playground", pidPtr)
	require.NoError(t, err)
	require.NotZero(t, id)

	mid, err := p.RecordMessage(id, 1, "hello", "sent")
	require.NoError(t, err)
	require.NotZero(t, mid)

	require.NoError(t, p.CloseConnection(id))

	got, err := conn.GetWebSocketConnection(id)
	require.NoError(t, err)
	require.NotNil(t, got.ClosedAt, "expected ClosedAt to be set")

	// Cleanup: hard-delete the connection (workspace cascade isn't available since we have no workspace).
	t.Cleanup(func() {
		conn.DB().Unscoped().Delete(&db.WebSocketConnection{}, id)
	})
}
