-- Fix auth_config foreign key to SET NULL on delete
ALTER TABLE "api_definitions" DROP CONSTRAINT IF EXISTS "fk_api_definitions_auth_config";
ALTER TABLE "api_definitions" ADD CONSTRAINT "fk_api_definitions_auth_config"
    FOREIGN KEY ("auth_config_id") REFERENCES "api_auth_configs" ("id")
    ON UPDATE CASCADE ON DELETE SET NULL;
