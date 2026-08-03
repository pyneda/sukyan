package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// craftArchive builds an archive by hand so a test can put values in it that
// Export would never produce.
func craftArchive(t *testing.T, rows []record) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	writer, err := newArchiveWriter(&buf, Manifest{
		FormatVersion: ArchiveFormatVersion,
		Workspace:     WorkspaceInfo{Code: "hostile", Title: "hostile"},
	})
	require.NoError(t, err)

	counts := make(map[string]int64)
	for _, row := range rows {
		require.NoError(t, writer.writeRow(row.Table, row.Row))
		counts[row.Table]++
	}

	var total int64
	for _, n := range counts {
		total += n
	}
	require.NoError(t, writer.close(Summary{RowsByTable: counts, TotalRows: total}))
	return &buf
}

// An archive is fully attacker-controlled input. Containment of its rows must
// not depend on arithmetic over values the attacker chose: a negative
// workspace_id cancels the import offset exactly and would otherwise drop rows
// into an unrelated, existing workspace.
func TestImportRejectsRowsAimedAtAnotherWorkspace(t *testing.T) {
	conn := db.Connection()

	victim, err := conn.GetOrCreateWorkspace(&db.Workspace{
		Code:  "hostile-victim-" + uuid.NewString()[:8],
		Title: "victim",
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		conn.DeleteWorkspaceCascade(context.Background(), victim.ID, db.WorkspaceDeleteOptions{})
	})

	_, floors, err := planIdentifierOffsets(context.Background(), conn, nil)
	require.NoError(t, err)

	// The crafted archive declares no identifier base, so histories are shifted
	// by their table's floor. This value resolves to exactly victim.ID.
	aimed := int64(victim.ID) - floors["histories"]

	archive := craftArchive(t, []record{
		{Table: "workspaces", Row: json.RawMessage(`{"id":1,"code":"hostile","title":"hostile"}`)},
		{Table: "histories", Row: json.RawMessage(fmt.Sprintf(
			`{"id":1,"url":"http://attacker.example/pwned","workspace_id":%d,"status_code":200}`, aimed))},
	})

	result, err := Import(context.Background(), conn, archive, ImportOptions{})
	if err == nil {
		t.Cleanup(func() {
			conn.DeleteWorkspaceCascade(context.Background(), result.WorkspaceID, db.WorkspaceDeleteOptions{})
		})
	}

	var landed int64
	require.NoError(t, conn.DB().Raw(
		"SELECT count(*) FROM histories WHERE workspace_id = ?", victim.ID).Scan(&landed).Error)
	assert.Zero(t, landed, "a crafted archive placed rows inside an unrelated workspace")
}

// Identifiers in an archive come from bigserial columns and are always
// positive. Anything else means the archive was tampered with.
func TestImportRejectsNonPositiveIdentifiers(t *testing.T) {
	archive := craftArchive(t, []record{
		{Table: "workspaces", Row: json.RawMessage(`{"id":1,"code":"hostile","title":"hostile"}`)},
		{Table: "scans", Row: json.RawMessage(`{"id":-5,"workspace_id":1,"status":"completed"}`)},
	})

	_, err := Import(context.Background(), db.Connection(), archive, ImportOptions{})
	require.Error(t, err, "a negative identifier must be rejected")
	assert.Contains(t, err.Error(), "identifier")
}

// Importing into the same database repeatedly must not compound the identifier
// space. Anchoring the offset at zero adds the archive's highest identifier to
// the target's, doubling the maximum every time and exhausting the bigint range
// within a few dozen imports.
func TestRepeatedImportsDoNotInflateIdentifierSpace(t *testing.T) {
	conn := db.Connection()
	seed := seedRichWorkspace(t, "rt-growth-"+uuid.NewString()[:8])
	t.Cleanup(func() {
		conn.DeleteWorkspaceCascade(context.Background(), seed.Workspace.ID, db.WorkspaceDeleteOptions{})
	})

	_, floorsBefore, err := planIdentifierOffsets(context.Background(), conn, nil)
	require.NoError(t, err)
	before := floorsBefore["histories"]

	for i := 0; i < 3; i++ {
		archive := exportToBuffer(t, seed.Workspace.ID)
		imported, err := Import(context.Background(), conn, archive, ImportOptions{})
		require.NoError(t, err)
		t.Cleanup(func() {
			conn.DeleteWorkspaceCascade(context.Background(), imported.WorkspaceID, db.WorkspaceDeleteOptions{})
		})
	}

	_, floorsAfter, err := planIdentifierOffsets(context.Background(), conn, nil)
	require.NoError(t, err)
	after := floorsAfter["histories"]

	// Three copies of a workspace holding a few dozen rows should consume a few
	// hundred identifiers, not multiply the ceiling.
	growth := after - before
	assert.Less(t, growth, int64(100_000),
		"three imports raised the highest identifier by %d; the offset is compounding", growth)
}

// A value near the top of the int64 range would wrap to a negative identifier
// once the offset is added.
func TestImportRejectsIdentifierOverflow(t *testing.T) {
	archive := craftArchive(t, []record{
		{Table: "workspaces", Row: json.RawMessage(`{"id":1,"code":"hostile","title":"hostile"}`)},
		{Table: "scans", Row: json.RawMessage(`{"id":9223372036854775807,"workspace_id":1,"status":"completed"}`)},
	})

	_, err := Import(context.Background(), db.Connection(), archive, ImportOptions{})
	require.Error(t, err, "an identifier that overflows when shifted must be rejected")
}
