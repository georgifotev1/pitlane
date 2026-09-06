-- +goose Up

-- A deployment can carry a demonstration garage alongside real ones so the
-- product can be shown to a prospect with a year of plausible history behind
-- it. The flag exists so that the only destructive command in the binary -
-- `pitlane demo reset`, which empties the tenant before rebuilding it - can
-- refuse to touch anything that is not demonstration data. RLS already keeps
-- the delete inside one tenant; this keeps it inside a tenant that was created
-- to be thrown away.
ALTER TABLE tenants ADD COLUMN is_demo boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tenants DROP COLUMN IF EXISTS is_demo;
