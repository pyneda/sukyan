-- Create "proxy_services" table
CREATE TABLE "proxy_services" (
  "id" uuid NOT NULL,
  "created_at" timestamptz NULL,
  "updated_at" timestamptz NULL,
  "deleted_at" timestamptz NULL,
  "workspace_id" bigint NOT NULL,
  "name" text NOT NULL,
  "host" text NULL DEFAULT 'localhost',
  "port" bigint NOT NULL,
  "verbose" boolean NULL DEFAULT true,
  "log_out_of_scope_requests" boolean NULL DEFAULT true,
  "enabled" boolean NULL DEFAULT false,
  PRIMARY KEY ("id"),
  CONSTRAINT "fk_proxy_services_workspace" FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON UPDATE CASCADE ON DELETE CASCADE
);
-- Create index "idx_proxy_services_deleted_at" to table: "proxy_services"
CREATE INDEX "idx_proxy_services_deleted_at" ON "proxy_services" ("deleted_at");
-- Create index "idx_proxy_services_enabled" to table: "proxy_services"
CREATE INDEX "idx_proxy_services_enabled" ON "proxy_services" ("enabled");
-- Create index "idx_proxy_services_port" to table: "proxy_services"
CREATE UNIQUE INDEX "idx_proxy_services_port" ON "proxy_services" ("port");
-- Create index "idx_proxy_services_workspace_id" to table: "proxy_services"
CREATE INDEX "idx_proxy_services_workspace_id" ON "proxy_services" ("workspace_id");
-- Modify "histories" table
ALTER TABLE "histories" ADD COLUMN "proxy_service_id" uuid NULL, ADD CONSTRAINT "fk_histories_proxy_service" FOREIGN KEY ("proxy_service_id") REFERENCES "proxy_services" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Create index "idx_histories_proxy_service_id" to table: "histories"
CREATE INDEX "idx_histories_proxy_service_id" ON "histories" ("proxy_service_id");
-- Modify "web_socket_connections" table
ALTER TABLE "web_socket_connections" ADD COLUMN "proxy_service_id" uuid NULL, ADD CONSTRAINT "fk_web_socket_connections_proxy_service" FOREIGN KEY ("proxy_service_id") REFERENCES "proxy_services" ("id") ON UPDATE CASCADE ON DELETE SET NULL;
-- Create index "idx_web_socket_connections_proxy_service_id" to table: "web_socket_connections"
CREATE INDEX "idx_web_socket_connections_proxy_service_id" ON "web_socket_connections" ("proxy_service_id");
