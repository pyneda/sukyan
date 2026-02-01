-- Create unique partial index on api_definitions(workspace_id, source_url)
-- Only applies when source_url is not null/empty to allow definitions without source URLs
CREATE UNIQUE INDEX IF NOT EXISTS "idx_api_definitions_workspace_source_url"
ON "api_definitions" ("workspace_id", "source_url")
WHERE "source_url" IS NOT NULL AND "source_url" != '';
