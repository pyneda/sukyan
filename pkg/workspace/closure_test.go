package workspace

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// danglingWorkspace is a workspace carrying one of every shape of reference that
// points outside itself. All three occur in real data:
//
//   - a globally shared parent: json_web_tokens is deduplicated by signature, so
//     one token row is reused by every workspace that ever saw it and its link
//     rows span workspaces;
//   - a relationally owned row with no workspace_id, which WHERE workspace_id =
//     $1 cannot see while a join still pulls in its children;
//   - a plain cross-workspace reference between two workspace-scoped tables.
type danglingWorkspace struct {
	Source          *db.Workspace
	Foreign         *db.Workspace
	HistoryWithTask uint
	FuzzIteration   uint
}

func seedDanglingWorkspace(t *testing.T, code string) danglingWorkspace {
	t.Helper()
	conn := db.Connection()
	seed := seedRichWorkspace(t, code)

	foreign, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Code:  code + "-foreign",
		Title: "Not part of the export",
	})
	require.NoError(t, err)

	foreignHistory := &db.History{
		URL: "http://elsewhere.example/foreign", CleanURL: "http://elsewhere.example/foreign",
		StatusCode: 200, Method: "GET", WorkspaceID: &foreign.ID, Source: db.SourceScanner,
	}
	created, err := conn.CreateHistory(foreignHistory)
	require.NoError(t, err)

	foreignTask := &db.Task{Title: "foreign task", WorkspaceID: foreign.ID, Status: "finished"}
	require.NoError(t, conn.DB().Create(foreignTask).Error)

	// (1) The source's JWT also links to a history in the other workspace.
	var tokenID uint
	require.NoError(t, conn.DB().Raw(
		"SELECT id FROM json_web_tokens WHERE workspace_id = ?", seed.Workspace.ID).Scan(&tokenID).Error)
	require.NotZero(t, tokenID)
	require.NoError(t, conn.DB().Exec(
		"INSERT INTO json_web_token_histories (json_web_token_id, history_id) VALUES (?, ?)",
		tokenID, created.ID).Error)

	// (2) A history in the source points at a task in the other workspace.
	var historyWithTask uint
	require.NoError(t, conn.DB().Raw(
		"SELECT id FROM histories WHERE workspace_id = ? ORDER BY id LIMIT 1", seed.Workspace.ID).
		Scan(&historyWithTask).Error)
	require.NoError(t, conn.DB().Exec(
		"UPDATE histories SET task_id = ? WHERE id = ?", foreignTask.ID, historyWithTask).Error)

	// (3) A fuzz iteration owned by the source through its playground session,
	// referencing a connection that carries no workspace_id at all.
	var orphanConnection uint
	require.NoError(t, conn.DB().Raw(
		`INSERT INTO web_socket_connections (url, source, created_at, updated_at)
		 VALUES ('ws://elsewhere.example/orphan', 'scanner', now(), now()) RETURNING id`).
		Scan(&orphanConnection).Error)

	var sessionID uint
	require.NoError(t, conn.DB().Raw(
		"SELECT id FROM playground_sessions WHERE workspace_id = ?", seed.Workspace.ID).Scan(&sessionID).Error)
	require.NotZero(t, sessionID)

	var runID uint
	require.NoError(t, conn.DB().Raw(
		"INSERT INTO playground_ws_fuzz_runs (session_id) VALUES (?) RETURNING id", sessionID).
		Scan(&runID).Error)
	var iterationID uint
	require.NoError(t, conn.DB().Raw(
		"INSERT INTO playground_ws_fuzz_iterations (run_id, web_socket_connection_id) VALUES (?, ?) RETURNING id",
		runID, orphanConnection).Scan(&iterationID).Error)

	t.Cleanup(func() {
		conn.DB().Exec("DELETE FROM web_socket_connections WHERE id = ?", orphanConnection)
		conn.DeleteWorkspaceCascade(context.Background(), foreign.ID, db.WorkspaceDeleteOptions{})
		conn.DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	return danglingWorkspace{
		Source: seed.Workspace, Foreign: foreign,
		HistoryWithTask: historyWithTask, FuzzIteration: iterationID,
	}
}

// TestExportOfAWorkspaceWithDanglingReferencesImports is the regression test for
// the shift validation failure. Before the archive was closed under its own
// references, exporting this workspace produced bytes that could not be imported
// back into the database they came from.
func TestExportOfAWorkspaceWithDanglingReferencesImports(t *testing.T) {
	seed := seedDanglingWorkspace(t, "rt-dangling-"+uuid.NewString()[:8])
	conn := db.Connection()

	var archive bytes.Buffer
	exported, err := Export(context.Background(), conn, seed.Source.ID, &archive, ExportOptions{})
	require.NoError(t, err)

	assert.Equal(t, int64(1), exported.DroppedRows["json_web_token_histories"],
		"the link row pointing at another workspace's history should have been left out and reported")

	imported, err := Import(context.Background(), conn, bytes.NewReader(archive.Bytes()), ImportOptions{})
	require.NoError(t, err, "an archive of a workspace with dangling references must still import")
	t.Cleanup(func() {
		conn.DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	// The optional references survived as NULL rather than aimed at a row in the
	// workspace they were never part of.
	var taskID *uint
	require.NoError(t, conn.DB().Raw(
		"SELECT task_id FROM histories WHERE workspace_id = ? AND task_id IS NOT NULL LIMIT 1",
		imported.WorkspaceID).Scan(&taskID).Error)
	if taskID != nil {
		var owner uint
		require.NoError(t, conn.DB().Raw("SELECT workspace_id FROM tasks WHERE id = ?", *taskID).Scan(&owner).Error)
		assert.Equal(t, imported.WorkspaceID, owner, "an imported history points at a task in another workspace")
	}

	var orphanIterations int64
	require.NoError(t, conn.DB().Raw(`
		SELECT count(*) FROM playground_ws_fuzz_iterations i
		JOIN playground_ws_fuzz_runs r ON r.id = i.run_id
		JOIN playground_sessions s ON s.id = r.session_id
		LEFT JOIN web_socket_connections c ON c.id = i.web_socket_connection_id AND c.workspace_id = s.workspace_id
		WHERE s.workspace_id = ? AND i.web_socket_connection_id IS NOT NULL AND c.id IS NULL`,
		imported.WorkspaceID).Scan(&orphanIterations).Error)
	assert.Zero(t, orphanIterations, "an imported fuzz iteration references a connection outside the workspace")
}

// TestImportedWorkspaceResolvesEveryDeclaredReference is the general form of the
// invariant, checked over every registered table rather than a hand-picked few.
// Whichever table happens to carry a cross-workspace reference first, this
// catches it.
func TestImportedWorkspaceResolvesEveryDeclaredReference(t *testing.T) {
	seed := seedDanglingWorkspace(t, "rt-closure-"+uuid.NewString()[:8])
	conn := db.Connection()

	archive := exportToBuffer(t, seed.Source.ID)
	imported, err := Import(context.Background(), conn, archive, ImportOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	for _, spec := range orderedTables {
		if spec.SkipImport {
			continue
		}
		for column, targetName := range spec.IDColumns {
			if column == "id" {
				continue
			}
			target, ok := tableByName(targetName)
			if !ok || !target.hasBigintPrimaryKey() || target.SkipImport {
				continue
			}
			query := fmt.Sprintf(`
				SELECT count(*) FROM (%s) AS src
				LEFT JOIN (%s) AS tgt ON tgt.id = src.%s
				WHERE src.%s IS NOT NULL AND tgt.id IS NULL`,
				spec.selectionQuery(), target.selectionQuery(), column, column)

			var dangling int64
			require.NoError(t, conn.DB().Raw(query, imported.WorkspaceID).Scan(&dangling).Error,
				"closure probe for %s.%s failed to run", spec.Name, column)
			assert.Zero(t, dangling,
				"%s.%s points outside the imported workspace in %d rows", spec.Name, column, dangling)
		}
	}
}

// The declarations drive which rows are dropped rather than degraded, so a
// column that changes nullability has to fail here rather than at an operator's
// import.
func TestContainedReferencesMatchLiveSchema(t *testing.T) {
	for _, spec := range orderedTables {
		for _, ref := range spec.Contained {
			target, ok := spec.IDColumns[ref.Column]
			assert.True(t, ok, "%s declares containment for %s, which is not an IDColumn", spec.Name, ref.Column)
			assert.NotEmpty(t, target)

			var nullable string
			require.NoError(t, db.Connection().DB().Raw(`
				SELECT is_nullable FROM information_schema.columns
				WHERE table_schema = 'public' AND table_name = ? AND column_name = ?`,
				spec.Name, ref.Column).Scan(&nullable).Error)
			require.NotEmpty(t, nullable, "%s.%s does not exist", spec.Name, ref.Column)

			assert.Equal(t, nullable == "NO", ref.Required,
				"%s.%s is_nullable=%s but Required=%v; a NOT NULL reference has to drop its row, a nullable one is written NULL",
				spec.Name, ref.Column, nullable, ref.Required)
		}
	}
}

// Required references are resolved by nesting the target's own selection, so a
// cycle would recurse forever while building the query.
func TestRequiredReferenceGraphIsAcyclic(t *testing.T) {
	var visit func(spec tableSpec, seen []string)
	visit = func(spec tableSpec, seen []string) {
		for _, name := range seen {
			require.NotEqual(t, name, spec.Name, "required reference cycle through %s: %v", spec.Name, seen)
		}
		required, _ := spec.partitionContained()
		for _, ref := range required {
			if target, ok := tableByName(spec.IDColumns[ref.Column]); ok {
				visit(target, append(seen, spec.Name))
			}
		}
	}
	for _, spec := range orderedTables {
		visit(spec, nil)
	}
}
