# Migrations

This directory holds Atlas-managed SQL migrations. Most files are generated via `atlas migrate diff --env gorm` from the GORM models registered in `db/atlas/main.go`.

## Hand-written migrations

A few migrations cannot be expressed with GORM struct tags and are written by hand. They live alongside the generated files and are timestamped to apply in the right order.

### Known recurring drift

These indexes cannot be expressed as a GORM tag, so they are emitted as raw DDL
from `indexesGormCannotExpress` in `db/atlas/main.go`. That keeps them in the
desired state, and `atlas migrate diff` no longer proposes dropping them:

- `idx_playground_ws_runs_status_running` — partial index on `playground_ws_runs(status) WHERE status = 'running'`. Used by the boot-time recovery sweep that marks orphaned runs as `aborted_server_restart`. GORM's `index` tag emits a full btree only; the partial form is created by `20260509092200_add_partial_index_ws_runs_status_running.sql`.

- `idx_playground_fuzz_runs_status_active` — partial index on `playground_fuzz_runs(status) WHERE status IN ('pending','calibrating','running','paused')`. Same purpose for the HTTP fuzzer recovery sweep (`MarkOrphanedFuzzRunsAborted`); created by `20260518060948_add_partial_index_fuzz_runs_active.sql` and widened by `20260519010000_add_paused_to_fuzz_runs_active_index.sql`.

If a fresh `atlas migrate diff` ever proposes `DROP INDEX` for either partial
index again, the entry has gone missing from `indexesGormCannotExpress` — restore
it there rather than discarding the diff, which would now also throw away
legitimate changes. The same applies to the join-table indexes listed alongside
them: GORM generates those tables itself, so there is no struct to tag.
