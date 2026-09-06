-- +goose Up

-- Cars belong to a customer (ADR §Domain Model). A car's tenant must always
-- equal its customer's tenant. RLS keeps each tenant's rows invisible to the
-- others, but RLS alone does not stop a tenant from *referencing* another
-- tenant's customer_id on insert (referential-integrity checks bypass RLS).
-- The airtight fix is a COMPOSITE foreign key on (customer_id, tenant_id):
-- Postgres then requires a real customers row with BOTH columns matching, so a
-- car can only ever point at a customer in its own tenant. That FK needs a
-- unique key on exactly (id, tenant_id) to target.
ALTER TABLE customers ADD CONSTRAINT customers_id_tenant_key UNIQUE (id, tenant_id);

CREATE TABLE cars (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    customer_id uuid NOT NULL,
    -- plate is normalized to upper-case in the handler so uniqueness is
    -- case-insensitive in practice (AB1234 == ab1234).
    plate text NOT NULL,
    -- Optional fields follow the customer pattern: NOT NULL DEFAULT so Go scans
    -- plain strings/ints, never sql.Null*. year/mileage use 0 as "unknown".
    vin text NOT NULL DEFAULT '',
    make text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    year integer NOT NULL DEFAULT 0,
    mileage integer NOT NULL DEFAULT 0,
    archived_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    -- Composite FK: guarantees a car and its customer share a tenant at the DB
    -- level, complementing the RLS policy below. ON DELETE RESTRICT per the
    -- deletion policy (soft-delete only; FKs never CASCADE).
    FOREIGN KEY (customer_id, tenant_id) REFERENCES customers (id, tenant_id) ON DELETE RESTRICT
);

-- The app role already receives table privileges via ALTER DEFAULT PRIVILEGES in
-- 0001, but we grant explicitly so this migration is self-contained and the
-- pattern is obvious when copied.
GRANT SELECT, INSERT, UPDATE, DELETE ON cars TO pitlane_app;

-- Five-layer tenancy: FORCE RLS so even the table owner is constrained.
ALTER TABLE cars FORCE ROW LEVEL SECURITY;

CREATE POLICY cars_isolation ON cars
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- The list query is always scoped to one customer and orders by plate.
CREATE INDEX idx_cars_tenant_customer_plate ON cars(tenant_id, customer_id, plate);

-- Plate is unique per tenant, but only among ACTIVE cars: archiving a car frees
-- its plate for re-registration. A partial unique index expresses exactly that.
CREATE UNIQUE INDEX idx_cars_tenant_plate_active ON cars(tenant_id, plate) WHERE archived_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS cars;
ALTER TABLE customers DROP CONSTRAINT IF EXISTS customers_id_tenant_key;
