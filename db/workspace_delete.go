package db

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
)

// DefaultWorkspaceDeleteBatchSize bounds how many rows a single delete
// statement removes. Every batch is its own transaction, so this also bounds
// WAL growth and how much work is lost when a delete is interrupted.
const DefaultWorkspaceDeleteBatchSize = 5000

// workspaceBulkTables are the tables large enough that deleting them through
// the workspace cascade would produce an unbounded transaction. They are
// emptied in batches first; the remaining tables are small and are left to the
// final cascade.
//
// web_socket_messages is listed before web_socket_connections because a batch
// bounds only the rows it matches directly: rows removed by cascade ride along
// in the same transaction.
var workspaceBulkTables = []workspaceBulkTable{
	{Name: "web_socket_messages", Scope: `connection_id IN (SELECT id FROM web_socket_connections WHERE workspace_id = ?)`},
	{Name: "histories", Scope: `workspace_id = ?`},
	{Name: "web_socket_connections", Scope: `workspace_id = ?`},
	{Name: "oob_interactions", Scope: `workspace_id = ?`},
	{Name: "oob_tests", Scope: `workspace_id = ?`},
	{Name: "issues", Scope: `workspace_id = ?`},
}

// workspaceBulkTable names a table and the predicate matching its rows to a
// workspace. Not every bulk table carries workspace_id; web_socket_messages is
// reached through its connection.
type workspaceBulkTable struct {
	Name  string
	Scope string
}

// WorkspaceDeleteOptions configures a batched workspace delete.
type WorkspaceDeleteOptions struct {
	// BatchSize is the number of rows removed per transaction. Zero selects
	// DefaultWorkspaceDeleteBatchSize.
	BatchSize int
	// Progress, when set, is called after each batch with the cumulative count
	// for that table.
	Progress func(table string, deleted int64)
}

// WorkspaceDeleteResult reports what a delete removed. The counts cover the
// rows deleted explicitly in batches; rows removed by the database cascade
// behind them are not tallied.
type WorkspaceDeleteResult struct {
	RowsByTable    map[string]int64 `json:"rows_by_table"`
	TotalRows      int64            `json:"total_rows"`
	ScansCancelled int64            `json:"scans_cancelled"`
	Duration       time.Duration    `json:"duration"`
}

// DeleteWorkspaceCascade removes a workspace and every row that belongs to it.
//
// The bulk tables are emptied in bounded batches, each in its own transaction,
// before the workspace row itself is deleted and the remaining (small) tables
// are removed by the database cascade. This keeps WAL growth bounded, lets the
// caller cancel through ctx, and makes an interrupted delete resumable: the
// workspace is left partially emptied and re-running finishes the job.
func (d *DatabaseConnection) DeleteWorkspaceCascade(ctx context.Context, workspaceID uint, opts WorkspaceDeleteOptions) (*WorkspaceDeleteResult, error) {
	// Callers report progress from the result even when an error comes back, so
	// it is never nil.
	started := time.Now()
	result := &WorkspaceDeleteResult{RowsByTable: make(map[string]int64, len(workspaceBulkTables)+1)}

	if workspaceID == 0 {
		return result, fmt.Errorf("a workspace ID is required")
	}
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultWorkspaceDeleteBatchSize
	}

	cancelled, err := d.cancelWorkspaceScans(ctx, workspaceID)
	if err != nil {
		return result, err
	}
	result.ScansCancelled = cancelled

	if err := d.detachHistoryScanJobCycle(ctx, workspaceID, batchSize); err != nil {
		return result, err
	}

	for _, table := range workspaceBulkTables {
		deleted, err := d.deleteWorkspaceTableInBatches(ctx, table, workspaceID, batchSize, opts.Progress)
		result.RowsByTable[table.Name] = deleted
		result.TotalRows += deleted
		if err != nil {
			return result, err
		}
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}

	tx := d.db.WithContext(ctx).Exec("DELETE FROM workspaces WHERE id = ?", workspaceID)
	if tx.Error != nil {
		log.Error().Err(tx.Error).Uint("workspace", workspaceID).Msg("Failed to delete workspace row")
		return result, fmt.Errorf("deleting workspace %d: %w", workspaceID, tx.Error)
	}
	result.RowsByTable["workspaces"] = tx.RowsAffected
	result.TotalRows += tx.RowsAffected
	result.Duration = time.Since(started)

	log.Info().
		Uint("workspace", workspaceID).
		Int64("rows", result.TotalRows).
		Dur("duration", result.Duration).
		Msg("Workspace deleted")

	return result, nil
}

// MarkWorkspaceForDeletion soft-deletes the workspace row so it disappears from
// every listing straight away, while the rows behind it are still being purged.
// Purging a large workspace takes minutes, and leaving it visible and shrinking
// for that long is worse than hiding it up front.
func (d *DatabaseConnection) MarkWorkspaceForDeletion(workspaceID uint) error {
	result := d.db.Where("id = ?", workspaceID).Delete(&Workspace{})
	if result.Error != nil {
		return fmt.Errorf("marking workspace %d for deletion: %w", workspaceID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workspace %d not found", workspaceID)
	}
	return nil
}

// WorkspacesPendingPurge lists workspaces that were marked for deletion but
// whose rows were never fully removed, which is what an interrupted purge leaves
// behind. Re-running the delete for each finishes the job.
//
// Note that a workspace soft-deleted by any other means shows up here too, so
// callers must treat the result as a proposal to confirm rather than a worklist
// to run unattended.
func (d *DatabaseConnection) WorkspacesPendingPurge() ([]Workspace, error) {
	var workspaces []Workspace
	if err := d.db.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("id").
		Find(&workspaces).Error; err != nil {
		return nil, fmt.Errorf("listing workspaces pending purge: %w", err)
	}
	return workspaces, nil
}

// detachHistoryScanJobCycle clears scan_jobs.history_id ahead of the deletes.
//
// histories and scan_jobs reference each other with ON DELETE CASCADE, so
// removing one history takes its scan job and, with it, every other history of
// that job -- measured at ~550 histories per scan job. Left in place, a single
// bounded batch cascades into most of the workspace inside one transaction,
// which is exactly what batching is here to prevent. Clearing the back
// reference first costs one cheap pass (scan_jobs is small, and the column is
// indexed) and makes every later batch bound what it claims to bound.
func (d *DatabaseConnection) detachHistoryScanJobCycle(ctx context.Context, workspaceID uint, batchSize int) error {
	statement := `UPDATE scan_jobs SET history_id = NULL WHERE id IN (
		SELECT id FROM scan_jobs WHERE workspace_id = ? AND history_id IS NOT NULL LIMIT ?)`

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		tx := d.db.WithContext(ctx).Exec(statement, workspaceID, batchSize)
		if tx.Error != nil {
			log.Error().Err(tx.Error).Uint("workspace", workspaceID).Msg("Failed to detach scan job history references")
			return fmt.Errorf("detaching scan job history references for workspace %d: %w", workspaceID, tx.Error)
		}
		if tx.RowsAffected == 0 {
			return nil
		}
	}
}

// cancelWorkspaceScans stops any scan still running against the workspace so
// that workers do not keep inserting rows behind the delete. Cancellation is
// published through the database, which is how the control registry propagates
// it to workers in other processes.
func (d *DatabaseConnection) cancelWorkspaceScans(ctx context.Context, workspaceID uint) (int64, error) {
	activeStatuses := []ScanStatus{ScanStatusPending, ScanStatusCrawling, ScanStatusScanning, ScanStatusPaused}

	var scanIDs []uint
	if err := d.db.WithContext(ctx).Model(&Scan{}).
		Where("workspace_id = ? AND status IN ?", workspaceID, activeStatuses).
		Pluck("id", &scanIDs).Error; err != nil {
		return 0, fmt.Errorf("listing active scans for workspace %d: %w", workspaceID, err)
	}
	if len(scanIDs) == 0 {
		return 0, nil
	}

	now := time.Now()
	if err := d.db.WithContext(ctx).Model(&Scan{}).
		Where("id IN ?", scanIDs).
		Updates(map[string]any{"status": ScanStatusCancelled, "completed_at": now}).Error; err != nil {
		return 0, fmt.Errorf("cancelling scans for workspace %d: %w", workspaceID, err)
	}
	if err := d.db.WithContext(ctx).Model(&ScanJob{}).
		Where("scan_id IN ? AND status IN ?", scanIDs, []ScanJobStatus{ScanJobStatusPending, ScanJobStatusClaimed}).
		Update("status", ScanJobStatusCancelled).Error; err != nil {
		return 0, fmt.Errorf("cancelling scan jobs for workspace %d: %w", workspaceID, err)
	}

	log.Info().Uint("workspace", workspaceID).Int("scans", len(scanIDs)).Msg("Cancelled active scans before workspace delete")
	return int64(len(scanIDs)), nil
}

func (d *DatabaseConnection) deleteWorkspaceTableInBatches(ctx context.Context, table workspaceBulkTable, workspaceID uint, batchSize int, progress func(string, int64)) (int64, error) {
	statement := fmt.Sprintf(
		"DELETE FROM %s WHERE id IN (SELECT id FROM %s WHERE %s LIMIT ?)",
		table.Name, table.Name, table.Scope,
	)

	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}

		tx := d.db.WithContext(ctx).Exec(statement, workspaceID, batchSize)
		if tx.Error != nil {
			log.Error().Err(tx.Error).Str("table", table.Name).Uint("workspace", workspaceID).Msg("Batched delete failed")
			return total, fmt.Errorf("deleting %s rows for workspace %d: %w", table.Name, workspaceID, tx.Error)
		}
		if tx.RowsAffected == 0 {
			return total, nil
		}

		total += tx.RowsAffected
		if progress != nil {
			progress(table.Name, total)
		}
	}
}
