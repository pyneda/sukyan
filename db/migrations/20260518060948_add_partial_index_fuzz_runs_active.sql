-- Partial index on "active" statuses used by the boot-time recovery sweep
-- (MarkOrphanedFuzzRunsAborted). Speeds up the WHERE status IN (...) scan.
CREATE INDEX "idx_playground_fuzz_runs_status_active"
  ON "playground_fuzz_runs" ("status")
  WHERE "status" IN ('pending', 'calibrating', 'running');
