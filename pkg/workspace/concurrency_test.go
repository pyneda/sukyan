package workspace

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeHistoryThroughTheSequence inserts the way a live scan does: no explicit
// identifier, so the value comes from nextval. It returns the identifier the
// allocator handed out.
func writeHistoryThroughTheSequence(t *testing.T, workspaceID uint) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Connection().DB().Raw(
		`INSERT INTO histories (url, clean_url, method, status_code, source, workspace_id, created_at, updated_at)
		 VALUES ('http://example.com/concurrent-writer', 'http://example.com/concurrent-writer',
		         'GET', 200, 'scanner', ?, now(), now())
		 RETURNING id`, workspaceID).Scan(&id).Error)
	return id
}

func historyIdentifierFloor(t *testing.T) int64 {
	t.Helper()
	var floor int64
	require.NoError(t, db.Connection().DB().
		Raw("SELECT COALESCE(MAX(id), 0) + 1 FROM histories").Scan(&floor).Error)
	return floor
}

// TestImportBlockIsReservedAgainstConcurrentWriters is the regression test for
// the primary key collision seen importing a 49k-history workspace on a
// deployment with scans running.
//
// The writer is fired from the progress callback, which runs after the offsets
// have been planned and before the first histories batch is inserted -- the
// exact window a scan would write into. It allocates through nextval rather than
// at an explicit id, because the claim under test is that the sequence still
// points inside the range the import is about to occupy.
func TestImportBlockIsReservedAgainstConcurrentWriters(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-concurrent-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	archive := exportToBuffer(t, seed.Workspace.ID).Bytes()

	var allocated int64
	imported, err := Import(context.Background(), db.Connection(), bytes.NewReader(archive), ImportOptions{
		Progress: func(table string, rows int64) {
			if table != "histories" || rows != 0 || allocated != 0 {
				return
			}
			allocated = writeHistoryThroughTheSequence(t, seed.Workspace.ID)
		},
	})
	if err == nil {
		t.Cleanup(func() {
			db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
		})
	}
	require.NotZero(t, allocated, "the progress hook never fired for histories")
	require.NoError(t, err, "a scan writing through nextval collided with the import")

	// Stated directly rather than inferred from the import succeeding: the
	// identifier the allocator handed the writer must not be one the import
	// went on to use.
	var belongsToImport int64
	require.NoError(t, db.Connection().DB().Raw(
		"SELECT count(*) FROM histories WHERE id = ? AND workspace_id = ?",
		allocated, imported.WorkspaceID).Scan(&belongsToImport).Error)
	assert.Zero(t, belongsToImport, "identifier %d was handed to a writer and also claimed by the import", allocated)
}

// TestExportIsConsistentWhileTheWorkspaceIsWrittenTo covers the other half of
// the concurrency: a scan writing into the workspace being exported.
//
// The manifest's identifier bounds are read before the rows are, so without a
// single snapshot across the whole export a row written in between lands in the
// archive above the span it declares, and the import rejects it with
// "above the last reserved histories identifier". The writer fires once the
// first table has been exported, which is after the bounds were read and well
// before histories is reached -- export's progress callback runs after each
// table, so keying it on histories itself would fire too late to matter.
func TestExportIsConsistentWhileTheWorkspaceIsWrittenTo(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-export-race-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	var archive bytes.Buffer
	var wrote int64
	_, err := Export(context.Background(), db.Connection(), seed.Workspace.ID, &archive, ExportOptions{
		Progress: func(table string, rows int64) {
			if table != "workspaces" || wrote != 0 {
				return
			}
			wrote = writeHistoryThroughTheSequence(t, seed.Workspace.ID)
		},
	})
	require.NoError(t, err)
	require.NotZero(t, wrote, "no row was written during the export")

	imported, err := Import(context.Background(), db.Connection(), bytes.NewReader(archive.Bytes()), ImportOptions{})
	require.NoError(t, err, "a workspace written to mid-export produced an archive that will not import")
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})
}

// TestImportSucceedsOnTheSameBytesWithoutAWriter is the control half. It proves
// any failure above is the concurrency, not the archive.
func TestImportSucceedsOnTheSameBytesWithoutAWriter(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-control-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	archive := exportToBuffer(t, seed.Workspace.ID).Bytes()

	first, err := Import(context.Background(), db.Connection(), bytes.NewReader(archive), ImportOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), first.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	second, err := Import(context.Background(), db.Connection(), bytes.NewReader(archive), ImportOptions{})
	require.NoError(t, err, "the same bytes must import twice")
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), second.WorkspaceID, db.WorkspaceDeleteOptions{})
	})
}

// TestSequenceIsAdvancedBeforeRowsAreInserted states the invariant that makes
// the above safe, independently of whether a writer happens to collide: by the
// time any row is inserted, the sequence must already be past everything the
// import will write.
func TestSequenceIsAdvancedBeforeRowsAreInserted(t *testing.T) {
	seed := seedRichWorkspace(t, "rt-reserve-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	floorBefore := historyIdentifierFloor(t)
	archive := exportToBuffer(t, seed.Workspace.ID).Bytes()

	var sequenceAtFirstRow int64
	imported, err := Import(context.Background(), db.Connection(), bytes.NewReader(archive), ImportOptions{
		Progress: func(table string, rows int64) {
			if table != "histories" || rows != 0 || sequenceAtFirstRow != 0 {
				return
			}
			require.NoError(t, db.Connection().DB().
				Raw("SELECT last_value FROM histories_id_seq").Scan(&sequenceAtFirstRow).Error)
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		db.Connection().DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
	})

	assert.GreaterOrEqual(t, sequenceAtFirstRow, floorBefore,
		"the sequence was still at %d when the first histories row was inserted; the import claims identifiers from %d upward",
		sequenceAtFirstRow, floorBefore)
}
