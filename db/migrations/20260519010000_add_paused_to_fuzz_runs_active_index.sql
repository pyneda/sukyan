-- The recovery sweep (MarkOrphanedFuzzRunsAborted) now also targets 'paused'
-- runs, since a paused run on a process that dies is genuinely orphaned.
-- Recreate the partial index to include 'paused' in its predicate.
DROP INDEX IF EXISTS "idx_playground_fuzz_runs_status_active";
CREATE INDEX "idx_playground_fuzz_runs_status_active"
  ON "playground_fuzz_runs" ("status")
  WHERE "status" IN ('pending', 'calibrating', 'running', 'paused');
