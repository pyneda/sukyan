-- Create "matcher_presets" table
CREATE TABLE "matcher_presets" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "workspace_id" bigint NOT NULL,
  "domain" varchar(32) NOT NULL,
  "name" varchar(128) NOT NULL,
  "matcher_set" jsonb NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_matcher_presets_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_matcher_presets_deleted_at" to table: "matcher_presets"
CREATE INDEX "idx_matcher_presets_deleted_at" ON "matcher_presets" ("deleted_at");
-- Unique composite index — names are unique per workspace + domain so the
-- same label can live independently in HTTP-fuzz and WS-fuzz pickers.
CREATE UNIQUE INDEX "idx_matcher_preset_ws_domain_name" ON "matcher_presets" ("workspace_id", "domain", "name");
