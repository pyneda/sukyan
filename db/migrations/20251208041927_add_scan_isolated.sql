-- Modify "scans" table
ALTER TABLE "scans" ADD COLUMN "isolated" boolean NULL DEFAULT false;
-- Create index "idx_scans_isolated" to table: "scans"
CREATE INDEX "idx_scans_isolated" ON "scans" ("isolated");
-- Drop enum type "severity"
DROP TYPE "severity";
