package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
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
	// DroppedRows counts, per table, rows left out because a NOT NULL reference
	// pointed at a row outside the workspace. Such a row cannot be written with
	// the reference degraded to NULL, so it cannot travel; reporting it keeps
	// that from being silent.
	DroppedRows map[string]int64 `json:"dropped_rows,omitempty"`
	Duration    time.Duration    `json:"duration"`
}

// Export writes a portable archive of a workspace to w.
//
// Rows are streamed straight out of Postgres one at a time and serialised by
// the database itself through to_jsonb, so bytea, jsonb, uuid and timestamp
// columns keep full fidelity without any Go-side type mapping. Nothing larger
// than a single row is ever held in memory, which is what makes a 27 GB
// workspace exportable.
//
// Everything that makes up the archive is read inside one repeatable read
// snapshot, because the manifest's identifier bounds are read before the rows
// are: a row written in between would exceed the span import reserves for it,
// and a row exported from one table could reference a row a later query has not
// seen yet. The cost is a long-lived snapshot -- an export of tens of gigabytes
// holds one for as long as it runs, holding vacuum back by the same amount.
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
	result := &ExportResult{Workspace: info, RowsByTable: make(map[string]int64, len(orderedTables))}

	err = conn.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bases, ceilings, err := identifierBounds(ctx, tx, workspaceID)
		if err != nil {
			return err
		}

		writer, err := newArchiveWriter(w, Manifest{
			FormatVersion:      ArchiveFormatVersion,
			CreatedAt:          started.UTC(),
			Workspace:          info,
			IdentifierBases:    bases,
			IdentifierCeilings: ceilings,
			Excluded:           excludedTables(),
		})
		if err != nil {
			return err
		}

		for _, spec := range orderedTables {
			rows, err := exportTable(ctx, tx, spec, workspaceID, writer)
			if err != nil {
				return err
			}
			result.RowsByTable[spec.Name] = rows
			result.TotalRows += rows

			dropped, err := countDroppedRows(ctx, tx, spec, workspaceID, rows)
			if err != nil {
				return err
			}
			if dropped > 0 {
				if result.DroppedRows == nil {
					result.DroppedRows = make(map[string]int64)
				}
				result.DroppedRows[spec.Name] = dropped
				log.Warn().
					Uint("workspace", workspaceID).
					Str("table", spec.Name).
					Int64("rows", dropped).
					Msg("Rows left out of the archive: a required reference points outside the workspace")
			}
			if opts.Progress != nil {
				opts.Progress(spec.Name, rows)
			}
		}

		return writer.close(Summary{RowsByTable: result.RowsByTable, TotalRows: result.TotalRows})
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
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

// countDroppedRows reports how many rows the spec's own Query returns that the
// archive leaves out, which is only ever the rows whose required reference falls
// outside the workspace. Skipped for specs that cannot drop anything, so the
// common case costs nothing.
func countDroppedRows(ctx context.Context, tx *gorm.DB, spec tableSpec, workspaceID uint, written int64) (int64, error) {
	required, _ := spec.partitionContained()
	if len(required) == 0 {
		return 0, nil
	}
	var total int64
	query := fmt.Sprintf("SELECT count(*) FROM (%s) AS %s", spec.Query, exportRowAlias)
	if err := tx.WithContext(ctx).Raw(query, workspaceID).Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("counting %s rows: %w", spec.Name, err)
	}
	return total - written, nil
}

func exportTable(ctx context.Context, tx *gorm.DB, spec tableSpec, workspaceID uint, writer *archiveWriter) (int64, error) {
	rows, err := tx.WithContext(ctx).Raw(spec.exportQuery(), workspaceID).Rows()
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

// identifierBounds records, per table, the smallest and largest bigint row
// identifier the export will contain. Import anchors each table's offset to the
// base and reserves base..ceiling worth of identifiers before it writes.
func identifierBounds(ctx context.Context, tx *gorm.DB, workspaceID uint) (bases, ceilings map[string]int64, err error) {
	bases = make(map[string]int64, len(orderedTables))
	ceilings = make(map[string]int64, len(orderedTables))
	for _, spec := range orderedTables {
		if !spec.hasBigintPrimaryKey() {
			continue
		}
		var bounds struct {
			Lowest  *int64
			Highest *int64
		}
		query := fmt.Sprintf(
			"SELECT MIN(id) AS lowest, MAX(id) AS highest FROM (%s) AS sukyan_exported_row",
			spec.selectionQuery())
		if err := tx.WithContext(ctx).Raw(query, workspaceID).Scan(&bounds).Error; err != nil {
			return nil, nil, fmt.Errorf("reading identifier bounds of %s: %w", spec.Name, err)
		}
		if bounds.Lowest != nil {
			bases[spec.Name] = *bounds.Lowest
		}
		if bounds.Highest != nil {
			ceilings[spec.Name] = *bounds.Highest
		}
	}
	return bases, ceilings, nil
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
