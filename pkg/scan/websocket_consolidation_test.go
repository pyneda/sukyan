package scan

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/pyneda/sukyan/pkg/scan/options"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// uniqueWSURL returns a per-run unique WebSocket URL so issue-count assertions are
// isolated from issues left in the persistent test DB by previous runs.
func uniqueWSURL(name string) string {
	return fmt.Sprintf("ws://%s.test/ws-%d", name, time.Now().UnixNano())
}

// TestWebSocketHandleVulnerabilityConsolidation verifies that many matching
// payloads for the same (endpoint, code, insertion point, message) produce a
// SINGLE issue with the additional payloads' upgrade requests attached as
// evidence, rather than one issue per payload.
func TestWebSocketHandleVulnerabilityConsolidation(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "WS Consolidation",
		Code:  "ws-consolidation",
	})
	require.NoError(t, err)

	conn := &db.WebSocketConnection{
		URL:             uniqueWSURL("consolidation"),
		Source:          db.SourceScanner,
		WorkspaceID:     &workspace.ID,
		StatusCode:      101,
		RequestHeaders:  datatypes.JSON(`{}`),
		ResponseHeaders: datatypes.JSON(`{}`),
	}
	require.NoError(t, db.Connection().CreateWebSocketConnection(conn))

	scanner := &WebSocketScanner{AvoidRepeatedIssues: true}

	point := InsertionPoint{Type: InsertionPointTypeWSJSONValue, Name: "id", Value: "1"}
	modified := &db.WebSocketMessage{
		ConnectionID: conn.ID,
		Opcode:       1,
		PayloadData:  `{"id":"1' OR '1'='1"}`,
		Direction:    db.MessageSent,
	}
	result := &WebSocketScannerResult{ModifiedMessage: modified}
	task := WebSocketScannerTask{
		connection:         conn,
		targetMessageIndex: 0,
		insertionPoint:     point,
		payload:            generation.Payload{Value: "1' OR '1'='1", IssueCode: string(db.SqlInjectionCode)},
		options:            options.WebSocketScanOptions{WorkspaceID: workspace.ID},
	}

	const payloadCount = 4
	var upgradeHistories []*db.History
	for i := 0; i < payloadCount; i++ {
		h, herr := db.Connection().CreateHistory(&db.History{
			URL:         "http://consolidation.test/ws",
			Method:      "GET",
			StatusCode:  101,
			Source:      db.SourceScanner,
			WorkspaceID: &workspace.ID,
		})
		require.NoError(t, herr)
		upgradeHistories = append(upgradeHistories, h)
	}

	for _, h := range upgradeHistories {
		scanner.handleVulnerability(result, task, "SQL error detected", 90, *h, *conn, log.Logger)
	}

	issues, _, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID: workspace.ID,
		Codes:       []string{string(db.SqlInjectionCode)},
		URL:         conn.URL,
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(issues), "expected exactly one consolidated issue for %d matching payloads", payloadCount)

	var evidenceCount int64
	evidenceCount = db.Connection().DB().Model(&issues[0]).Association("Requests").Count()
	assert.GreaterOrEqual(t, evidenceCount, int64(payloadCount-1),
		"additional payloads' upgrade requests must be attached as evidence")
}

// TestWebSocketHandleVulnerabilityDistinctInsertionPoints verifies that distinct
// insertion points on the same message remain distinct findings (identity is per
// insertion point), so consolidation does not collapse genuinely different sinks.
func TestWebSocketHandleVulnerabilityDistinctInsertionPoints(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "WS Consolidation Points",
		Code:  "ws-consolidation-points",
	})
	require.NoError(t, err)

	conn := &db.WebSocketConnection{
		URL:             uniqueWSURL("consolidation-points"),
		Source:          db.SourceScanner,
		WorkspaceID:     &workspace.ID,
		StatusCode:      101,
		RequestHeaders:  datatypes.JSON(`{}`),
		ResponseHeaders: datatypes.JSON(`{}`),
	}
	require.NoError(t, db.Connection().CreateWebSocketConnection(conn))

	scanner := &WebSocketScanner{AvoidRepeatedIssues: true}
	modified := &db.WebSocketMessage{ConnectionID: conn.ID, Opcode: 1, PayloadData: `{"id":"x"}`, Direction: db.MessageSent}
	result := &WebSocketScannerResult{ModifiedMessage: modified}

	for _, name := range []string{"id", "name"} {
		task := WebSocketScannerTask{
			connection:         conn,
			targetMessageIndex: 0,
			insertionPoint:     InsertionPoint{Type: InsertionPointTypeWSJSONValue, Name: name, Value: "x"},
			payload:            generation.Payload{Value: "x' OR 1=1", IssueCode: string(db.SqlInjectionCode)},
			options:            options.WebSocketScanOptions{WorkspaceID: workspace.ID},
		}
		h, herr := db.Connection().CreateHistory(&db.History{URL: "http://consolidation-points.test/ws", Method: "GET", StatusCode: 101, Source: db.SourceScanner, WorkspaceID: &workspace.ID})
		require.NoError(t, herr)
		scanner.handleVulnerability(result, task, "SQL error detected", 90, *h, *conn, log.Logger)
	}

	issues, _, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID: workspace.ID,
		Codes:       []string{string(db.SqlInjectionCode)},
		URL:         conn.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, len(issues), "two distinct insertion points must produce two findings")
}

// TestWebSocketHandleVulnerabilityConsolidationConcurrent forces the dedup race:
// many goroutines call handleVulnerability with the SAME finding identity at once
// (mirroring the payload pool). The atomic LoadOrStore must let exactly ONE issue
// be created. Run with -race to prove the sync.Map claim is sound under contention.
func TestWebSocketHandleVulnerabilityConsolidationConcurrent(t *testing.T) {
	workspace, err := db.Connection().GetOrCreateWorkspace(&db.Workspace{
		Title: "WS Consolidation Concurrent",
		Code:  "ws-consolidation-concurrent",
	})
	require.NoError(t, err)

	conn := &db.WebSocketConnection{
		URL:             uniqueWSURL("consolidation-concurrent"),
		Source:          db.SourceScanner,
		WorkspaceID:     &workspace.ID,
		StatusCode:      101,
		RequestHeaders:  datatypes.JSON(`{}`),
		ResponseHeaders: datatypes.JSON(`{}`),
	}
	require.NoError(t, db.Connection().CreateWebSocketConnection(conn))

	scanner := &WebSocketScanner{AvoidRepeatedIssues: true}
	point := InsertionPoint{Type: InsertionPointTypeWSJSONValue, Name: "id", Value: "1"}

	const goroutines = 12
	histories := make([]*db.History, goroutines)
	for i := 0; i < goroutines; i++ {
		h, herr := db.Connection().CreateHistory(&db.History{
			URL:         "http://consolidation-concurrent.test/ws",
			Method:      "GET",
			StatusCode:  101,
			Source:      db.SourceScanner,
			WorkspaceID: &workspace.ID,
		})
		require.NoError(t, herr)
		histories[i] = h
	}

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine has its own result and task copy, exactly as each
			// processTask does; only scanner.issuesFound is shared.
			result := &WebSocketScannerResult{ModifiedMessage: &db.WebSocketMessage{
				ConnectionID: conn.ID,
				Opcode:       1,
				PayloadData:  fmt.Sprintf(`{"id":"1' OR %d=%d"}`, idx, idx),
				Direction:    db.MessageSent,
			}}
			task := WebSocketScannerTask{
				connection:         conn,
				targetMessageIndex: 0,
				insertionPoint:     point,
				payload:            generation.Payload{Value: "1' OR 1=1", IssueCode: string(db.SqlInjectionCode)},
				options:            options.WebSocketScanOptions{WorkspaceID: workspace.ID},
			}
			scanner.handleVulnerability(result, task, "SQL error detected", 90, *histories[idx], *conn, log.Logger)
		}(i)
	}
	wg.Wait()

	issues, _, err := db.Connection().ListIssues(db.IssueFilter{
		WorkspaceID: workspace.ID,
		Codes:       []string{string(db.SqlInjectionCode)},
		URL:         conn.URL,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, len(issues), "exactly one issue must survive %d concurrent matching payloads", goroutines)
}
