# Migrations

This directory holds Atlas-managed SQL migrations. Most files are generated via `atlas migrate diff --env gorm` from the GORM models registered in `db/atlas/main.go`.

## Hand-written migrations

A few migrations cannot be expressed with GORM struct tags and are written by hand. They live alongside the generated files and are timestamped to apply in the right order.

### Known recurring drift

When you run `atlas migrate diff` you may see Atlas propose a migration that drops the following indexes:

- `idx_playground_ws_runs_status_running` — partial index on `playground_ws_runs(status) WHERE status = 'running'`. Used by the boot-time recovery sweep that marks orphaned runs as `aborted_server_restart`. GORM's `index` tag emits a full btree only; the partial form is created by `20260509092200_add_partial_index_ws_runs_status_running.sql`.

- `idx_playground_fuzz_runs_status_active` — partial index on `playground_fuzz_runs(status) WHERE status IN ('pending','calibrating','running','paused')`. Same purpose for the HTTP fuzzer recovery sweep (`MarkOrphanedFuzzRunsAborted`); created by `20260518060948_add_partial_index_fuzz_runs_active.sql` and widened by `20260519010000_add_paused_to_fuzz_runs_active_index.sql`.

If you see `DROP INDEX` for either partial index in a fresh `atlas migrate diff` output, **discard that diff**. It's drift between gormschema and the live database, not a real change.
