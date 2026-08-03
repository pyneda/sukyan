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

func TestTopVulnTypesOrderIsTotal(t *testing.T) {
	issues := []*ReportIssue{
		{Code: "zebra", Title: "Zebra", Severity: "Low", URL: "https://a.test/1"},
		{Code: "alpha", Title: "Alpha", Severity: "Low", URL: "https://a.test/2"},
		{Code: "mango", Title: "Mango", Severity: "Low", URL: "https://a.test/3"},
	}

	want := generateSummary(issues).TopVulnTypes
	for i := 0; i < 200; i++ {
		require.Equal(t, want, generateSummary(issues).TopVulnTypes,
			"types with equal counts must come out in a fixed order")
	}
}

func TestGroupOrderIsTotalWhenTitlesCollide(t *testing.T) {
	issues := []*ReportIssue{
		{Code: "b_code", Title: "Same Title", Severity: "High", URL: "https://a.test/1"},
		{Code: "a_code", Title: "Same Title", Severity: "High", URL: "https://a.test/2"},
		{Code: "c_code", Title: "Same Title", Severity: "High", URL: "https://a.test/3"},
	}

	want := groupIssuesByType(issues)
	for i := 0; i < 200; i++ {
		got := groupIssuesByType(issues)
		require.Len(t, got, len(want))
		for j := range want {
			require.Equal(t, want[j].Code, got[j].Code, "tied groups must come out in a fixed order")
		}
	}
}

func TestInstanceOrderWithinGroupIsTotal(t *testing.T) {
	issues := []*ReportIssue{
		{ID: 3, Code: "c", Title: "C", Severity: "Low", URL: "https://a.test/z", Confidence: 50},
		{ID: 1, Code: "c", Title: "C", Severity: "Low", URL: "https://a.test/a", Confidence: 50},
		{ID: 2, Code: "c", Title: "C", Severity: "Low", URL: "https://a.test/m", Confidence: 50},
	}

	want := groupIssuesByType(issues)[0].Issues
	for i := 0; i < 200; i++ {
		got := groupIssuesByType(issues)[0].Issues
		require.Len(t, got, len(want))
		for j := range want {
			require.Equal(t, want[j].ID, got[j].ID, "tied instances must come out in a fixed order")
		}
	}
}
