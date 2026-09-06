-- +goose Up
-- +goose StatementBegin

-- pgcrypto provides gen_random_uuid().
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Non-owner application role. The API connects as this role so RLS is enforced.
CREATE ROLE pitlane_app WITH LOGIN PASSWORD 'pitlane_app' NOINHERIT;

-- The migration owner (superuser in dev) can switch into the app role for
-- tests that want to verify RLS from a privileged connection.
GRANT pitlane_app TO pitlane;

-- +goose StatementEnd

CREATE TABLE tenants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL,
    address text,
    vat_number text,
    logo_key text,
    currency text NOT NULL DEFAULT 'EUR',
    locale text NOT NULL DEFAULT 'en',
    default_tax_rate integer NOT NULL DEFAULT 1900,
    settings jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('owner', 'admin', 'mechanic')),
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- scs session store. Infrastructure table: no RLS, but it contains no tenant data.
CREATE TABLE sessions (
    token text PRIMARY KEY,
    data bytea NOT NULL,
    expiry timestamptz NOT NULL
);

-- Phase 10 will consume this; created now so the auth surface is complete.
CREATE TABLE password_reset_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash text NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid,
    payload jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Privileges for the app role.
GRANT USAGE ON SCHEMA public TO pitlane_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO pitlane_app;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO pitlane_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO pitlane_app;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO pitlane_app;

-- Force RLS even for the table owner (the migration role).
ALTER TABLE tenants FORCE ROW LEVEL SECURITY;
ALTER TABLE users FORCE ROW LEVEL SECURITY;
ALTER TABLE password_reset_tokens FORCE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE ROW LEVEL SECURITY;

-- RLS policies: tenant scoping via app.tenant_id.
CREATE POLICY tenants_isolation ON tenants
    USING (id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (id = current_setting('app.tenant_id')::uuid);

CREATE POLICY users_isolation ON users
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY password_reset_tokens_isolation ON password_reset_tokens
    USING (EXISTS (SELECT 1 FROM users WHERE users.id = password_reset_tokens.user_id AND users.tenant_id = current_setting('app.tenant_id')::uuid))
    WITH CHECK (EXISTS (SELECT 1 FROM users WHERE users.id = password_reset_tokens.user_id AND users.tenant_id = current_setting('app.tenant_id')::uuid));

CREATE POLICY audit_log_isolation ON audit_log
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Login needs to look up a user by email before the tenant context is known.
-- This SECURITY DEFINER function is the only controlled RLS bypass.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_user_by_email(p_email text)
RETURNS SETOF users
LANGUAGE sql
SECURITY DEFINER
SET row_security = off
AS $$
    SELECT * FROM users WHERE email = lower(p_email);
$$;
-- +goose StatementEnd

GRANT EXECUTE ON FUNCTION get_user_by_email(text) TO pitlane_app;

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_tenant_id ON users(tenant_id);
CREATE INDEX idx_sessions_expiry ON sessions(expiry);
CREATE INDEX idx_audit_log_tenant_created ON audit_log(tenant_id, created_at);

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS get_user_by_email(text);
DROP TABLE IF EXISTS audit_log;
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS tenants;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM pitlane_app;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM pitlane_app;
REVOKE USAGE ON SCHEMA public FROM pitlane_app;
REVOKE pitlane_app FROM pitlane;
DROP ROLE IF EXISTS pitlane_app;
-- +goose StatementEnd
