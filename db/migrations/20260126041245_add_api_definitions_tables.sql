-- Modify "histories" table
ALTER TABLE "histories" ADD COLUMN "api_definition_id" uuid NULL, ADD COLUMN "api_endpoint_id" uuid NULL;
-- Create index "idx_histories_api_definition_id" to table: "histories"
CREATE INDEX "idx_histories_api_definition_id" ON "histories" ("api_definition_id");
-- Create index "idx_histories_api_endpoint_id" to table: "histories"
CREATE INDEX "idx_histories_api_endpoint_id" ON "histories" ("api_endpoint_id");
-- Modify "issues" table
ALTER TABLE "issues" ADD COLUMN "api_definition_id" uuid NULL, ADD COLUMN "api_endpoint_id" uuid NULL;
-- Create index "idx_issues_api_definition_id" to table: "issues"
CREATE INDEX "idx_issues_api_definition_id" ON "issues" ("api_definition_id");
-- Create index "idx_issues_api_endpoint_id" to table: "issues"
CREATE INDEX "idx_issues_api_endpoint_id" ON "issues" ("api_endpoint_id");
-- Create "api_auth_configs" table
CREATE TABLE "api_auth_configs" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "workspace_id" bigint NOT NULL,
  "name" character varying(255) NULL,
  "type" character varying(50) NOT NULL,
  "username" character varying(255) NULL,
  "password" character varying(500) NULL,
  "token" text NULL,
  "token_prefix" character varying(50) NULL DEFAULT 'Bearer',
  "api_key_name" character varying(255) NULL,
  "api_key_value" text NULL,
  "api_key_location" character varying(50) NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_auth_configs_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_auth_configs_deleted_at" to table: "api_auth_configs"
CREATE INDEX "idx_api_auth_configs_deleted_at" ON "api_auth_configs" ("deleted_at");
-- Create index "idx_api_auth_configs_workspace_id" to table: "api_auth_configs"
CREATE INDEX "idx_api_auth_configs_workspace_id" ON "api_auth_configs" ("workspace_id");
-- Create "api_auth_headers" table
CREATE TABLE "api_auth_headers" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "auth_config_id" uuid NOT NULL,
  "header_name" character varying(255) NOT NULL,
  "header_value" text NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_auth_configs_custom_headers" FOREIGN KEY ("auth_config_id") REFERENCES "api_auth_configs" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_auth_headers_auth_config_id" to table: "api_auth_headers"
CREATE INDEX "idx_api_auth_headers_auth_config_id" ON "api_auth_headers" ("auth_config_id");
-- Create index "idx_api_auth_headers_deleted_at" to table: "api_auth_headers"
CREATE INDEX "idx_api_auth_headers_deleted_at" ON "api_auth_headers" ("deleted_at");
-- Create "api_definitions" table
CREATE TABLE "api_definitions" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "workspace_id" bigint NOT NULL,
  "name" character varying(255) NULL,
  "type" character varying(50) NOT NULL,
  "status" character varying(50) NULL DEFAULT 'parsed',
  "source_url" text NULL,
  "base_url" text NULL,
  "source_history_id" bigint NULL,
  "raw_definition" bytea NULL,
  "auto_discovered" boolean NULL DEFAULT false,
  "scan_id" bigint NULL,
  "auth_config_id" uuid NULL,
  "endpoint_count" bigint NULL DEFAULT 0,
  "open_api_version" character varying(20) NULL,
  "open_api_title" character varying(255) NULL,
  "open_api_servers" bigint NULL DEFAULT 0,
  "graph_ql_query_count" bigint NULL DEFAULT 0,
  "graph_ql_mutation_count" bigint NULL DEFAULT 0,
  "graph_ql_subscription_count" bigint NULL DEFAULT 0,
  "graph_ql_type_count" bigint NULL DEFAULT 0,
  "wsdl_target_namespace" text NULL,
  "wsdl_service_count" bigint NULL DEFAULT 0,
  "wsdl_port_count" bigint NULL DEFAULT 0,
  "wsdlsoap_version" character varying(10) NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_definitions_auth_config" FOREIGN KEY ("auth_config_id") REFERENCES "api_auth_configs" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "fk_api_definitions_scan" FOREIGN KEY ("scan_id") REFERENCES "scans" ("id") ON UPDATE NO ACTION ON DELETE SET NULL,
  CONSTRAINT "fk_api_definitions_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_definitions_auth_config_id" to table: "api_definitions"
CREATE INDEX "idx_api_definitions_auth_config_id" ON "api_definitions" ("auth_config_id");
-- Create index "idx_api_definitions_deleted_at" to table: "api_definitions"
CREATE INDEX "idx_api_definitions_deleted_at" ON "api_definitions" ("deleted_at");
-- Create index "idx_api_definitions_scan_id" to table: "api_definitions"
CREATE INDEX "idx_api_definitions_scan_id" ON "api_definitions" ("scan_id");
-- Create index "idx_api_definitions_source_history_id" to table: "api_definitions"
CREATE INDEX "idx_api_definitions_source_history_id" ON "api_definitions" ("source_history_id");
-- Create index "idx_api_definitions_status" to table: "api_definitions"
CREATE INDEX "idx_api_definitions_status" ON "api_definitions" ("status");
-- Create index "idx_api_definitions_type" to table: "api_definitions"
CREATE INDEX "idx_api_definitions_type" ON "api_definitions" ("type");
-- Create index "idx_api_definitions_workspace_id" to table: "api_definitions"
CREATE INDEX "idx_api_definitions_workspace_id" ON "api_definitions" ("workspace_id");
-- Create "api_endpoints" table
CREATE TABLE "api_endpoints" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "definition_id" uuid NOT NULL,
  "operation_id" character varying(255) NULL,
  "name" character varying(255) NULL,
  "summary" character varying(500) NULL,
  "description" text NULL,
  "enabled" boolean NULL DEFAULT true,
  "last_scanned_at" timestamptz NULL,
  "issues_found" bigint NULL DEFAULT 0,
  "method" character varying(10) NULL,
  "path" text NULL,
  "operation_type" character varying(50) NULL,
  "return_type" character varying(255) NULL,
  "service_name" character varying(255) NULL,
  "port_name" character varying(255) NULL,
  "soap_action" character varying(500) NULL,
  "binding_style" character varying(50) NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_definitions_endpoints" FOREIGN KEY ("definition_id") REFERENCES "api_definitions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_endpoints_definition_id" to table: "api_endpoints"
CREATE INDEX "idx_api_endpoints_definition_id" ON "api_endpoints" ("definition_id");
-- Create index "idx_api_endpoints_deleted_at" to table: "api_endpoints"
CREATE INDEX "idx_api_endpoints_deleted_at" ON "api_endpoints" ("deleted_at");
-- Create index "idx_api_endpoints_enabled" to table: "api_endpoints"
CREATE INDEX "idx_api_endpoints_enabled" ON "api_endpoints" ("enabled");
-- Create index "idx_api_endpoints_method" to table: "api_endpoints"
CREATE INDEX "idx_api_endpoints_method" ON "api_endpoints" ("method");
-- Create index "idx_api_endpoints_operation_id" to table: "api_endpoints"
CREATE INDEX "idx_api_endpoints_operation_id" ON "api_endpoints" ("operation_id");
-- Create "api_endpoint_parameters" table
CREATE TABLE "api_endpoint_parameters" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "endpoint_id" uuid NOT NULL,
  "name" character varying(255) NOT NULL,
  "location" character varying(50) NOT NULL,
  "required" boolean NULL DEFAULT false,
  "data_type" character varying(50) NULL,
  "format" character varying(50) NULL,
  "pattern" character varying(500) NULL,
  "min_length" bigint NULL,
  "max_length" bigint NULL,
  "minimum" numeric NULL,
  "maximum" numeric NULL,
  "enum_values" text NULL,
  "default_value" text NULL,
  "example" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_endpoints_parameters" FOREIGN KEY ("endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_endpoint_parameters_deleted_at" to table: "api_endpoint_parameters"
CREATE INDEX "idx_api_endpoint_parameters_deleted_at" ON "api_endpoint_parameters" ("deleted_at");
-- Create index "idx_api_endpoint_parameters_endpoint_id" to table: "api_endpoint_parameters"
CREATE INDEX "idx_api_endpoint_parameters_endpoint_id" ON "api_endpoint_parameters" ("endpoint_id");
-- Create "api_endpoint_securities" table
CREATE TABLE "api_endpoint_securities" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "endpoint_id" uuid NOT NULL,
  "scheme_name" character varying(255) NOT NULL,
  "scopes" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_endpoints_security_schemes" FOREIGN KEY ("endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_endpoint_securities_deleted_at" to table: "api_endpoint_securities"
CREATE INDEX "idx_api_endpoint_securities_deleted_at" ON "api_endpoint_securities" ("deleted_at");
-- Create index "idx_api_endpoint_securities_endpoint_id" to table: "api_endpoint_securities"
CREATE INDEX "idx_api_endpoint_securities_endpoint_id" ON "api_endpoint_securities" ("endpoint_id");
-- Create "api_request_variations" table
CREATE TABLE "api_request_variations" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "endpoint_id" uuid NOT NULL,
  "label" character varying(255) NOT NULL,
  "description" text NULL,
  "url" text NULL,
  "method" character varying(10) NULL,
  "headers" bytea NULL,
  "body" bytea NULL,
  "content_type" character varying(100) NULL,
  "query" text NULL,
  "variables" bytea NULL,
  "operation_name" character varying(255) NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_endpoints_request_variations" FOREIGN KEY ("endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_request_variations_deleted_at" to table: "api_request_variations"
CREATE INDEX "idx_api_request_variations_deleted_at" ON "api_request_variations" ("deleted_at");
-- Create index "idx_api_request_variations_endpoint_id" to table: "api_request_variations"
CREATE INDEX "idx_api_request_variations_endpoint_id" ON "api_request_variations" ("endpoint_id");
-- Create "api_scans" table
CREATE TABLE "api_scans" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "scan_id" bigint NOT NULL,
  "definition_id" uuid NOT NULL,
  "run_api_specific_tests" boolean NULL DEFAULT true,
  "run_standard_tests" boolean NULL DEFAULT true,
  "total_endpoints" bigint NULL DEFAULT 0,
  "completed_endpoints" bigint NULL DEFAULT 0,
  "started_at" timestamptz NULL,
  "completed_at" timestamptz NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_scans_definition" FOREIGN KEY ("definition_id") REFERENCES "api_definitions" ("id") ON UPDATE NO ACTION ON DELETE CASCADE,
  CONSTRAINT "fk_api_scans_scan" FOREIGN KEY ("scan_id") REFERENCES "scans" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_api_scans_definition_id" to table: "api_scans"
CREATE INDEX "idx_api_scans_definition_id" ON "api_scans" ("definition_id");
-- Create index "idx_api_scans_deleted_at" to table: "api_scans"
CREATE INDEX "idx_api_scans_deleted_at" ON "api_scans" ("deleted_at");
-- Create index "idx_api_scans_scan_id" to table: "api_scans"
CREATE INDEX "idx_api_scans_scan_id" ON "api_scans" ("scan_id");
-- Create "api_scan_endpoints" table
CREATE TABLE "api_scan_endpoints" (
  "api_scan_id" uuid NOT NULL,
  "api_endpoint_id" uuid NOT NULL,
  PRIMARY KEY ("api_scan_id", "api_endpoint_id"),
  CONSTRAINT "fk_api_scan_endpoints_api_endpoint" FOREIGN KEY ("api_endpoint_id") REFERENCES "api_endpoints" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION,
  CONSTRAINT "fk_api_scan_endpoints_api_scan" FOREIGN KEY ("api_scan_id") REFERENCES "api_scans" ("id") ON UPDATE NO ACTION ON DELETE NO ACTION
);
