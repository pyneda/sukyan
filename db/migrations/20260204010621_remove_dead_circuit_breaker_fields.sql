-- Modify "scans" table
ALTER TABLE "scans" DROP COLUMN "consecutive_failures", DROP COLUMN "last_failure_at", DROP COLUMN "throttled_until";
