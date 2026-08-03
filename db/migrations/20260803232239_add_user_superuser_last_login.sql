-- Modify "users" table
ALTER TABLE "users" ADD COLUMN "superuser" boolean NOT NULL DEFAULT false, ADD COLUMN "last_login_at" timestamptz NULL;
