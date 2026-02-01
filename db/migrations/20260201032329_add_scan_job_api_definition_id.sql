-- Modify "scan_jobs" table
ALTER TABLE "scan_jobs" ADD COLUMN "api_definition_id" uuid NULL, ADD CONSTRAINT "fk_scan_jobs_api_definition" FOREIGN KEY ("api_definition_id") REFERENCES "api_definitions" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Create index "idx_scan_jobs_api_definition_id" to table: "scan_jobs"
CREATE INDEX "idx_scan_jobs_api_definition_id" ON "scan_jobs" ("api_definition_id");
