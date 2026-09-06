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


-- +goose Down
DROP TABLE IF EXISTS invitations;
ALTER TABLE users DROP COLUMN IF EXISTS password_changed_at;
