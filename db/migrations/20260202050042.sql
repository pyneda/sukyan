-- Modify "site_behavior_results" table
ALTER TABLE "site_behavior_results" ADD COLUMN "base_url_sample_id" bigint NULL, ADD CONSTRAINT "fk_site_behavior_results_base_url_sample" FOREIGN KEY ("base_url_sample_id") REFERENCES "histories" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Create index "idx_site_behavior_results_base_url_sample_id" to table: "site_behavior_results"
CREATE INDEX "idx_site_behavior_results_base_url_sample_id" ON "site_behavior_results" ("base_url_sample_id");
-- Create "site_behavior_not_found_samples" table
CREATE TABLE "site_behavior_not_found_samples" (
  "id" bigserial NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "site_behavior_result_id" uuid NOT NULL,
  "history_id" bigint NOT NULL,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_site_behavior_not_found_samples_history" FOREIGN KEY ("history_id") REFERENCES "histories" ("id") ON UPDATE CASCADE ON DELETE CASCADE,
  CONSTRAINT "fk_site_behavior_results_not_found_samples" FOREIGN KEY ("site_behavior_result_id") REFERENCES "site_behavior_results" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_site_behavior_not_found_samples_deleted_at" to table: "site_behavior_not_found_samples"
CREATE INDEX "idx_site_behavior_not_found_samples_deleted_at" ON "site_behavior_not_found_samples" ("deleted_at");
-- Create index "idx_site_behavior_not_found_samples_history_id" to table: "site_behavior_not_found_samples"
CREATE INDEX "idx_site_behavior_not_found_samples_history_id" ON "site_behavior_not_found_samples" ("history_id");
-- Create index "idx_site_behavior_not_found_samples_site_behavior_result_id" to table: "site_behavior_not_found_samples"
CREATE INDEX "idx_site_behavior_not_found_samples_site_behavior_result_id" ON "site_behavior_not_found_samples" ("site_behavior_result_id");
