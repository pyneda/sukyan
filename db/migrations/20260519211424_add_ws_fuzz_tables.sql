-- Create "playground_ws_fuzz_runs" table
CREATE TABLE "playground_ws_fuzz_runs" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "session_id" bigint NULL,
  "config_snapshot" jsonb NULL,
  "baseline_snapshot" jsonb NULL,
  "matchers_snapshot" jsonb NULL,
  "status" text NULL,
  "iteration_count" bigint NULL,
  "sent_count" bigint NULL,
  "error_count" bigint NULL,
  "finding_count" bigint NULL,
  "started_at" timestamptz NULL,
  "finished_at" timestamptz NULL,
  "failure_reason" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_playground_ws_fuzz_runs_session" FOREIGN KEY ("session_id") REFERENCES "playground_sessions" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_playground_ws_fuzz_runs_deleted_at" to table: "playground_ws_fuzz_runs"
CREATE INDEX "idx_playground_ws_fuzz_runs_deleted_at" ON "playground_ws_fuzz_runs" ("deleted_at");
-- Create index "idx_playground_ws_fuzz_runs_session_id" to table: "playground_ws_fuzz_runs"
CREATE INDEX "idx_playground_ws_fuzz_runs_session_id" ON "playground_ws_fuzz_runs" ("session_id");
-- Create index "idx_playground_ws_fuzz_runs_status" to table: "playground_ws_fuzz_runs"
CREATE INDEX "idx_playground_ws_fuzz_runs_status" ON "playground_ws_fuzz_runs" ("status");
-- Create "playground_ws_fuzz_iterations" table
CREATE TABLE "playground_ws_fuzz_iterations" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "run_id" bigint NULL,
  "iteration_index" bigint NULL,
  "status" text NULL,
  "payload_values" jsonb NULL,
  "baseline_match" boolean NULL,
  "duration_ms" bigint NULL,
  "handshake_status_code" bigint NULL,
  "handshake_headers" jsonb NULL,
  "web_socket_connection_id" bigint NULL,
  "peer_close_code" bigint NULL,
  "failure_reason" text NULL,
  "failed_step_index" bigint NULL,
  "check_results" jsonb NULL,
  "variables_snapshot" jsonb NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_playground_ws_fuzz_iterations_run" FOREIGN KEY ("run_id") REFERENCES "playground_ws_fuzz_runs" ("id") ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT "fk_playground_ws_fuzz_iterations_web_socket_connection" FOREIGN KEY ("web_socket_connection_id") REFERENCES "web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_playground_ws_fuzz_iterations_baseline_match" to table: "playground_ws_fuzz_iterations"
CREATE INDEX "idx_playground_ws_fuzz_iterations_baseline_match" ON "playground_ws_fuzz_iterations" ("baseline_match");
-- Create index "idx_playground_ws_fuzz_iterations_deleted_at" to table: "playground_ws_fuzz_iterations"
CREATE INDEX "idx_playground_ws_fuzz_iterations_deleted_at" ON "playground_ws_fuzz_iterations" ("deleted_at");
-- Create index "idx_playground_ws_fuzz_iterations_run_id" to table: "playground_ws_fuzz_iterations"
CREATE INDEX "idx_playground_ws_fuzz_iterations_run_id" ON "playground_ws_fuzz_iterations" ("run_id");
-- Create index "idx_playground_ws_fuzz_iterations_status" to table: "playground_ws_fuzz_iterations"
CREATE INDEX "idx_playground_ws_fuzz_iterations_status" ON "playground_ws_fuzz_iterations" ("status");
-- Create index "idx_ws_fuzz_run_iter" to table: "playground_ws_fuzz_iterations"
CREATE UNIQUE INDEX "idx_ws_fuzz_run_iter" ON "playground_ws_fuzz_iterations" ("run_id", "iteration_index");
