-- +goose Up

-- Remediation: turn the RLS layer ON.
--
-- Migrations 0001–0004 each declared `ALTER TABLE ... FORCE ROW LEVEL SECURITY`
-- and a per-table isolation POLICY, but never issued the companion
-- `ENABLE ROW LEVEL SECURITY`. In Postgres those are independent flags: FORCE
-- only governs whether RLS also applies to the table owner, and it does nothing
-- unless RLS is actually ENABLED. The result was that every policy sat dormant
-- and tenant isolation rested solely on the app-level `WHERE tenant_id = $1`
-- filters — the ADR's RLS layer (five-layer tenancy) was effectively absent.
--
-- The surrounding design already assumes RLS is live: `WithTenant` sets the
-- pitlane_app role + `app.tenant_id` per transaction, the tenants policy is
-- self-referential on `id`, and `get_user_by_email` is a SECURITY DEFINER
-- bypass built specifically for the login lookup that runs before a tenant is
-- known. So this migration simply supplies the one statement that was missing,
-- for every tenant-owned table that already carries a policy + FORCE.
--
-- Forward-only and safe on any already-migrated database: it flips a flag,
-- touches no rows, and creates nothing.

ALTER TABLE tenants               ENABLE ROW LEVEL SECURITY;
ALTER TABLE users                 ENABLE ROW LEVEL SECURITY;
ALTER TABLE password_reset_tokens ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log             ENABLE ROW LEVEL SECURITY;
ALTER TABLE customers             ENABLE ROW LEVEL SECURITY;
ALTER TABLE cars                  ENABLE ROW LEVEL SECURITY;
ALTER TABLE offers                ENABLE ROW LEVEL SECURITY;
ALTER TABLE offer_items           ENABLE ROW LEVEL SECURITY;

-- +goose Down
ALTER TABLE offer_items           DISABLE ROW LEVEL SECURITY;
ALTER TABLE offers                DISABLE ROW LEVEL SECURITY;
ALTER TABLE cars                  DISABLE ROW LEVEL SECURITY;
ALTER TABLE customers             DISABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log             DISABLE ROW LEVEL SECURITY;
ALTER TABLE password_reset_tokens DISABLE ROW LEVEL SECURITY;
ALTER TABLE users                 DISABLE ROW LEVEL SECURITY;
ALTER TABLE tenants               DISABLE ROW LEVEL SECURITY;
