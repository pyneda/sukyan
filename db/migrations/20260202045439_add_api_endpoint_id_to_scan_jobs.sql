-- Drop index "idx_api_definitions_workspace_source_url" from table: "api_definitions"
DROP INDEX "idx_api_definitions_workspace_source_url";
-- Modify "scan_jobs" table
ALTER TABLE "scan_jobs" ADD COLUMN "api_endpoint_id" uuid NULL, ADD CONSTRAINT "fk_scan_jobs_api_endpoint" FOREIGN KEY ("api_endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Create index "idx_scan_jobs_api_endpoint_id" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_api_endpoint_id" ON "scan_jobs" ("api_endpoint_id");
-- Drop "api_endpoint_parameters" table
DROP TABLE "api_endpoint_parameters";
-- Drop "api_endpoint_securities" table
DROP TABLE "api_endpoint_securities";
-- Drop "api_request_variations" table
DROP TABLE "api_request_variations";
