package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// DefaultImportBatchSize is how many rows are inserted per statement.
const DefaultImportBatchSize = 500

// maxPendingBatchBytes caps how much row data an insert batch accumulates,
// independently of the row count.
const maxPendingBatchBytes = 32 << 20

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
// Each table's bigint identifiers are shifted by an offset that lands them above
// everything already stored in that table. A foreign key is shifted by the
// offset of the table it points at, so all internal references stay consistent
// without a per-row mapping table -- which matters when a single workspace holds
// millions of history rows.
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

	space, err := reserveIdentifierSpace(ctx, conn, reader.Manifest)
	if err != nil {
		return nil, err
	}

	started := time.Now()
	state := &importState{
		conn:           conn,
		offsets:        space.offsets,
		lowestAllowed:  space.floors,
		highestAllowed: space.ceilings,
		uuids:          make(map[string]string),
		batchSize:      batchSize,
		deferred:       make(map[string][]deferredPatch),
		counts:         make(map[string]int64),
	}

	result = &ImportResult{
		Source:      reader.Manifest.Workspace,
		RowsByTable: state.counts,
		Skipped:     reader.Manifest.Excluded,
	}

	// A failed import leaves a partial workspace behind; remove it so the
	// operator is not left with half a dataset carrying a real workspace ID.
	//
	// Gated on the workspace row actually having been written by this import.
	// The id is chosen before the insert, so acting on it earlier could delete a
	// workspace someone else created in the meantime, cascading through all of
	// its data.
	defer func() {
		if err != nil && state.workspaceInserted && state.newWorkspaceID != 0 {
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
	conn *db.DatabaseConnection
	// offsets shifts a table's archived identifiers onto the block reserved for
	// it; lowestAllowed and highestAllowed are the ends of that block, which
	// every shifted value has to land between. All keyed by table name.
	offsets        map[string]int64
	lowestAllowed  map[string]int64
	highestAllowed map[string]int64
	uuids          map[string]string
	batchSize      int
	// deferred is keyed by "table.column"; a table may defer more than one
	// column and each needs its own set of values.
	deferred         map[string][]deferredPatch
	counts           map[string]int64
	total            int64
	newWorkspaceID   uint
	newWorkspaceCode string
	// workspaceInserted records that this import wrote the workspace row, which
	// is what makes it safe to delete it again during cleanup.
	workspaceInserted bool
}

func (s *importState) consume(ctx context.Context, reader *archiveReader, opts ImportOptions) error {
	var (
		pending      []json.RawMessage
		pendingBytes int
		pendingID    string
	)

	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		spec, ok := tableByName(pendingID)
		if !ok {
			return fmt.Errorf("archive references unknown table %q", pendingID)
		}
		if err := s.insertBatch(ctx, spec, pending); err != nil {
			return err
		}
		s.counts[pendingID] += int64(len(pending))
		s.total += int64(len(pending))
		pending = pending[:0]
		pendingBytes = 0
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
		// Every later row is pinned to the workspace this record creates, so
		// nothing may precede it.
		if s.newWorkspaceID == 0 && rec.Table != "workspaces" {
			return fmt.Errorf("archive starts with %q; the workspace record must come first", rec.Table)
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
		pendingBytes += len(transformed)

		// Bounded by bytes as well as rows: a single archived row may be
		// megabytes (a stored HTTP exchange), and an archive is untrusted input,
		// so a row count alone puts no ceiling on what a batch holds.
		if len(pending) >= s.batchSize || pendingBytes >= maxPendingBatchBytes {
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

	for column, target := range spec.IDColumns {
		shifted, err := s.shift(row[column], target)
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
	} else if spec.ownsWorkspaceColumn() {
		// Containment is asserted here rather than inherited from the shifted
		// value: whatever the archive claims, an imported row belongs to the
		// workspace this import created.
		row["workspace_id"] = json.Number(strconv.FormatUint(uint64(s.newWorkspaceID), 10))
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

// shift moves an archived identifier into this database's identifier space.
//
// An archive is untrusted input. Identifiers originate from bigserial columns
// and are always positive, so anything else has been tampered with -- and a
// negative value is not harmless, it cancels the offset and aims the row at an
// identifier of the attacker's choosing.
func (s *importState) shift(value any, target string) (any, error) {
	if value == nil {
		return nil, nil
	}
	// Anything that is not a JSON number has to be rejected rather than passed
	// through. Postgres coerces a JSON string to bigint on insert, so letting a
	// non-numeric value survive unshifted would hand the archive direct control
	// of the stored identifier -- and child tables carry no workspace_id for the
	// pin below to correct.
	number, ok := value.(json.Number)
	if !ok {
		return nil, fmt.Errorf("identifier is %T, expected a number", value)
	}
	parsed, err := number.Int64()
	if err != nil {
		return nil, fmt.Errorf("identifier %q is not an integer: %w", number.String(), err)
	}
	if parsed < 1 {
		return nil, fmt.Errorf("identifier %d is not positive", parsed)
	}

	ceiling, reserved := s.highestAllowed[target]
	if !reserved {
		// Closure means a non-NULL reference implies its target is in the
		// archive, so the target table cannot be empty. Reaching here means the
		// archive is not closed, and there is no block to shift onto.
		return nil, fmt.Errorf("reference to %s identifier %d, but the archive holds no %s rows", target, parsed, target)
	}
	offset := s.offsets[target]
	floor := s.lowestAllowed[target]
	if offset > 0 && parsed > math.MaxInt64-offset {
		return nil, fmt.Errorf("identifier %d overflows when shifted by %d", parsed, offset)
	}
	shifted := parsed + offset
	// The offset comes from bounds the archive supplied, so the result is
	// checked rather than trusted. Both ends matter: below the block and the row
	// could collide with an unrelated one, above it and the row escapes the
	// range reserved for this import, where a concurrent writer may already have
	// been handed the identifier.
	if shifted < floor {
		return nil, fmt.Errorf("identifier %d shifts to %d, below the first reserved %s identifier %d", parsed, shifted, target, floor)
	}
	if shifted > ceiling {
		return nil, fmt.Errorf("identifier %d shifts to %d, above the last reserved %s identifier %d", parsed, shifted, target, ceiling)
	}
	return json.Number(strconv.FormatInt(shifted, 10)), nil
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
	if spec.Name == "workspaces" {
		s.workspaceInserted = true
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

// identifierSpace is the outcome of reserving room for an import: per table, the
// block of identifiers it owns and the offset that moves archived identifiers
// onto it.
type identifierSpace struct {
	offsets  map[string]int64
	floors   map[string]int64
	ceilings map[string]int64
}

// reserveIdentifierSpace claims, for every table the archive carries rows for, a
// run of identifiers wide enough to hold them -- before a single row is
// inserted. Inserting above MAX(id) instead leaves the sequence pointing at the
// first identifier the import is about to use, so every scan running alongside
// is handed identifiers out of the middle of the import's range.
//
// A table's offset is anchored to the lowest identifier the archive holds for
// that table, so growth is the archive's own span rather than the target
// database's ceiling. A single shared offset would instead drag every table up
// to the largest table's maximum, doubling it on each same-database import.
func reserveIdentifierSpace(ctx context.Context, conn *db.DatabaseConnection, manifest Manifest) (*identifierSpace, error) {
	space := &identifierSpace{
		offsets:  make(map[string]int64, len(orderedTables)),
		floors:   make(map[string]int64, len(orderedTables)),
		ceilings: make(map[string]int64, len(orderedTables)),
	}

	for _, spec := range orderedTables {
		if !spec.hasBigintPrimaryKey() {
			continue
		}
		base, top := manifest.IdentifierBases[spec.Name], manifest.IdentifierCeilings[spec.Name]
		if base <= 0 {
			// No rows for this table in the archive, so nothing to reserve. A
			// reference to it is rejected by shift.
			continue
		}
		if top < base {
			return nil, fmt.Errorf("archive declares %s identifiers from %d to %d", spec.Name, base, top)
		}
		span := top - base + 1

		first, err := reserveSequenceBlock(ctx, conn, spec.Name, span)
		if err != nil {
			return nil, err
		}
		space.floors[spec.Name] = first
		space.ceilings[spec.Name] = first + span - 1
		space.offsets[spec.Name] = first - base
	}
	return space, nil
}

// reserveSequenceBlock advances a table's identity sequence by span in one
// atomic step and returns the first identifier of the run it just consumed.
//
// nextval is the only allocator Postgres offers that reserves atomically, so the
// increment is widened for a single call to turn it into a block allocator.
// Reading the sequence and setval-ing past it would leave a window in which a
// writer is handed an identifier inside the block. ALTER SEQUENCE is
// transactional, so an aborted reservation cannot strand the widened increment,
// and it holds a lock that keeps concurrent writers out for the three statements
// this takes.
func reserveSequenceBlock(ctx context.Context, conn *db.DatabaseConnection, table string, span int64) (int64, error) {
	var sequence *string
	if err := conn.DB().WithContext(ctx).
		Raw("SELECT pg_get_serial_sequence(?, 'id')", table).Scan(&sequence).Error; err != nil {
		return 0, fmt.Errorf("locating the %s identity sequence: %w", table, err)
	}
	if sequence == nil || *sequence == "" {
		return 0, fmt.Errorf("table %s has no identity sequence to reserve from", table)
	}
	name := *sequence

	var top int64
	err := conn.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A database restored without its sequences would otherwise hand out
		// identifiers that already exist. Forward only: winding a sequence back
		// would reissue values another process may already hold.
		if err := tx.Exec(fmt.Sprintf(
			`SELECT setval('%s', GREATEST(COALESCE(pg_sequence_last_value('%s'), 1), COALESCE((SELECT MAX(id) FROM %s), 1), 1))`,
			name, name, table)).Error; err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("ALTER SEQUENCE %s INCREMENT BY %d", name, span)).Error; err != nil {
			return err
		}
		if err := tx.Raw(fmt.Sprintf("SELECT nextval('%s')", name)).Scan(&top).Error; err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf("ALTER SEQUENCE %s INCREMENT BY 1", name)).Error
	})
	if err != nil {
		return 0, fmt.Errorf("reserving %d %s identifiers: %w", span, table, err)
	}
	return top - span + 1, nil
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
	_, ok := s.IDColumns["id"]
	return ok
}

// ownsWorkspaceColumn reports whether rows of this table carry the workspace
// they belong to directly, rather than inheriting it through a parent.
func (s tableSpec) ownsWorkspaceColumn() bool {
	_, ok := s.IDColumns["workspace_id"]
	return ok
}
