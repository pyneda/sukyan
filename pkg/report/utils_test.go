package report

import (
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessIssuesAttachesWebSocketMessages(t *testing.T) {
	workspace := setupTestWorkspace(t)
	defer func() {
		assert.NoError(t, db.Connection().DeleteWorkspace(workspace.ID))
	}()

	conn := &db.WebSocketConnection{
		URL:         "ws://example.com/socket",
		StatusCode:  101,
		StatusText:  "Switching Protocols",
		Source:      "crawler",
		WorkspaceID: &workspace.ID,
	}
	require.NoError(t, db.Connection().CreateWebSocketConnection(conn))

	require.NoError(t, db.Connection().CreateWebSocketMessage(&db.WebSocketMessage{
		ConnectionID: conn.ID,
		Opcode:       1,
		PayloadData:  "hello",
		Direction:    db.MessageSent,
		Timestamp:    time.Now(),
	}))

	// Two issues sharing one connection: the batch must serve both from a
	// single fetch.
	issues := make([]*db.Issue, 0, 2)
	for i := range 2 {
		issue := createTestIssue(workspace.ID)
		issue.Title = "WebSocket Issue"
		issue.URL = "ws://example.com/socket?n=" + string(rune('a'+i))
		issue.WebsocketConnectionID = &conn.ID
		saved, err := db.Connection().CreateIssue(*issue)
		require.NoError(t, err)
		issues = append(issues, &saved)
	}

	processed := processIssues(issues, 0)
	require.Len(t, processed, 2)

	for _, issue := range processed {
		require.NotNil(t, issue.WebSocketConnection, "websocket connection should be attached")
		assert.Equal(t, "ws://example.com/socket", issue.WebSocketConnection.URL)
		assert.Equal(t, 101, issue.WebSocketConnection.StatusCode)
		require.Len(t, issue.WebSocketConnection.Messages, 1, "messages must survive the batched fetch")
		assert.Equal(t, "hello", issue.WebSocketConnection.Messages[0].PayloadData)
		assert.Equal(t, "sent", issue.WebSocketConnection.Messages[0].Direction)
	}
}
