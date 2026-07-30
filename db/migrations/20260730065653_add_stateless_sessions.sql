-- Modify "users" table
ALTER TABLE "users" ADD COLUMN "tokens_valid_from" timestamptz NOT NULL DEFAULT '1970-01-01 00:00:00+00';
-- Drop "refresh_tokens" table
DROP TABLE "refresh_tokens";
