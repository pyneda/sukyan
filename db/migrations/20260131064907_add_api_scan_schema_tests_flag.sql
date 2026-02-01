-- Modify "api_scan_endpoints" table
ALTER TABLE "api_scan_endpoints" DROP CONSTRAINT "fk_api_scan_endpoints_api_endpoint", DROP CONSTRAINT "fk_api_scan_endpoints_api_scan", ADD CONSTRAINT "fk_api_scan_endpoints_api_endpoint" FOREIGN KEY ("api_endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_api_scan_endpoints_api_scan" FOREIGN KEY ("api_scan_id") REFERENCES "api_scans" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "api_scans" table
ALTER TABLE "api_scans" ADD COLUMN "run_schema_tests" boolean NULL DEFAULT false;
-- Modify "scan_api_definitions" table
ALTER TABLE "scan_api_definitions" DROP CONSTRAINT "fk_scan_api_definitions_api_definition", DROP CONSTRAINT "fk_scan_api_definitions_scan", ADD CONSTRAINT "fk_scan_api_definitions_api_definition" FOREIGN KEY ("api_definition_id") REFERENCES "api_definitions" ("id") ON UPDATE CASCADE ON DELETE CASCADE, ADD CONSTRAINT "fk_scan_api_definitions_scan" FOREIGN KEY ("scan_id") REFERENCES "scans" ("id") ON UPDATE CASCADE ON DELETE CASCADE;
-- Modify "api_definitions" table
ALTER TABLE "api_definitions" ADD CONSTRAINT "fk_api_definitions_source_history" FOREIGN KEY ("source_history_id") REFERENCES "histories" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Modify "histories" table
ALTER TABLE "histories" ADD CONSTRAINT "fk_histories_api_definition" FOREIGN KEY ("api_definition_id") REFERENCES "api_definitions" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_histories_api_endpoint" FOREIGN KEY ("api_endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Modify "issues" table
ALTER TABLE "issues" ADD CONSTRAINT "fk_issues_api_definition" FOREIGN KEY ("api_definition_id") REFERENCES "api_definitions" ("id") ON UPDATE CASCADE ON DELETE SET NULL, ADD CONSTRAINT "fk_issues_api_endpoint" FOREIGN KEY ("api_endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
