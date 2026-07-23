-- +goose Up

-- Phase 10: password reset + staff invitations.

-- password_changed_at powers "destroy all sessions on reset" (ADR §Security).
-- requireAuth reloads the user on every request; a session whose createdAt
-- predates this timestamp is destroyed there. NULL = password never changed.
ALTER TABLE users ADD COLUMN password_changed_at timestamptz;

-- Invitations: an owner/admin invites an email with a role; the invitee
-- completes their account from the emailed link. The token is SHA-256 hashed
-- at rest, single-use (accepted_at), and expiring. Only staff roles are
-- invitable — the owner role is born only at signup.
CREATE TABLE invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    email text NOT NULL,
    role text NOT NULL CHECK (role IN ('admin', 'mechanic')),
    token_hash text NOT NULL,
    invited_by uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- One pending invitation per email per tenant (revoked rows are deleted, so
-- they never block a re-invite; accepted rows are history).
CREATE UNIQUE INDEX invitations_one_pending_per_email
    ON invitations (tenant_id, email) WHERE accepted_at IS NULL;

-- Per-email reset throttle counts recent tokens (ADR decision 18, Postgres half).
CREATE INDEX idx_password_reset_tokens_user_created
    ON password_reset_tokens (user_id, created_at);

GRANT SELECT, INSERT, UPDATE, DELETE ON invitations TO pitlane_app;

-- Five-layer tenancy.
ALTER TABLE invitations FORCE ROW LEVEL SECURITY;
ALTER TABLE invitations ENABLE ROW LEVEL SECURITY;

CREATE POLICY invitations_isolation ON invitations
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- Pre-tenant lookups for the public reset/accept endpoints, which run without
-- a session and therefore without a tenant context. Same controlled-bypass
-- pattern as get_user_by_email (0001): SECURITY DEFINER + row_security=off,
-- keyed by the token hash, never by a client-supplied tenant.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_password_reset_token(p_token_hash text)
RETURNS TABLE (id uuid, user_id uuid, tenant_id uuid, expires_at timestamptz, used_at timestamptz)
LANGUAGE sql
SECURITY DEFINER
SET row_security = off
AS $$
    SELECT t.id, t.user_id, u.tenant_id, t.expires_at, t.used_at
    FROM password_reset_tokens t
    JOIN users u ON u.id = t.user_id
    WHERE t.token_hash = p_token_hash;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION get_invitation_by_token(p_token_hash text)
RETURNS SETOF invitations
LANGUAGE sql
SECURITY DEFINER
SET row_security = off
AS $$
    SELECT * FROM invitations WHERE token_hash = p_token_hash;
$$;
-- +goose StatementEnd

GRANT EXECUTE ON FUNCTION get_password_reset_token(text) TO pitlane_app;
GRANT EXECUTE ON FUNCTION get_invitation_by_token(text) TO pitlane_app;

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS get_invitation_by_token(text);
DROP FUNCTION IF EXISTS get_password_reset_token(text);
DROP TABLE IF EXISTS invitations;
ALTER TABLE users DROP COLUMN IF EXISTS password_changed_at;
-- +goose StatementEnd
