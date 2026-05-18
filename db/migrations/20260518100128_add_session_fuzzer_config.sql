-- Modify "playground_sessions" table
ALTER TABLE "playground_sessions" ADD COLUMN "fuzzer_config" jsonb NULL;
