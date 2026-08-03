package workspace

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// ExportOptions configures an export.
type ExportOptions struct {
	// Progress, when set, is called after each table with the rows written.
	Progress func(table string, rows int64)
}

// ExportResult reports what an export wrote.
type ExportResult struct {
	Workspace   WorkspaceInfo    `json:"workspace"`
	RowsByTable map[string]int64 `json:"rows_by_table"`
	TotalRows   int64            `json:"total_rows"`
	Duration    time.Duration    `json:"duration"`
}

// Export writes a portable archive of a workspace to w.
//
// Rows are streamed straight out of Postgres one at a time and serialised by
// the database itself through row_to_json, so bytea, jsonb, uuid and timestamp
// columns keep full fidelity without any Go-side type mapping. Nothing larger
// than a single row is ever held in memory, which is what makes a 27 GB
// workspace exportable.
func Export(ctx context.Context, conn *db.DatabaseConnection, workspaceID uint, w io.Writer, opts ExportOptions) (*ExportResult, error) {
	source, err := conn.GetWorkspaceByID(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("loading workspace %d: %w", workspaceID, err)
	}

	started := time.Now()
	info := WorkspaceInfo{
		ID:          source.ID,
		Code:        source.Code,
		Title:       source.Title,
		Description: source.Description,
	}

	bases, err := lowestIdentifiers(ctx, conn, workspaceID)
	if err != nil {
		return nil, err
	}

	manifest := Manifest{
		FormatVersion:   ArchiveFormatVersion,
		CreatedAt:       started.UTC(),
		Workspace:       info,
		IdentifierBases: bases,
		Excluded:        excludedTables(),
	}

	writer, err := newArchiveWriter(w, manifest)
	if err != nil {
		return nil, err
	}

	result := &ExportResult{Workspace: info, RowsByTable: make(map[string]int64, len(orderedTables))}

	for _, spec := range orderedTables {
		rows, err := exportTable(ctx, conn, spec, workspaceID, writer)
		if err != nil {
			return nil, err
		}
		result.RowsByTable[spec.Name] = rows
		result.TotalRows += rows
		if opts.Progress != nil {
			opts.Progress(spec.Name, rows)
		}
	}

	if err := writer.close(Summary{RowsByTable: result.RowsByTable, TotalRows: result.TotalRows}); err != nil {
		return nil, err
	}

	result.Duration = time.Since(started)
	log.Info().
		Uint("workspace", workspaceID).
		Int64("rows", result.TotalRows).
		Dur("duration", result.Duration).
		Msg("Workspace exported")

	return result, nil
}

func exportTable(ctx context.Context, conn *db.DatabaseConnection, spec tableSpec, workspaceID uint, writer *archiveWriter) (int64, error) {
	// The alias must not collide with any column name in the selected table, or
	// row_to_json resolves to the column instead of the row (histories.source
	// is the case that catches naive aliases).
	query := fmt.Sprintf("SELECT row_to_json(sukyan_exported_row) FROM (%s) AS sukyan_exported_row", spec.Query)

	rows, err := conn.DB().WithContext(ctx).Raw(query, workspaceID).Rows()
	if err != nil {
		return 0, fmt.Errorf("selecting %s rows: %w", spec.Name, err)
	}
	defer rows.Close()

	var written int64
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return written, fmt.Errorf("scanning %s row: %w", spec.Name, err)
		}
		if err := writer.writeRow(spec.Name, encoded); err != nil {
			return written, fmt.Errorf("writing %s row: %w", spec.Name, err)
		}
		written++
	}
	if err := rows.Err(); err != nil {
		return written, fmt.Errorf("iterating %s rows: %w", spec.Name, err)
	}
	return written, nil
}

// lowestIdentifiers records, per table, the smallest bigint row identifier the
// export will contain. Import anchors each table's offset to its own base, which
// keeps the identifier space growing by the size of the archive rather than by
// the size of the database it is imported into.
func lowestIdentifiers(ctx context.Context, conn *db.DatabaseConnection, workspaceID uint) (map[string]int64, error) {
	bases := make(map[string]int64, len(orderedTables))
	for _, spec := range orderedTables {
		if !spec.hasBigintPrimaryKey() {
			continue
		}
		var lowest *int64
		query := fmt.Sprintf("SELECT MIN(id) FROM (%s) AS sukyan_exported_row", spec.Query)
		if err := conn.DB().WithContext(ctx).Raw(query, workspaceID).Scan(&lowest).Error; err != nil {
			return nil, fmt.Errorf("reading lowest id of %s: %w", spec.Name, err)
		}
		if lowest != nil {
			bases[spec.Name] = *lowest
		}
	}
	return bases, nil
}

func excludedTables() []ExcludedTable {
	var excluded []ExcludedTable
	for _, spec := range orderedTables {
		if spec.SkipImport {
			excluded = append(excluded, ExcludedTable{Table: spec.Name, Reason: spec.SkipReason})
		}
	}
	return excluded
}
