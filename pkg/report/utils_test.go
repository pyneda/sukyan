package report

import (
	"encoding/base64"
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

func TestProcessIssuesMapsInteractionsAndPOC(t *testing.T) {
	workspace := setupTestWorkspace(t)
	defer func() {
		assert.NoError(t, db.Connection().DeleteWorkspace(workspace.ID))
	}()

	issue := createTestIssue(workspace.ID)
	issue.URL = "https://example.com/login"
	issue.POC = "<img src=x onerror=alert(1)>"
	issue.POCType = "html"
	saved, err := db.Connection().CreateIssue(*issue)
	require.NoError(t, err)

	oobTest, err := db.Connection().CreateOOBTest(db.OOBTest{
		Code:              "sqli",
		TestName:          "Blind SQLi via DNS",
		Target:            "https://example.com/login",
		InteractionDomain: "abc.oast.site",
		InteractionFullID: "abc123",
		Payload:           "' OR 1=1--",
		InsertionPoint:    "parameter:username",
		WorkspaceID:       &workspace.ID,
		IssueID:           &saved.ID,
	})
	require.NoError(t, err)

	_, err = db.Connection().CreateInteraction(&db.OOBInteraction{
		OOBTestID:     &oobTest.ID,
		Protocol:      "dns",
		FullID:        "abc123",
		UniqueID:      "abc",
		QType:         "A",
		RawRequest:    "query abc.oast.site",
		RemoteAddress: "203.0.113.7",
		Timestamp:     time.Now(),
		WorkspaceID:   &workspace.ID,
		IssueID:       &saved.ID,
	})
	require.NoError(t, err)

	reloaded, _, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID:         workspace.ID,
		IncludeInteractions: true,
	})
	require.NoError(t, err)
	require.NotEmpty(t, reloaded)

	processed := processIssues(reloaded, 0)

	var target *ReportIssue
	for _, candidate := range processed {
		if candidate.ID == saved.ID {
			target = candidate
			break
		}
	}
	require.NotNil(t, target, "the created issue should be in the processed set")

	assert.Equal(t, "<img src=x onerror=alert(1)>", target.POC)
	assert.Equal(t, "html", target.POCType)

	require.Len(t, target.Interactions, 1)
	interaction := target.Interactions[0]
	assert.Equal(t, "dns", interaction.Protocol)
	assert.Equal(t, "203.0.113.7", interaction.RemoteAddress)
	assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("query abc.oast.site")), interaction.RawRequest)

	require.NotNil(t, interaction.Cause)
	assert.Equal(t, "Blind SQLi via DNS", interaction.Cause.TestName)
	assert.Equal(t, "' OR 1=1--", interaction.Cause.Payload)
	assert.Equal(t, "parameter:username", interaction.Cause.InsertionPoint)
}
