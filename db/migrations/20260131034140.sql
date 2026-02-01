-- Modify "api_definitions" table
ALTER TABLE "api_definitions" ADD COLUMN "global_security_json" jsonb NULL;
-- Create "api_definition_security_schemes" table
CREATE TABLE "api_definition_security_schemes" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "definition_id" uuid NOT NULL,
  "name" character varying(255) NOT NULL,
  "type" character varying(50) NOT NULL,
  "scheme" character varying(50) NULL,
  "in" character varying(50) NULL,
  "parameter_name" character varying(255) NULL,
  "bearer_format" character varying(50) NULL,
  "description" text NULL,
  "open_id_connect_url" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_definitions_security_schemes" FOREIGN KEY ("definition_id") REFERENCES "api_definitions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_definition_security_schemes_definition_id" to table: "api_definition_security_schemes"
CREATE INDEX "idx_api_definition_security_schemes_definition_id" ON "api_definition_security_schemes" ("definition_id");
-- Create index "idx_api_definition_security_schemes_deleted_at" to table: "api_definition_security_schemes"
CREATE INDEX "idx_api_definition_security_schemes_deleted_at" ON "api_definition_security_schemes" ("deleted_at");
-- Drop index "idx_scan_api_definitions_api_definition_id" from table: "scan_api_definitions"
DROP INDEX "idx_scan_api_definitions_api_definition_id";
-- Drop index "idx_scan_api_definitions_scan_id" from table: "scan_api_definitions"
DROP INDEX "idx_scan_api_definitions_scan_id";
-- Modify "scan_api_definitions" table
ALTER TABLE "scan_api_definitions" DROP CONSTRAINT "scan_api_definitions_api_definition_id_fkey", DROP CONSTRAINT "scan_api_definitions_scan_id_fkey", ALTER COLUMN "created_at" DROP DEFAULT, ADD CONSTRAINT "fk_scan_api_definitions_api_definition" FOREIGN KEY ("api_definition_id") REFERENCES "api_definitions" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION, ADD CONSTRAINT "fk_scan_api_definitions_scan" FOREIGN KEY ("scan_id") REFERENCES "scans" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION;
