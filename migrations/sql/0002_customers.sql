-- +goose Up

CREATE TABLE customers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    name text NOT NULL,
    -- Optional contact fields use NOT NULL DEFAULT '' rather than nullable
    -- columns: this matches the tenant/user pattern (Go plain strings, no
    -- sql.NullString), so scans never hit a NULL. archived_at is the one
    -- genuinely-absent value, so it stays nullable and maps to *time.Time.
    company text NOT NULL DEFAULT '',
    email text NOT NULL DEFAULT '',
    phone text NOT NULL DEFAULT '',
    address text NOT NULL DEFAULT '',
    notes text NOT NULL DEFAULT '',
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- List query orders by name within a tenant; the search predicate also filters
-- by tenant_id first.
CREATE INDEX idx_customers_tenant_name ON customers(tenant_id, name);

-- +goose Down
DROP TABLE IF EXISTS customers;
