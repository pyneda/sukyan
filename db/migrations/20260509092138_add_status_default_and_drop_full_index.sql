-- Drop index "idx_playground_ws_runs_status" from table: "playground_ws_runs"
DROP INDEX "idx_playground_ws_runs_status";
-- Modify "playground_ws_runs" table
ALTER TABLE "playground_ws_runs" ALTER COLUMN "status" SET NOT NULL, ALTER COLUMN "status" SET DEFAULT 'pending';
