-- Create "playground_fuzz_runs" table
CREATE TABLE "playground_fuzz_runs" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "playground_session_id" bigint NOT NULL,
  "workspace_id" bigint NOT NULL,
  "config_snapshot" jsonb NOT NULL,
  "baseline" jsonb NULL,
  "matchers" jsonb NULL,
  "status" text NOT NULL DEFAULT 'pending',
  "failure_reason" text NULL,
  "started_at" timestamptz NULL,
  "finished_at" timestamptz NULL,
  "planned_request_count" bigint NULL,
  "sent_request_count" bigint NULL,
  "error_count" bigint NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_playground_fuzz_runs_playground_session" FOREIGN KEY ("playground_session_id") REFERENCES "playground_sessions" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_playground_fuzz_runs_deleted_at" to table: "playground_fuzz_runs"
CREATE INDEX "idx_playground_fuzz_runs_deleted_at" ON "playground_fuzz_runs" ("deleted_at");
-- Create index "idx_playground_fuzz_runs_playground_session_id" to table: "playground_fuzz_runs"
CREATE INDEX "idx_playground_fuzz_runs_playground_session_id" ON "playground_fuzz_runs" ("playground_session_id");
-- Create index "idx_playground_fuzz_runs_status" to table: "playground_fuzz_runs"
CREATE INDEX "idx_playground_fuzz_runs_status" ON "playground_fuzz_runs" ("status");
-- Create index "idx_playground_fuzz_runs_workspace_id" to table: "playground_fuzz_runs"
CREATE INDEX "idx_playground_fuzz_runs_workspace_id" ON "playground_fuzz_runs" ("workspace_id");
-- Modify "histories" table
ALTER TABLE "histories" ADD COLUMN "playground_fuzz_run_id" bigint NULL, ADD CONSTRAINT "fk_histories_playground_fuzz_run" FOREIGN KEY ("playground_fuzz_run_id") REFERENCES "playground_fuzz_runs" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Create index "idx_histories_playground_fuzz_run_id" to table: "histories"
CREATE INDEX "idx_histories_playground_fuzz_run_id" ON "histories" ("playground_fuzz_run_id");
