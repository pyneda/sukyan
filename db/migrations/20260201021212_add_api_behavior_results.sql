-- Create index "idx_endpoint_def_enabled" to table: "api_endpoints"
CREATE INDEX "idx_endpoint_def_enabled" ON "api_endpoints" ("definition_id", "enabled");
-- Create index "idx_api_scan_def_scan" to table: "api_scans"
CREATE INDEX "idx_api_scan_def_scan" ON "api_scans" ("definition_id", "scan_id");
-- Create "api_behavior_results" table
CREATE TABLE "api_behavior_results" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "scan_id" bigint NOT NULL,
  "scan_job_id" bigint NULL,
  "workspace_id" bigint NOT NULL,
  "definition_id" uuid NOT NULL,
  "definition_type" character varying(50) NOT NULL,
  "not_found_fingerprints" jsonb NULL,
  "unauthenticated_fingerprints" jsonb NULL,
  "invalid_content_type_fingerprints" jsonb NULL,
  "malformed_body_fingerprints" jsonb NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_behavior_results_definition" FOREIGN KEY ("definition_id") REFERENCES "api_definitions" ("id") ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT "fk_api_behavior_results_scan" FOREIGN KEY ("scan_id") REFERENCES "scans" ("id") ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT "fk_api_behavior_results_scan_job" FOREIGN KEY ("scan_job_id") REFERENCES "scan_jobs" ("id") ON UPDATE CASCADE ON DELETE SET NULL,
  CONSTRAINT "fk_api_behavior_results_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_api_behavior_results_deleted_at" to table: "api_behavior_results"
CREATE INDEX "idx_api_behavior_results_deleted_at" ON "api_behavior_results" ("deleted_at");
-- Create index "idx_api_behavior_results_scan_job_id" to table: "api_behavior_results"
CREATE INDEX "idx_api_behavior_results_scan_job_id" ON "api_behavior_results" ("scan_job_id");
-- Create index "idx_api_behavior_results_workspace_id" to table: "api_behavior_results"
CREATE INDEX "idx_api_behavior_results_workspace_id" ON "api_behavior_results" ("workspace_id");
-- Create index "idx_api_behavior_scan_def" to table: "api_behavior_results"
CREATE UNIQUE INDEX "idx_api_behavior_scan_def" ON "api_behavior_results" ("scan_id", "definition_id");
