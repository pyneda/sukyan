-- Modify "scans" table
ALTER TABLE "scans" ADD COLUMN "pause_on_auth_failure" boolean NULL DEFAULT false, ADD COLUMN "pause_reason" text NULL;
-- Create "token_refresh_configs" table
CREATE TABLE "token_refresh_configs" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "auth_config_id" uuid NOT NULL,
  "request_url" text NOT NULL,
  "request_method" character varying(10) NOT NULL,
  "request_headers" text NULL,
  "request_body" text NULL,
  "request_content_type" character varying(100) NULL,
  "interval_seconds" bigint NOT NULL,
  "extraction_source" character varying(50) NOT NULL,
  "extraction_value" text NOT NULL,
  "current_token" text NULL,
  "token_fetched_at" timestamptz NULL,
  "last_error" text NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_api_auth_configs_token_refresh_config" FOREIGN KEY ("auth_config_id") REFERENCES "api_auth_configs" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);
-- Create index "idx_token_refresh_configs_auth_config_id" to table: "token_refresh_configs"
CREATE UNIQUE INDEX "idx_token_refresh_configs_auth_config_id" ON "token_refresh_configs" ("auth_config_id");
-- Create index "idx_token_refresh_configs_deleted_at" to table: "token_refresh_configs"
CREATE INDEX "idx_token_refresh_configs_deleted_at" ON "token_refresh_configs" ("deleted_at");
