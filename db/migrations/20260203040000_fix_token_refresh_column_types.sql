-- Modify "token_refresh_configs" table: change request_headers from text to jsonb
ALTER TABLE "token_refresh_configs" ALTER COLUMN "request_headers" TYPE jsonb USING "request_headers"::jsonb;
-- Modify "token_refresh_configs" table: change interval_seconds from bigint to integer
ALTER TABLE "token_refresh_configs" ALTER COLUMN "interval_seconds" TYPE integer;
