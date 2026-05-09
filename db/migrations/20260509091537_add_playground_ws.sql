-- Modify "web_socket_connections" table
ALTER TABLE "web_socket_connections" ADD COLUMN "playground_session_id" bigint NULL, ADD CONSTRAINT "fk_web_socket_connections_playground_session" FOREIGN KEY ("playground_session_id") REFERENCES "playground_sessions" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Create index "idx_web_socket_connections_playground_session_id" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_playground_session_id" ON "web_socket_connections" ("playground_session_id");
-- Create "playground_ws_sessions" table
CREATE TABLE "playground_ws_sessions" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "playground_session_id" bigint NOT NULL,
  "target_url" text NULL,
  "request_headers" jsonb NULL,
  "script" jsonb NULL,
  "options" jsonb NULL,
  "imported_from_connection_id" bigint NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_playground_ws_sessions_imported_from_connection" FOREIGN KEY ("imported_from_connection_id") REFERENCES "web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT "fk_playground_ws_sessions_playground_session" FOREIGN KEY ("playground_session_id") REFERENCES "playground_sessions" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_playground_ws_sessions_deleted_at" to table: "playground_ws_sessions"
CREATE INDEX "idx_playground_ws_sessions_deleted_at" ON "playground_ws_sessions" ("deleted_at");
-- Create index "idx_playground_ws_sessions_imported_from_connection_id" to table: "playground_ws_sessions"
CREATE INDEX "idx_playground_ws_sessions_imported_from_connection_id" ON "playground_ws_sessions" ("imported_from_connection_id");
-- Create index "idx_playground_ws_sessions_playground_session_id" to table: "playground_ws_sessions"
CREATE UNIQUE INDEX "idx_playground_ws_sessions_playground_session_id" ON "playground_ws_sessions" ("playground_session_id");
-- Create "playground_ws_runs" table
CREATE TABLE "playground_ws_runs" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "playground_ws_session_id" bigint NOT NULL,
  "web_socket_connection_id" bigint NULL,
  "script_snapshot" jsonb NULL,
  "options_snapshot" jsonb NULL,
  "status" text NULL,
  "current_step_index" bigint NULL,
  "failure_reason" text NULL,
  "started_at" timestamptz NULL,
  "finished_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_playground_ws_runs_playground_ws_session" FOREIGN KEY ("playground_ws_session_id") REFERENCES "playground_ws_sessions" ("id") ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT "fk_playground_ws_runs_web_socket_connection" FOREIGN KEY ("web_socket_connection_id") REFERENCES "web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE SET NULL
);
-- Create index "idx_playground_ws_runs_deleted_at" to table: "playground_ws_runs"
CREATE INDEX "idx_playground_ws_runs_deleted_at" ON "playground_ws_runs" ("deleted_at");
-- Create index "idx_playground_ws_runs_playground_ws_session_id" to table: "playground_ws_runs"
CREATE INDEX "idx_playground_ws_runs_playground_ws_session_id" ON "playground_ws_runs" ("playground_ws_session_id");
-- Create index "idx_playground_ws_runs_status" to table: "playground_ws_runs"
CREATE INDEX "idx_playground_ws_runs_status" ON "playground_ws_runs" ("status");
-- Create index "idx_playground_ws_runs_web_socket_connection_id" to table: "playground_ws_runs"
CREATE INDEX "idx_playground_ws_runs_web_socket_connection_id" ON "playground_ws_runs" ("web_socket_connection_id");
