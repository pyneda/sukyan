-- Create partial index "idx_playground_ws_runs_status_running" to table: "playground_ws_runs"
CREATE INDEX "idx_playground_ws_runs_status_running"
  ON "playground_ws_runs" ("status")
  WHERE "status" = 'running';
