package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// DefaultImportBatchSize is how many rows are inserted per statement.
const DefaultImportBatchSize = 500

// ImportOptions configures an import.
type ImportOptions struct {
	// Code overrides the imported workspace's code. Empty keeps the archived
	// code, suffixed if that code is already taken.
	Code string
	// Title overrides the imported workspace's title.
	Title string
	// BatchSize is the number of rows per insert statement.
	BatchSize int
	// Progress, when set, is called after each table with the rows inserted.
	Progress func(table string, rows int64)
}

// ImportResult reports what an import created.
type ImportResult struct {
	WorkspaceID uint             `json:"workspace_id"`
	Code        string           `json:"code"`
	Source      WorkspaceInfo    `json:"source"`
	RowsByTable map[string]int64 `json:"rows_by_table"`
	TotalRows   int64            `json:"total_rows"`
	Skipped     []ExcludedTable  `json:"skipped_tables,omitempty"`
	Duration    time.Duration    `json:"duration"`
}

// Import reads an archive produced by Export and recreates the workspace.
//
// Every bigint identifier in the archive is shifted by one offset chosen to sit
// above the highest identifier currently in the database. Because the same
// offset applies to primary keys and to the foreign keys pointing at them, all
// internal references stay consistent without a per-row mapping table -- which
// matters when a single workspace holds millions of history rows.
//
// UUID identities are minted fresh and rewritten through a map. A uuid
// reference whose target was never imported resolves to NULL, which is what
// makes deliberately skipped tables safe to leave out.
func Import(ctx context.Context, conn *db.DatabaseConnection, r io.Reader, opts ImportOptions) (result *ImportResult, err error) {
	batchSize := opts.BatchSize
	if batchSize <= 0 {
		batchSize = DefaultImportBatchSize
	}

	reader, err := newArchiveReader(r)
	if err != nil {
		return nil, err
	}
	defer reader.close()

	offset, err := computeIDOffset(ctx, conn)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	state := &importState{
		conn:      conn,
		offset:    offset,
		uuids:     make(map[string]string),
		batchSize: batchSize,
		deferred:  make(map[string][]deferredPatch),
		counts:    make(map[string]int64),
	}

	result = &ImportResult{
		Source:      reader.Manifest.Workspace,
		RowsByTable: state.counts,
		Skipped:     reader.Manifest.Excluded,
	}

	// A failed import leaves a partial workspace behind; remove it so the
	// operator is not left with half a dataset carrying a real workspace ID.
	defer func() {
		if err != nil && state.newWorkspaceID != 0 {
			if _, cleanupErr := conn.DeleteWorkspaceCascade(context.WithoutCancel(ctx), state.newWorkspaceID, db.WorkspaceDeleteOptions{}); cleanupErr != nil {
				log.Error().Err(cleanupErr).Uint("workspace", state.newWorkspaceID).Msg("Failed to clean up partially imported workspace")
			}
		}
	}()

	if err = state.consume(ctx, reader, opts); err != nil {
		return nil, err
	}
	if err = state.applyDeferredPatches(ctx); err != nil {
		return nil, err
	}
	if err = state.advanceSequences(ctx); err != nil {
		return nil, err
	}

	result.WorkspaceID = state.newWorkspaceID
	result.Code = state.newWorkspaceCode
	result.TotalRows = state.total
	result.Duration = time.Since(started)

	log.Info().
		Uint("workspace", result.WorkspaceID).
		Str("code", result.Code).
		Int64("rows", result.TotalRows).
		Dur("duration", result.Duration).
		Msg("Workspace imported")

	return result, nil
}

type deferredPatch struct {
	PK    string      `json:"pk"`
	Value json.Number `json:"val"`
}

type importState struct {
	conn      *db.DatabaseConnection
	offset    int64
	uuids     map[string]string
	batchSize int
	// deferred is keyed by "table.column"; a table may defer more than one
	// column and each needs its own set of values.
	deferred         map[string][]deferredPatch
	counts           map[string]int64
	total            int64
	newWorkspaceID   uint
	newWorkspaceCode string
}

func (s *importState) consume(ctx context.Context, reader *archiveReader, opts ImportOptions) error {
	var (
		pending   []json.RawMessage
		pendingID string
	)

	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		spec, _ := tableByName(pendingID)
		if err := s.insertBatch(ctx, spec, pending); err != nil {
			return err
		}
		s.counts[pendingID] += int64(len(pending))
		s.total += int64(len(pending))
		pending = pending[:0]
		return nil
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		rec, err := reader.next()
		if err != nil {
			return err
		}
		if rec.Table == summaryTag {
			if err := flush(); err != nil {
				return err
			}
			return s.verifySummary(rec.Row, opts)
		}

		spec, ok := tableByName(rec.Table)
		if !ok {
			return fmt.Errorf("archive references unknown table %q", rec.Table)
		}

		if rec.Table != pendingID {
			if err := flush(); err != nil {
				return err
			}
			pendingID = rec.Table
			if opts.Progress != nil && s.counts[rec.Table] == 0 {
				opts.Progress(rec.Table, 0)
			}
		}
		if spec.SkipImport {
			continue
		}

		transformed, err := s.transform(spec, rec.Row, opts)
		if err != nil {
			return err
		}
		pending = append(pending, transformed)

		if len(pending) >= s.batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
}

func (s *importState) verifySummary(raw json.RawMessage, opts ImportOptions) error {
	var summary Summary
	if err := json.Unmarshal(raw, &summary); err != nil {
		return fmt.Errorf("decoding archive summary: %w", err)
	}
	for table, expected := range summary.RowsByTable {
		spec, ok := tableByName(table)
		if !ok || spec.SkipImport {
			continue
		}
		if got := s.counts[table]; got != expected {
			return fmt.Errorf("archive is truncated: expected %d %s rows, read %d", expected, table, got)
		}
		if opts.Progress != nil && expected > 0 {
			opts.Progress(table, expected)
		}
	}
	return nil
}

// transform rewrites one archived row into the identifier space of this
// database.
func (s *importState) transform(spec tableSpec, raw json.RawMessage, opts ImportOptions) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()

	row := map[string]any{}
	if err := decoder.Decode(&row); err != nil {
		return nil, fmt.Errorf("decoding %s row: %w", spec.Name, err)
	}

	for _, column := range spec.IDColumns {
		shifted, err := s.shift(row[column])
		if err != nil {
			return nil, fmt.Errorf("%s.%s: %w", spec.Name, column, err)
		}
		if shifted != nil {
			row[column] = shifted
		}
	}

	if spec.UUIDPrimaryKey != "" {
		if old, ok := row[spec.UUIDPrimaryKey].(string); ok && old != "" {
			minted := uuid.New().String()
			s.uuids[old] = minted
			row[spec.UUIDPrimaryKey] = minted
		}
	}
	for _, column := range spec.UUIDReferences {
		old, ok := row[column].(string)
		if !ok || old == "" {
			continue
		}
		if minted, known := s.uuids[old]; known {
			row[column] = minted
		} else {
			// The target was never imported (a skipped table, or a reference
			// out of the exported set). NULL is the only valid value.
			row[column] = nil
		}
	}

	pk := spec.primaryKeyColumn()
	for _, column := range spec.DeferredColumns {
		value, ok := row[column].(json.Number)
		if ok && pk != "" {
			key := spec.Name + "." + column
			s.deferred[key] = append(s.deferred[key], deferredPatch{
				PK:    fmt.Sprintf("%v", row[pk]),
				Value: value,
			})
		}
		delete(row, column)
	}

	if spec.Name == "workspaces" {
		if err := s.claimWorkspace(row, opts); err != nil {
			return nil, err
		}
	}

	encoded, err := json.Marshal(row)
	if err != nil {
		return nil, fmt.Errorf("encoding %s row: %w", spec.Name, err)
	}
	return encoded, nil
}

func (s *importState) claimWorkspace(row map[string]any, opts ImportOptions) error {
	id, err := toInt64(row["id"])
	if err != nil {
		return fmt.Errorf("workspace row has no usable id: %w", err)
	}
	s.newWorkspaceID = uint(id)

	code, _ := row["code"].(string)
	if opts.Code != "" {
		code = opts.Code
	}
	code = s.uniqueWorkspaceCode(code)
	row["code"] = code
	s.newWorkspaceCode = code

	if opts.Title != "" {
		row["title"] = opts.Title
	}
	// A soft-deleted source workspace must not import as already deleted.
	row["deleted_at"] = nil
	return nil
}

// uniqueWorkspaceCode keeps codes distinguishable. The column has no unique
// index, so a collision would not error -- it would silently produce two
// workspaces that GetWorkspaceByCode cannot tell apart.
func (s *importState) uniqueWorkspaceCode(code string) string {
	if code == "" {
		code = "imported"
	}
	candidate := code
	for suffix := 2; suffix < 1000; suffix++ {
		if _, err := s.conn.GetWorkspaceByCode(candidate); err != nil {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", code, suffix)
	}
	return fmt.Sprintf("%s-%d", code, time.Now().UnixNano())
}

func (s *importState) shift(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	number, ok := value.(json.Number)
	if !ok {
		return nil, nil
	}
	parsed, err := number.Int64()
	if err != nil {
		return nil, fmt.Errorf("identifier %q is not an integer: %w", number.String(), err)
	}
	return json.Number(strconv.FormatInt(parsed+s.offset, 10)), nil
}

func (s *importState) insertBatch(ctx context.Context, spec tableSpec, rows []json.RawMessage) error {
	if len(rows) == 0 {
		return nil
	}
	payload, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("encoding %s batch: %w", spec.Name, err)
	}

	statement := fmt.Sprintf(
		"INSERT INTO %s SELECT * FROM json_populate_recordset(NULL::%s, ?::json)",
		spec.Name, spec.Name,
	)
	if err := s.conn.DB().WithContext(ctx).Exec(statement, string(payload)).Error; err != nil {
		return fmt.Errorf("inserting %d %s rows: %w", len(rows), spec.Name, err)
	}
	return nil
}

// applyDeferredPatches fills in the foreign keys that were withheld to break
// cycles in the schema.
func (s *importState) applyDeferredPatches(ctx context.Context) error {
	for _, spec := range orderedTables {
		if len(spec.DeferredColumns) == 0 {
			continue
		}
		pk := spec.primaryKeyColumn()
		if pk == "" {
			return fmt.Errorf("table %s defers columns but has no single-column primary key", spec.Name)
		}

		for _, column := range spec.DeferredColumns {
			patches := s.deferred[spec.Name+"."+column]
			if len(patches) == 0 {
				continue
			}
			payload, err := json.Marshal(patches)
			if err != nil {
				return fmt.Errorf("encoding %s.%s patches: %w", spec.Name, column, err)
			}
			statement := fmt.Sprintf(`
				UPDATE %s AS target SET %s = patch.val
				FROM json_to_recordset(?::json) AS patch(pk text, val bigint)
				WHERE target.%s::text = patch.pk`, spec.Name, column, pk)
			if err := s.conn.DB().WithContext(ctx).Exec(statement, string(payload)).Error; err != nil {
				return fmt.Errorf("patching %s.%s: %w", spec.Name, column, err)
			}
		}
	}
	return nil
}

// advanceSequences moves each identity sequence past the rows just inserted, so
// that the next natural insert does not collide with an imported identifier.
func (s *importState) advanceSequences(ctx context.Context) error {
	for _, spec := range orderedTables {
		if spec.SkipImport || !spec.hasBigintPrimaryKey() {
			continue
		}
		statement := fmt.Sprintf(`
			SELECT setval(seq, GREATEST(COALESCE((SELECT MAX(id) FROM %s), 1), 1))
			FROM pg_get_serial_sequence('%s', 'id') AS seq
			WHERE seq IS NOT NULL`, spec.Name, spec.Name)
		if err := s.conn.DB().WithContext(ctx).Exec(statement).Error; err != nil {
			return fmt.Errorf("advancing %s sequence: %w", spec.Name, err)
		}
	}
	return nil
}

// computeIDOffset returns a shift large enough that no imported bigint
// identifier collides with an existing row in any table.
func computeIDOffset(ctx context.Context, conn *db.DatabaseConnection) (int64, error) {
	var highest int64
	for _, spec := range orderedTables {
		if !spec.hasBigintPrimaryKey() {
			continue
		}
		var current *int64
		if err := conn.DB().WithContext(ctx).
			Raw(fmt.Sprintf("SELECT MAX(id) FROM %s", spec.Name)).
			Scan(&current).Error; err != nil {
			return 0, fmt.Errorf("reading highest id of %s: %w", spec.Name, err)
		}
		if current != nil && *current > highest {
			highest = *current
		}
	}
	return highest + 1, nil
}

func toInt64(value any) (int64, error) {
	switch typed := value.(type) {
	case json.Number:
		return typed.Int64()
	case float64:
		return int64(typed), nil
	case int64:
		return typed, nil
	}
	return 0, errors.New("value is not numeric")
}

func (s tableSpec) primaryKeyColumn() string {
	if s.UUIDPrimaryKey != "" {
		return s.UUIDPrimaryKey
	}
	if s.hasBigintPrimaryKey() {
		return "id"
	}
	return ""
}

func (s tableSpec) hasBigintPrimaryKey() bool {
	for _, column := range s.IDColumns {
		if column == "id" {
			return true
		}
	}
	return false
}
