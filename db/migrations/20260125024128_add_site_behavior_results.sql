-- Create "site_behavior_results" table
CREATE TABLE "site_behavior_results" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "scan_id" bigint NOT NULL,
  "scan_job_id" bigint NULL,
  "workspace_id" bigint NOT NULL,
  "base_url" text NOT NULL,
  "not_found_returns404" boolean NULL,
  "not_found_changes" boolean NULL,
  "not_found_common_hash" character varying(255) NULL,
  "not_found_status_code" bigint NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_site_behavior_results_scan" FOREIGN KEY ("scan_id") REFERENCES "scans" ("id") ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT "fk_site_behavior_results_scan_job" FOREIGN KEY ("scan_job_id") REFERENCES "scan_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT "fk_site_behavior_results_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_site_behavior_results_deleted_at" to table: "site_behavior_results"
CREATE INDEX "idx_site_behavior_results_deleted_at" ON "site_behavior_results" ("deleted_at");
-- Create index "idx_site_behavior_results_scan_job_id" to table: "site_behavior_results"
CREATE INDEX "idx_site_behavior_results_scan_job_id" ON "site_behavior_results" ("scan_job_id");
-- Create index "idx_site_behavior_results_workspace_id" to table: "site_behavior_results"
CREATE INDEX "idx_site_behavior_results_workspace_id" ON "site_behavior_results" ("workspace_id");
-- Create index "idx_site_behavior_scan_url" to table: "site_behavior_results"
CREATE UNIQUE INDEX "idx_site_behavior_scan_url" ON "site_behavior_results" ("scan_id", "base_url");
