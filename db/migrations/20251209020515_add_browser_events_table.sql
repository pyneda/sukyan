-- Modify "scans" table
ALTER TABLE "scans" ADD COLUMN "capture_browser_events" boolean NULL DEFAULT false;
-- Create "browser_events" table
CREATE TABLE "browser_events" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "event_type" character varying(50) NOT NULL,
  "category" character varying(50) NOT NULL,
  "url" text NULL,
  "description" text NULL,
  "data" jsonb NULL,
  "content_hash" character varying(64) NOT NULL,
  "occurrence_count" bigint NULL DEFAULT 1,
  "first_seen_at" timestamptz NULL,
  "last_seen_at" timestamptz NULL,
  "workspace_id" bigint NOT NULL,
  "scan_id" bigint NULL,
  "scan_job_id" bigint NULL,
  "history_id" bigint NULL,
  "task_id" bigint NULL,
  "source" character varying(50) NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_browser_events_history" FOREIGN KEY ("history_id") REFERENCES "histories" ("id") ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT "fk_browser_events_scan" FOREIGN KEY ("scan_id") REFERENCES "scans" ("id") ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT "fk_browser_events_scan_job" FOREIGN KEY ("scan_job_id") REFERENCES "scan_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT "fk_browser_events_task" FOREIGN KEY ("task_id") REFERENCES "tasks" ("id") ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT "fk_browser_events_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_browser_events_category" to table: "browser_events"
CREATE INDEX "idx_browser_events_category" ON "browser_events" ("category");
-- Create index "idx_browser_events_content_hash" to table: "browser_events"
CREATE INDEX "idx_browser_events_content_hash" ON "browser_events" ("content_hash");
-- Create index "idx_browser_events_deleted_at" to table: "browser_events"
CREATE INDEX "idx_browser_events_deleted_at" ON "browser_events" ("deleted_at");
-- Create index "idx_browser_events_event_type" to table: "browser_events"
CREATE INDEX "idx_browser_events_event_type" ON "browser_events" ("event_type");
-- Create index "idx_browser_events_history_id" to table: "browser_events"
CREATE INDEX "idx_browser_events_history_id" ON "browser_events" ("history_id");
-- Create index "idx_browser_events_scan_id" to table: "browser_events"
CREATE INDEX "idx_browser_events_scan_id" ON "browser_events" ("scan_id");
-- Create index "idx_browser_events_scan_job_id" to table: "browser_events"
CREATE INDEX "idx_browser_events_scan_job_id" ON "browser_events" ("scan_job_id");
-- Create index "idx_browser_events_source" to table: "browser_events"
CREATE INDEX "idx_browser_events_source" ON "browser_events" ("source");
-- Create index "idx_browser_events_task_id" to table: "browser_events"
CREATE INDEX "idx_browser_events_task_id" ON "browser_events" ("task_id");
-- Create index "idx_browser_events_url" to table: "browser_events"
CREATE INDEX "idx_browser_events_url" ON "browser_events" ("url");
-- Create index "idx_browser_events_workspace_id" to table: "browser_events"
CREATE INDEX "idx_browser_events_workspace_id" ON "browser_events" ("workspace_id");
