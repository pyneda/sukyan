-- Create severity enum type
DO $$ BEGIN
  CREATE TYPE severity AS ENUM ('Unknown', 'Info', 'Low', 'Medium', 'High', 'Critical');
EXCEPTION
  WHEN duplicate_object THEN null;
END $$;

-- Create "histories" table
CREATE TABLE "public"."histories" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "status_code" bigint NULL,
  "url" text NULL,
  "clean_url" text NULL,
  "depth" bigint NULL,
  "raw_request" bytea NULL,
  "raw_response" bytea NULL,
  "method" text NULL,
  "proto" text NULL,
  "parameters_count" bigint NULL,
  "evaluated" boolean NULL,
  "note" text NULL,
  "source" text NULL,
  "workspace_id" bigint NULL,
  "task_id" bigint NULL,
  "scan_id" bigint NULL,
  "scan_job_id" bigint NULL,
  "playground_session_id" bigint NULL,
  "response_body_size" bigint NULL,
  "request_body_size" bigint NULL,
  "request_content_type" text NULL,
  "response_content_type" text NULL,
  "is_web_socket_upgrade" boolean NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_histories_clean_url" to table: "histories"
CREATE INDEX "idx_histories_clean_url" ON "public"."histories" ("clean_url");
-- Create index "idx_histories_deleted_at" to table: "histories"
CREATE INDEX "idx_histories_deleted_at" ON "public"."histories" ("deleted_at");
-- Create index "idx_histories_depth" to table: "histories"
CREATE INDEX "idx_histories_depth" ON "public"."histories" ("depth");
-- Create index "idx_histories_evaluated" to table: "histories"
CREATE INDEX "idx_histories_evaluated" ON "public"."histories" ("evaluated");
-- Create index "idx_histories_method" to table: "histories"
CREATE INDEX "idx_histories_method" ON "public"."histories" ("method");
-- Create index "idx_histories_parameters_count" to table: "histories"
CREATE INDEX "idx_histories_parameters_count" ON "public"."histories" ("parameters_count");
-- Create index "idx_histories_playground_session_id" to table: "histories"
CREATE INDEX "idx_histories_playground_session_id" ON "public"."histories" ("playground_session_id");
-- Create index "idx_histories_proto" to table: "histories"
CREATE INDEX "idx_histories_proto" ON "public"."histories" ("proto");
-- Create index "idx_histories_request_body_size" to table: "histories"
CREATE INDEX "idx_histories_request_body_size" ON "public"."histories" ("request_body_size");
-- Create index "idx_histories_request_content_type" to table: "histories"
CREATE INDEX "idx_histories_request_content_type" ON "public"."histories" ("request_content_type");
-- Create index "idx_histories_response_body_size" to table: "histories"
CREATE INDEX "idx_histories_response_body_size" ON "public"."histories" ("response_body_size");
-- Create index "idx_histories_response_content_type" to table: "histories"
CREATE INDEX "idx_histories_response_content_type" ON "public"."histories" ("response_content_type");
-- Create index "idx_histories_scan_id" to table: "histories"
CREATE INDEX "idx_histories_scan_id" ON "public"."histories" ("scan_id");
-- Create index "idx_histories_scan_job_id" to table: "histories"
CREATE INDEX "idx_histories_scan_job_id" ON "public"."histories" ("scan_job_id");
-- Create index "idx_histories_source" to table: "histories"
CREATE INDEX "idx_histories_source" ON "public"."histories" ("source");
-- Create index "idx_histories_status_code" to table: "histories"
CREATE INDEX "idx_histories_status_code" ON "public"."histories" ("status_code");
-- Create index "idx_histories_task_id" to table: "histories"
CREATE INDEX "idx_histories_task_id" ON "public"."histories" ("task_id");
-- Create index "idx_histories_workspace_id" to table: "histories"
CREATE INDEX "idx_histories_workspace_id" ON "public"."histories" ("workspace_id");
-- Create "issue_requests" table
CREATE TABLE "public"."issue_requests" (
  "issue_id" bigint NOT NULL,
  "history_id" bigint NOT NULL,
  PRIMARY KEY ("issue_id", "history_id")
);
-- Create "issues" table
CREATE TABLE "public"."issues" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "code" text NULL,
  "title" text NULL,
  "description" text NULL,
  "details" text NULL,
  "remediation" text NULL,
  "cwe" bigint NULL,
  "url" text NULL,
  "status_code" bigint NULL,
  "http_method" text NULL,
  "payload" text NULL,
  "request" bytea NULL,
  "response" bytea NULL,
  "false_positive" boolean NULL,
  "confidence" bigint NULL,
  "references" bytea NULL,
  "severity" text NULL DEFAULT 'Info',
  "c_url_command" text NULL,
  "note" text NULL,
  "workspace_id" bigint NULL,
  "task_id" bigint NULL,
  "task_job_id" bigint NULL,
  "scan_id" bigint NULL,
  "scan_job_id" bigint NULL,
  "websocket_connection_id" bigint NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_issues_code" to table: "issues"
CREATE INDEX "idx_issues_code" ON "public"."issues" ("code");
-- Create index "idx_issues_confidence" to table: "issues"
CREATE INDEX "idx_issues_confidence" ON "public"."issues" ("confidence");
-- Create index "idx_issues_deleted_at" to table: "issues"
CREATE INDEX "idx_issues_deleted_at" ON "public"."issues" ("deleted_at");
-- Create index "idx_issues_false_positive" to table: "issues"
CREATE INDEX "idx_issues_false_positive" ON "public"."issues" ("false_positive");
-- Create index "idx_issues_http_method" to table: "issues"
CREATE INDEX "idx_issues_http_method" ON "public"."issues" ("http_method");
-- Create index "idx_issues_scan_id" to table: "issues"
CREATE INDEX "idx_issues_scan_id" ON "public"."issues" ("scan_id");
-- Create index "idx_issues_scan_job_id" to table: "issues"
CREATE INDEX "idx_issues_scan_job_id" ON "public"."issues" ("scan_job_id");
-- Create index "idx_issues_status_code" to table: "issues"
CREATE INDEX "idx_issues_status_code" ON "public"."issues" ("status_code");
-- Create index "idx_issues_task_id" to table: "issues"
CREATE INDEX "idx_issues_task_id" ON "public"."issues" ("task_id");
-- Create index "idx_issues_task_job_id" to table: "issues"
CREATE INDEX "idx_issues_task_job_id" ON "public"."issues" ("task_job_id");
-- Create index "idx_issues_title" to table: "issues"
CREATE INDEX "idx_issues_title" ON "public"."issues" ("title");
-- Create index "idx_issues_url" to table: "issues"
CREATE INDEX "idx_issues_url" ON "public"."issues" ("url");
-- Create index "idx_issues_websocket_connection_id" to table: "issues"
CREATE INDEX "idx_issues_websocket_connection_id" ON "public"."issues" ("websocket_connection_id");
-- Create index "idx_issues_workspace_id" to table: "issues"
CREATE INDEX "idx_issues_workspace_id" ON "public"."issues" ("workspace_id");
-- Create "json_web_token_histories" table
CREATE TABLE "public"."json_web_token_histories" (
  "json_web_token_id" bigint NOT NULL,
  "history_id" bigint NOT NULL,
  PRIMARY KEY ("json_web_token_id", "history_id")
);
-- Create "json_web_token_websocket_connections" table
CREATE TABLE "public"."json_web_token_websocket_connections" (
  "json_web_token_id" bigint NOT NULL,
  "web_socket_connection_id" bigint NOT NULL,
  PRIMARY KEY ("json_web_token_id", "web_socket_connection_id")
);
-- Create "json_web_tokens" table
CREATE TABLE "public"."json_web_tokens" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "token" text NULL,
  "header" jsonb NULL,
  "payload" jsonb NULL,
  "signature" text NULL,
  "algorithm" text NULL,
  "issuer" text NULL,
  "subject" text NULL,
  "audience" text NULL,
  "expiration" timestamp NULL,
  "issued_at" timestamp NULL,
  "workspace_id" bigint NULL,
  "tested_embedded_wordlist" boolean NULL,
  "cracked" boolean NULL,
  "secret" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_json_web_tokens_deleted_at" to table: "json_web_tokens"
CREATE INDEX "idx_json_web_tokens_deleted_at" ON "public"."json_web_tokens" ("deleted_at");
-- Create "oob_interactions" table
CREATE TABLE "public"."oob_interactions" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "oob_test_id" bigint NULL,
  "protocol" text NULL,
  "full_id" text NULL,
  "unique_id" text NULL,
  "q_type" text NULL,
  "raw_request" text NULL,
  "raw_response" text NULL,
  "remote_address" text NULL,
  "timestamp" timestamptz NULL,
  "workspace_id" bigint NULL,
  "issue_id" bigint NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_oob_interactions_deleted_at" to table: "oob_interactions"
CREATE INDEX "idx_oob_interactions_deleted_at" ON "public"."oob_interactions" ("deleted_at");
-- Create "oob_tests" table
CREATE TABLE "public"."oob_tests" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "code" text NULL,
  "test_name" text NULL,
  "target" text NULL,
  "history_id" bigint NULL,
  "interaction_domain" text NULL,
  "interaction_full_id" text NULL,
  "payload" text NULL,
  "insertion_point" text NULL,
  "workspace_id" bigint NULL,
  "task_id" bigint NULL,
  "task_job_id" bigint NULL,
  "scan_id" bigint NULL,
  "scan_job_id" bigint NULL,
  "issue_id" bigint NULL,
  "note" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_oob_tests_deleted_at" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_deleted_at" ON "public"."oob_tests" ("deleted_at");
-- Create index "idx_oob_tests_interaction_domain" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_interaction_domain" ON "public"."oob_tests" ("interaction_domain");
-- Create index "idx_oob_tests_interaction_full_id" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_interaction_full_id" ON "public"."oob_tests" ("interaction_full_id");
-- Create index "idx_oob_tests_issue_id" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_issue_id" ON "public"."oob_tests" ("issue_id");
-- Create index "idx_oob_tests_scan_id" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_scan_id" ON "public"."oob_tests" ("scan_id");
-- Create index "idx_oob_tests_scan_job_id" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_scan_job_id" ON "public"."oob_tests" ("scan_job_id");
-- Create index "idx_oob_tests_task_id" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_task_id" ON "public"."oob_tests" ("task_id");
-- Create index "idx_oob_tests_task_job_id" to table: "oob_tests"
CREATE INDEX "idx_oob_tests_task_job_id" ON "public"."oob_tests" ("task_job_id");
-- Create "playground_collections" table
CREATE TABLE "public"."playground_collections" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "name" text NULL,
  "description" text NULL,
  "workspace_id" bigint NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_playground_collections_deleted_at" to table: "playground_collections"
CREATE INDEX "idx_playground_collections_deleted_at" ON "public"."playground_collections" ("deleted_at");
-- Create index "idx_playground_collections_workspace_id" to table: "playground_collections"
CREATE INDEX "idx_playground_collections_workspace_id" ON "public"."playground_collections" ("workspace_id");
-- Create "playground_sessions" table
CREATE TABLE "public"."playground_sessions" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "name" text NULL,
  "type" text NULL,
  "original_request_id" bigint NULL,
  "initial_raw_request" text NULL,
  "collection_id" bigint NULL,
  "workspace_id" bigint NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_playground_sessions_deleted_at" to table: "playground_sessions"
CREATE INDEX "idx_playground_sessions_deleted_at" ON "public"."playground_sessions" ("deleted_at");
-- Create index "idx_playground_sessions_workspace_id" to table: "playground_sessions"
CREATE INDEX "idx_playground_sessions_workspace_id" ON "public"."playground_sessions" ("workspace_id");
-- Create "refresh_tokens" table
CREATE TABLE "public"."refresh_tokens" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "user_id" uuid NOT NULL,
  "token" text NOT NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_refresh_tokens_deleted_at" to table: "refresh_tokens"
CREATE INDEX "idx_refresh_tokens_deleted_at" ON "public"."refresh_tokens" ("deleted_at");
-- Create "scan_jobs" table
CREATE TABLE "public"."scan_jobs" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "scan_id" bigint NOT NULL,
  "status" character varying(50) NOT NULL DEFAULT 'pending',
  "job_type" character varying(50) NOT NULL,
  "priority" bigint NULL DEFAULT 0,
  "workspace_id" bigint NOT NULL,
  "worker_id" character varying(255) NULL,
  "claimed_at" timestamptz NULL,
  "target_host" character varying(255) NULL,
  "url" text NULL,
  "method" character varying(10) NULL,
  "history_id" bigint NULL,
  "web_socket_connection_id" bigint NULL,
  "payload" jsonb NULL,
  "attempts" bigint NULL DEFAULT 0,
  "max_attempts" bigint NULL DEFAULT 3,
  "started_at" timestamptz NULL,
  "completed_at" timestamptz NULL,
  "max_duration" bigint NULL DEFAULT 1800000000000,
  "error_type" character varying(100) NULL,
  "error_message" text NULL,
  "http_status" bigint NULL,
  "issues_found" bigint NULL DEFAULT 0,
  "checkpoint" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_scan_jobs_deleted_at" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_deleted_at" ON "public"."scan_jobs" ("deleted_at");
-- Create index "idx_scan_jobs_history_id" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_history_id" ON "public"."scan_jobs" ("history_id");
-- Create index "idx_scan_jobs_job_type" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_job_type" ON "public"."scan_jobs" ("job_type");
-- Create index "idx_scan_jobs_priority" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_priority" ON "public"."scan_jobs" ("priority");
-- Create index "idx_scan_jobs_scan_id" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_scan_id" ON "public"."scan_jobs" ("scan_id");
-- Create index "idx_scan_jobs_status" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_status" ON "public"."scan_jobs" ("status");
-- Create index "idx_scan_jobs_target_host" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_target_host" ON "public"."scan_jobs" ("target_host");
-- Create index "idx_scan_jobs_web_socket_connection_id" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_web_socket_connection_id" ON "public"."scan_jobs" ("web_socket_connection_id");
-- Create index "idx_scan_jobs_worker_id" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_worker_id" ON "public"."scan_jobs" ("worker_id");
-- Create index "idx_scan_jobs_workspace_id" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_workspace_id" ON "public"."scan_jobs" ("workspace_id");
-- Create "scans" table
CREATE TABLE "public"."scans" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "workspace_id" bigint NOT NULL,
  "title" character varying(255) NULL,
  "status" character varying(50) NOT NULL DEFAULT 'pending',
  "phase" character varying(50) NULL,
  "previous_status" character varying(50) NULL,
  "options" text NULL,
  "max_rps" bigint NULL,
  "max_concurrent_jobs" bigint NULL,
  "consecutive_failures" bigint NULL DEFAULT 0,
  "last_failure_at" timestamptz NULL,
  "throttled_until" timestamptz NULL,
  "total_jobs_count" bigint NULL DEFAULT 0,
  "pending_jobs_count" bigint NULL DEFAULT 0,
  "running_jobs_count" bigint NULL DEFAULT 0,
  "completed_jobs_count" bigint NULL DEFAULT 0,
  "failed_jobs_count" bigint NULL DEFAULT 0,
  "started_at" timestamptz NULL,
  "paused_at" timestamptz NULL,
  "completed_at" timestamptz NULL,
  "checkpoint" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_scans_deleted_at" to table: "scans"
CREATE INDEX "idx_scans_deleted_at" ON "public"."scans" ("deleted_at");
-- Create index "idx_scans_status" to table: "scans"
CREATE INDEX "idx_scans_status" ON "public"."scans" ("status");
-- Create index "idx_scans_workspace_id" to table: "scans"
CREATE INDEX "idx_scans_workspace_id" ON "public"."scans" ("workspace_id");
-- Create "stored_browser_actions" table
CREATE TABLE "public"."stored_browser_actions" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "title" text NULL,
  "actions" text NULL,
  "scope" text NULL,
  "workspace_id" bigint NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_stored_browser_actions_deleted_at" to table: "stored_browser_actions"
CREATE INDEX "idx_stored_browser_actions_deleted_at" ON "public"."stored_browser_actions" ("deleted_at");
-- Create index "idx_stored_browser_actions_scope" to table: "stored_browser_actions"
CREATE INDEX "idx_stored_browser_actions_scope" ON "public"."stored_browser_actions" ("scope");
-- Create index "idx_stored_browser_actions_title" to table: "stored_browser_actions"
CREATE INDEX "idx_stored_browser_actions_title" ON "public"."stored_browser_actions" ("title");
-- Create index "idx_stored_browser_actions_workspace_id" to table: "stored_browser_actions"
CREATE INDEX "idx_stored_browser_actions_workspace_id" ON "public"."stored_browser_actions" ("workspace_id");
-- Create "task_jobs" table
CREATE TABLE "public"."task_jobs" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "title" text NULL,
  "task_id" bigint NULL,
  "status" text NULL,
  "started_at" timestamptz NULL,
  "completed_at" timestamptz NULL,
  "history_id" bigint NULL,
  "websocket_connection_id" bigint NULL,
  "url" text NULL,
  "method" text NULL,
  "original_status_code" bigint NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_task_jobs_deleted_at" to table: "task_jobs"
CREATE INDEX "idx_task_jobs_deleted_at" ON "public"."task_jobs" ("deleted_at");
-- Create index "idx_task_jobs_status" to table: "task_jobs"
CREATE INDEX "idx_task_jobs_status" ON "public"."task_jobs" ("status");
-- Create index "idx_task_jobs_websocket_connection_id" to table: "task_jobs"
CREATE INDEX "idx_task_jobs_websocket_connection_id" ON "public"."task_jobs" ("websocket_connection_id");
-- Create "tasks" table
CREATE TABLE "public"."tasks" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "title" text NULL,
  "type" text NULL,
  "status" text NULL,
  "started_at" timestamptz NULL,
  "finished_at" timestamptz NULL,
  "workspace_id" bigint NULL,
  "playground_session_id" bigint NULL,
  "scan_options" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_tasks_deleted_at" to table: "tasks"
CREATE INDEX "idx_tasks_deleted_at" ON "public"."tasks" ("deleted_at");
-- Create index "idx_tasks_playground_session_id" to table: "tasks"
CREATE INDEX "idx_tasks_playground_session_id" ON "public"."tasks" ("playground_session_id");
-- Create index "idx_tasks_status" to table: "tasks"
CREATE INDEX "idx_tasks_status" ON "public"."tasks" ("status");
-- Create index "idx_tasks_type" to table: "tasks"
CREATE INDEX "idx_tasks_type" ON "public"."tasks" ("type");
-- Create index "idx_tasks_workspace_id" to table: "tasks"
CREATE INDEX "idx_tasks_workspace_id" ON "public"."tasks" ("workspace_id");
-- Create "users" table
CREATE TABLE "public"."users" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "email" character varying(255) NOT NULL,
  "password_hash" text NULL,
  "active" boolean NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "uni_users_email" UNIQUE ("email")
);
-- Create index "idx_users_deleted_at" to table: "users"
CREATE INDEX "idx_users_deleted_at" ON "public"."users" ("deleted_at");
-- Create "web_socket_connections" table
CREATE TABLE "public"."web_socket_connections" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "url" text NULL,
  "request_headers" jsonb NULL,
  "response_headers" jsonb NULL,
  "status_code" bigint NULL,
  "status_text" text NULL,
  "closed_at" timestamptz NULL,
  "workspace_id" bigint NULL,
  "task_id" bigint NULL,
  "scan_id" bigint NULL,
  "scan_job_id" bigint NULL,
  "source" text NULL,
  "upgrade_request_id" bigint NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_web_socket_connections_deleted_at" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_deleted_at" ON "public"."web_socket_connections" ("deleted_at");
-- Create index "idx_web_socket_connections_scan_id" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_scan_id" ON "public"."web_socket_connections" ("scan_id");
-- Create index "idx_web_socket_connections_scan_job_id" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_scan_job_id" ON "public"."web_socket_connections" ("scan_job_id");
-- Create index "idx_web_socket_connections_status_code" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_status_code" ON "public"."web_socket_connections" ("status_code");
-- Create index "idx_web_socket_connections_task_id" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_task_id" ON "public"."web_socket_connections" ("task_id");
-- Create index "idx_web_socket_connections_upgrade_request_id" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_upgrade_request_id" ON "public"."web_socket_connections" ("upgrade_request_id");
-- Create "web_socket_messages" table
CREATE TABLE "public"."web_socket_messages" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "connection_id" bigint NULL,
  "opcode" numeric NULL,
  "mask" boolean NULL,
  "payload_data" text NULL,
  "is_binary" boolean NULL,
  "timestamp" timestamptz NULL,
  "direction" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_web_socket_messages_deleted_at" to table: "web_socket_messages"
CREATE INDEX "idx_web_socket_messages_deleted_at" ON "public"."web_socket_messages" ("deleted_at");
-- Create index "idx_web_socket_messages_direction" to table: "web_socket_messages"
CREATE INDEX "idx_web_socket_messages_direction" ON "public"."web_socket_messages" ("direction");
-- Create index "idx_web_socket_messages_is_binary" to table: "web_socket_messages"
CREATE INDEX "idx_web_socket_messages_is_binary" ON "public"."web_socket_messages" ("is_binary");
-- Create index "idx_web_socket_messages_mask" to table: "web_socket_messages"
CREATE INDEX "idx_web_socket_messages_mask" ON "public"."web_socket_messages" ("mask");
-- Create "worker_nodes" table
CREATE TABLE "public"."worker_nodes" (
  "id" character varying(255) NOT NULL,
  "hostname" character varying(255) NULL,
  "worker_count" bigint NULL,
  "status" character varying(50) NULL,
  "started_at" timestamptz NULL,
  "last_seen_at" timestamptz NULL,
  "jobs_claimed" bigint NULL,
  "jobs_completed" bigint NULL,
  "jobs_failed" bigint NULL,
  "version" character varying(50) NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_worker_nodes_hostname" to table: "worker_nodes"
CREATE INDEX "idx_worker_nodes_hostname" ON "public"."worker_nodes" ("hostname");
-- Create index "idx_worker_nodes_last_seen_at" to table: "worker_nodes"
CREATE INDEX "idx_worker_nodes_last_seen_at" ON "public"."worker_nodes" ("last_seen_at");
-- Create index "idx_worker_nodes_status" to table: "worker_nodes"
CREATE INDEX "idx_worker_nodes_status" ON "public"."worker_nodes" ("status");
-- Create "workspace_cookies" table
CREATE TABLE "public"."workspace_cookies" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "workspace_id" bigint NULL,
  "name" text NULL,
  "value" text NULL,
  "domain" text NULL,
  "path" text NULL,
  "expires" timestamptz NULL,
  "max_age" bigint NULL,
  "secure" boolean NULL,
  "http_only" boolean NULL,
  "same_site" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_workspace_cookies_deleted_at" to table: "workspace_cookies"
CREATE INDEX "idx_workspace_cookies_deleted_at" ON "public"."workspace_cookies" ("deleted_at");
-- Create index "idx_workspace_cookies_domain" to table: "workspace_cookies"
CREATE INDEX "idx_workspace_cookies_domain" ON "public"."workspace_cookies" ("domain");
-- Create index "idx_workspace_cookies_name" to table: "workspace_cookies"
CREATE INDEX "idx_workspace_cookies_name" ON "public"."workspace_cookies" ("name");
-- Create index "idx_workspace_cookies_workspace_id" to table: "workspace_cookies"
CREATE INDEX "idx_workspace_cookies_workspace_id" ON "public"."workspace_cookies" ("workspace_id");
-- Create "workspaces" table
CREATE TABLE "public"."workspaces" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "code" text NULL,
  "title" text NULL,
  "description" text NULL,
  PRIMARY KEY ("id")
);
-- Create index "idx_workspaces_deleted_at" to table: "workspaces"
CREATE INDEX "idx_workspaces_deleted_at" ON "public"."workspaces" ("deleted_at");
-- Modify "histories" table
ALTER TABLE "public"."histories" ADD CONSTRAINT "fk_histories_scan_job" FOREIGN KEY ("scan_job_id") REFERENCES "public"."scan_jobs" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_histories_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_playground_sessions_histories" FOREIGN KEY ("playground_session_id") REFERENCES "public"."playground_sessions" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, ADD CONSTRAINT "fk_scans_histories" FOREIGN KEY ("scan_id") REFERENCES "public"."scans" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_tasks_histories" FOREIGN KEY ("task_id") REFERENCES "public"."tasks" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "issue_requests" table
ALTER TABLE "public"."issue_requests" ADD CONSTRAINT "fk_issue_requests_history" FOREIGN KEY ("history_id") REFERENCES "public"."histories" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_issue_requests_issue" FOREIGN KEY ("issue_id") REFERENCES "public"."issues" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "issues" table
ALTER TABLE "public"."issues" ADD CONSTRAINT "fk_issues_scan" FOREIGN KEY ("scan_id") REFERENCES "public"."scans" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_issues_scan_job" FOREIGN KEY ("scan_job_id") REFERENCES "public"."scan_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_issues_task_job" FOREIGN KEY ("task_job_id") REFERENCES "public"."task_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_issues_web_socket_connection" FOREIGN KEY ("websocket_connection_id") REFERENCES "public"."web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_issues_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_tasks_issues" FOREIGN KEY ("task_id") REFERENCES "public"."tasks" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "json_web_token_histories" table
ALTER TABLE "public"."json_web_token_histories" ADD CONSTRAINT "fk_json_web_token_histories_history" FOREIGN KEY ("history_id") REFERENCES "public"."histories" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_json_web_token_histories_json_web_token" FOREIGN KEY ("json_web_token_id") REFERENCES "public"."json_web_tokens" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "json_web_token_websocket_connections" table
ALTER TABLE "public"."json_web_token_websocket_connections" ADD CONSTRAINT "fk_json_web_token_websocket_connections_json_web_token" FOREIGN KEY ("json_web_token_id") REFERENCES "public"."json_web_tokens" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_json_web_token_websocket_connections_web_socket_connection" FOREIGN KEY ("web_socket_connection_id") REFERENCES "public"."web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "json_web_tokens" table
ALTER TABLE "public"."json_web_tokens" ADD CONSTRAINT "fk_json_web_tokens_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "oob_interactions" table
ALTER TABLE "public"."oob_interactions" ADD CONSTRAINT "fk_issues_interactions" FOREIGN KEY ("issue_id") REFERENCES "public"."issues" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_oob_interactions_oob_test" FOREIGN KEY ("oob_test_id") REFERENCES "public"."oob_tests" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_oob_interactions_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "oob_tests" table
ALTER TABLE "public"."oob_tests" ADD CONSTRAINT "fk_oob_tests_history_item" FOREIGN KEY ("history_id") REFERENCES "public"."histories" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_oob_tests_issue" FOREIGN KEY ("issue_id") REFERENCES "public"."issues" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_oob_tests_scan" FOREIGN KEY ("scan_id") REFERENCES "public"."scans" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_oob_tests_scan_job" FOREIGN KEY ("scan_job_id") REFERENCES "public"."scan_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_oob_tests_task" FOREIGN KEY ("task_id") REFERENCES "public"."tasks" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_oob_tests_task_job" FOREIGN KEY ("task_job_id") REFERENCES "public"."task_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_oob_tests_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "playground_collections" table
ALTER TABLE "public"."playground_collections" ADD CONSTRAINT "fk_playground_collections_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "playground_sessions" table
ALTER TABLE "public"."playground_sessions" ADD CONSTRAINT "fk_playground_collections_sessions" FOREIGN KEY ("collection_id") REFERENCES "public"."playground_collections" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_playground_sessions_original_request" FOREIGN KEY ("original_request_id") REFERENCES "public"."histories" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_playground_sessions_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "scan_jobs" table
ALTER TABLE "public"."scan_jobs" ADD CONSTRAINT "fk_scan_jobs_history" FOREIGN KEY ("history_id") REFERENCES "public"."histories" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_scan_jobs_web_socket_connection" FOREIGN KEY ("web_socket_connection_id") REFERENCES "public"."web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_scans_jobs" FOREIGN KEY ("scan_id") REFERENCES "public"."scans" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "scans" table
ALTER TABLE "public"."scans" ADD CONSTRAINT "fk_scans_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "stored_browser_actions" table
ALTER TABLE "public"."stored_browser_actions" ADD CONSTRAINT "fk_stored_browser_actions_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "task_jobs" table
ALTER TABLE "public"."task_jobs" ADD CONSTRAINT "fk_task_jobs_history" FOREIGN KEY ("history_id") REFERENCES "public"."histories" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_task_jobs_task" FOREIGN KEY ("task_id") REFERENCES "public"."tasks" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_task_jobs_web_socket_connection" FOREIGN KEY ("websocket_connection_id") REFERENCES "public"."web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Modify "tasks" table
ALTER TABLE "public"."tasks" ADD CONSTRAINT "fk_tasks_playground_session" FOREIGN KEY ("playground_session_id") REFERENCES "public"."playground_sessions" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_tasks_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "web_socket_connections" table
ALTER TABLE "public"."web_socket_connections" ADD CONSTRAINT "fk_web_socket_connections_scan" FOREIGN KEY ("scan_id") REFERENCES "public"."scans" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_web_socket_connections_scan_job" FOREIGN KEY ("scan_job_id") REFERENCES "public"."scan_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_web_socket_connections_task" FOREIGN KEY ("task_id") REFERENCES "public"."tasks" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_web_socket_connections_upgrade_request" FOREIGN KEY ("upgrade_request_id") REFERENCES "public"."histories" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_web_socket_connections_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "web_socket_messages" table
ALTER TABLE "public"."web_socket_messages" ADD CONSTRAINT "fk_web_socket_connections_messages" FOREIGN KEY ("connection_id") REFERENCES "public"."web_socket_connections" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "workspace_cookies" table
ALTER TABLE "public"."workspace_cookies" ADD CONSTRAINT "fk_workspace_cookies_workspace" FOREIGN KEY ("workspace_id") REFERENCES "public"."workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
