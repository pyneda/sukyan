package workspace

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/datatypes"
)

// seededWorkspace holds the identifiers a test needs to assert against.
type seededWorkspace struct {
	Workspace  *db.Workspace
	RawRequest []byte
	HistoryURL string
}

// seedRichWorkspace populates a workspace across the tables that exercise every
// interesting part of the transfer: bigint keys, uuid keys, join tables, bytea,
// jsonb, and both foreign key cycles.
func seedRichWorkspace(t *testing.T, code string) seededWorkspace {
	t.Helper()
	conn := db.Connection()

	workspace, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Code:        code,
		Title:       "Round trip source",
		Description: "seeded for export/import tests",
	})
	require.NoError(t, err)

	scan, err := conn.CreateScan(&db.Scan{
		WorkspaceID: workspace.ID,
		Status:      db.ScanStatusCompleted,
		Title:       "round trip scan",
	})
	require.NoError(t, err)

	task := &db.Task{Title: "round trip task", WorkspaceID: workspace.ID, Status: "finished"}
	require.NoError(t, conn.DB().Create(task).Error)

	collection := &db.PlaygroundCollection{Name: "rt collection", WorkspaceID: workspace.ID}
	require.NoError(t, conn.DB().Create(collection).Error)

	session := &db.PlaygroundSession{
		Name:         "rt session",
		Type:         db.ManualType,
		CollectionID: collection.ID,
		WorkspaceID:  workspace.ID,
	}
	require.NoError(t, conn.DB().Create(session).Error)

	rawRequest := []byte("GET /round-trip HTTP/1.1\r\nHost: example.com\r\nX-Binary: \x00\x01\x02\xff\r\n\r\n")
	rawResponse := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html>\x00\xfe</html>")

	histories := make([]*db.History, 0, 4)
	for i := 0; i < 4; i++ {
		history := &db.History{
			URL:                 fmt.Sprintf("http://example.com/round-trip/%d", i),
			CleanURL:            fmt.Sprintf("http://example.com/round-trip/%d", i),
			StatusCode:          200,
			Method:              "GET",
			RawRequest:          rawRequest,
			RawResponse:         rawResponse,
			WorkspaceID:         &workspace.ID,
			ScanID:              &scan.ID,
			TaskID:              &task.ID,
			Source:              db.SourceScanner,
			ResponseContentType: "text/html",
		}
		created, err := conn.CreateHistory(history)
		require.NoError(t, err)
		histories = append(histories, created)
	}

	// Closes the playground_sessions -> histories half of a foreign key cycle.
	require.NoError(t, conn.DB().Model(session).Update("original_request_id", histories[0].ID).Error)

	scanJob := &db.ScanJob{
		ScanID:      scan.ID,
		WorkspaceID: workspace.ID,
		JobType:     db.ScanJobTypeActiveScan,
		Status:      db.ScanJobStatusCompleted,
		HistoryID:   &histories[1].ID,
	}
	require.NoError(t, conn.DB().Create(scanJob).Error)

	definition := &db.APIDefinition{
		WorkspaceID:     workspace.ID,
		Name:            "rt api",
		Type:            db.APIDefinitionTypeOpenAPI,
		BaseURL:         "http://example.com/api",
		RawDefinition:   []byte(`{"openapi":"3.0.0"}`),
		SourceHistoryID: &histories[2].ID,
	}
	require.NoError(t, conn.DB().Create(definition).Error)

	endpoint := &db.APIEndpoint{DefinitionID: definition.ID, Path: "/rt", Method: "GET"}
	require.NoError(t, conn.DB().Create(endpoint).Error)

	connection := &db.WebSocketConnection{
		URL:              "ws://example.com/socket",
		WorkspaceID:      &workspace.ID,
		ScanID:           &scan.ID,
		UpgradeRequestID: &histories[3].ID,
		RequestHeaders:   datatypes.JSON([]byte(`{"Origin":["http://example.com"]}`)),
		Source:           db.SourceScanner,
	}
	require.NoError(t, conn.CreateWebSocketConnection(connection))
	require.NoError(t, conn.DB().Model(scanJob).Update("web_socket_connection_id", connection.ID).Error)

	for i := 0; i < 3; i++ {
		require.NoError(t, conn.DB().Create(&db.WebSocketMessage{
			ConnectionID: connection.ID,
			PayloadData:  fmt.Sprintf("message %d", i),
			Direction:    db.MessageSent,
			Timestamp:    time.Now(),
		}).Error)
	}

	issue, err := conn.CreateIssue(db.Issue{
		Code:        string(db.XssReflectedCode),
		Title:       "round trip issue",
		WorkspaceID: &workspace.ID,
		ScanID:      &scan.ID,
		URL:         histories[0].URL,
		Confidence:  90,
	})
	require.NoError(t, err)
	require.NoError(t, issue.AppendHistories([]*db.History{histories[0], histories[1]}))

	oobTest, err := conn.CreateOOBTest(db.OOBTest{
		Code:              db.XssReflectedCode,
		TestName:          "rt oob",
		Target:            "http://example.com",
		WorkspaceID:       &workspace.ID,
		HistoryID:         &histories[0].ID,
		IssueID:           &issue.ID,
		InteractionDomain: "rt.oob",
		InteractionFullID: code + "-oob",
	})
	require.NoError(t, err)

	_, err = conn.CreateInteraction(&db.OOBInteraction{
		OOBTestID:   &oobTest.ID,
		WorkspaceID: &workspace.ID,
		IssueID:     &issue.ID,
		Protocol:    "http",
		FullID:      code + "-int",
	})
	require.NoError(t, err)

	token := &db.JsonWebToken{
		Token:       "eyJhbGciOiJIUzI1NiJ9.e30.rt",
		Algorithm:   "HS256",
		WorkspaceID: &workspace.ID,
		Header:      datatypes.JSON([]byte(`{"alg":"HS256"}`)),
		Payload:     datatypes.JSON([]byte(`{"sub":"rt"}`)),
	}
	require.NoError(t, conn.DB().Create(token).Error)
	require.NoError(t, conn.DB().Exec(
		"INSERT INTO json_web_token_histories (json_web_token_id, history_id) VALUES (?, ?)",
		token.ID, histories[0].ID).Error)
	require.NoError(t, conn.DB().Exec(
		"INSERT INTO json_web_token_websocket_connections (json_web_token_id, web_socket_connection_id) VALUES (?, ?)",
		token.ID, connection.ID).Error)

	return seededWorkspace{Workspace: workspace, RawRequest: rawRequest, HistoryURL: histories[0].URL}
}

// tableCounts reports how many rows each registered table holds for a workspace,
// reusing the export scoping queries so source and copy are measured identically.
func tableCounts(t *testing.T, workspaceID uint) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64, len(orderedTables))
	for _, spec := range orderedTables {
		var n int64
		query := fmt.Sprintf("SELECT count(*) FROM (%s) AS sukyan_exported_row", spec.Query)
		require.NoError(t, db.Connection().DB().Raw(query, workspaceID).Scan(&n).Error)
		counts[spec.Name] = n
	}
	return counts
}

func exportToBuffer(t *testing.T, workspaceID uint) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	result, err := Export(context.Background(), db.Connection(), workspaceID, &buf, ExportOptions{})
	require.NoError(t, err)
	require.Positive(t, result.TotalRows)
	return &buf
}

func TestRoundTripPreservesEveryTable(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-source-"+uuid.NewString()[:8])
	source := seed.Workspace
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), source.ID, db.WorkspaceDeleteOptions{})
	})

	before := tableCounts(t, source.ID)
	archive := exportToBuffer(t, source.ID)

	imported, err := Import(context.Background(), db.Connection(), archive, ImportOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	require.NotEqual(t, source.ID, imported.WorkspaceID, "the copy must be a distinct workspace")
	after := tableCounts(t, imported.WorkspaceID)

	for _, spec := range orderedTables {
		expected := before[spec.Name]
		if spec.SkipImport {
			assert.Zero(t, after[spec.Name], "%s is excluded from import but rows appeared", spec.Name)
			continue
		}
		assert.Equal(t, expected, after[spec.Name], "row count mismatch for %s", spec.Name)
	}

	assert.Positive(t, before["histories"])
	assert.Positive(t, before["issue_requests"])
	assert.Positive(t, before["web_socket_messages"])
	assert.Positive(t, before["json_web_token_histories"])
}

func TestRoundTripPreservesBinaryAndJSONContent(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-binary-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	archive := exportToBuffer(t, seed.Workspace.ID)
	imported, err := Import(context.Background(), db.Connection(), archive, ImportOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	var copied db.History
	require.NoError(t, db.Connection().DB().
		Where("workspace_id = ? AND url = ?", imported.WorkspaceID, seed.HistoryURL).
		First(&copied).Error)
	assert.Equal(t, seed.RawRequest, copied.RawRequest, "raw request bytes must survive the round trip")

	var headers string
	require.NoError(t, db.Connection().DB().Raw(
		"SELECT request_headers::text FROM web_socket_connections WHERE workspace_id = ?",
		imported.WorkspaceID).Scan(&headers).Error)
	assert.Contains(t, headers, "http://example.com", "jsonb columns must survive the round trip")
}

// The cycle-breaking columns are withheld on first insert; this proves the
// second pass actually restores them rather than leaving them NULL.
func TestRoundTripRestoresDeferredForeignKeys(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-deferred-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	archive := exportToBuffer(t, seed.Workspace.ID)
	imported, err := Import(context.Background(), db.Connection(), archive, ImportOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	checks := []struct {
		description string
		query       string
	}{
		{"playground_sessions.original_request_id", `
			SELECT count(*) FROM playground_sessions s
			JOIN histories h ON h.id = s.original_request_id
			WHERE s.workspace_id = ? AND h.workspace_id = s.workspace_id`},
		{"api_definitions.source_history_id", `
			SELECT count(*) FROM api_definitions d
			JOIN histories h ON h.id = d.source_history_id
			WHERE d.workspace_id = ? AND h.workspace_id = d.workspace_id`},
		{"scan_jobs.history_id", `
			SELECT count(*) FROM scan_jobs j
			JOIN histories h ON h.id = j.history_id
			WHERE j.workspace_id = ? AND h.workspace_id = j.workspace_id`},
		{"scan_jobs.web_socket_connection_id", `
			SELECT count(*) FROM scan_jobs j
			JOIN web_socket_connections c ON c.id = j.web_socket_connection_id
			WHERE j.workspace_id = ? AND c.workspace_id = j.workspace_id`},
	}

	for _, check := range checks {
		var n int64
		require.NoError(t, db.Connection().DB().Raw(check.query, imported.WorkspaceID).Scan(&n).Error)
		assert.Positive(t, n, "%s was not restored, or points outside the imported workspace", check.description)
	}
}

// Every foreign key in the copy must resolve inside the copy. If the offset were
// applied inconsistently, a reference would silently point at the source
// workspace's rows instead.
func TestImportedWorkspaceReferencesNothingOutsideItself(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-isolation-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	archive := exportToBuffer(t, seed.Workspace.ID)
	imported, err := Import(context.Background(), db.Connection(), archive, ImportOptions{})
	require.NoError(t, err)

	crossReferences := []struct {
		description string
		query       string
	}{
		{"histories -> scans", `
			SELECT count(*) FROM histories h JOIN scans s ON s.id = h.scan_id
			WHERE h.workspace_id = ? AND s.workspace_id <> h.workspace_id`},
		{"issues -> histories", `
			SELECT count(*) FROM issues i
			JOIN issue_requests r ON r.issue_id = i.id
			JOIN histories h ON h.id = r.history_id
			WHERE i.workspace_id = ? AND h.workspace_id <> i.workspace_id`},
		{"web_socket_messages -> connections", `
			SELECT count(*) FROM web_socket_messages m
			JOIN web_socket_connections c ON c.id = m.connection_id
			WHERE c.workspace_id = ? AND c.workspace_id <> ?`},
		{"oob_interactions -> oob_tests", `
			SELECT count(*) FROM oob_interactions i
			JOIN oob_tests o ON o.id = i.oob_test_id
			WHERE i.workspace_id = ? AND o.workspace_id <> i.workspace_id`},
	}

	for _, check := range crossReferences {
		var n int64
		args := []any{imported.WorkspaceID}
		if check.description == "web_socket_messages -> connections" {
			args = append(args, imported.WorkspaceID)
		}
		require.NoError(t, db.Connection().DB().Raw(check.query, args...).Scan(&n).Error)
		assert.Zero(t, n, "%s leaks across workspaces", check.description)
	}

	// Deleting the copy must leave the source untouched.
	sourceBefore := tableCounts(t, seed.Workspace.ID)
	_, err = db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	require.NoError(t, err)
	assert.Equal(t, sourceBefore, tableCounts(t, seed.Workspace.ID), "deleting the copy changed the source")
}

func TestImportAssignsDistinctCodeAndHonoursOverrides(t *testing.T) {
	code := "rt-code-" + uuid.NewString()[:8]
	seed := seedRichWorkspace(t, code)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	archive := exportToBuffer(t, seed.Workspace.ID)
	imported, err := Import(context.Background(), db.Connection(), archive, ImportOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})
	assert.NotEqual(t, code, imported.Code, "importing beside the source must not reuse its code")

	archive2 := exportToBuffer(t, seed.Workspace.ID)
	renamed, err := Import(context.Background(), db.Connection(), archive2, ImportOptions{
		Code:  "rt-explicit-" + uuid.NewString()[:8],
		Title: "Renamed copy",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), renamed.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	loaded, err := db.Connection().GetWorkspaceByID(renamed.WorkspaceID)
	require.NoError(t, err)
	assert.Equal(t, "Renamed copy", loaded.Title)
	assert.Equal(t, renamed.Code, loaded.Code)
}

func TestImportRejectsUnknownFormatVersion(t *testing.T) {
	var buf bytes.Buffer
	writer, err := newArchiveWriter(&buf, Manifest{FormatVersion: ArchiveFormatVersion + 1})
	require.NoError(t, err)
	require.NoError(t, writer.close(Summary{}))

	_, err = Import(context.Background(), db.Connection(), &buf, ImportOptions{})
	require.ErrorContains(t, err, "unsupported archive format version")
}

func TestImportDetectsTruncatedArchive(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-truncated-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	archive := exportToBuffer(t, seed.Workspace.ID)
	truncated := archive.Bytes()[:archive.Len()/2]

	_, err := Import(context.Background(), db.Connection(), bytes.NewReader(truncated), ImportOptions{})
	require.Error(t, err, "a truncated archive must not import silently")
}
