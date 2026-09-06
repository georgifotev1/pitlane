-- +goose Up

-- A repair is the WORK a garage actually performs on a car. It is a separate
-- entity from the offer (ADR §Domain Model): an offer is a quote, a repair is
-- the job. A repair is anchored to one car and MAY carry provenance back to the
-- offer it was converted from (offer_id, nullable). Its line items are COPIED
-- from the offer at conversion, never shared — editing a repair item must never
-- mutate the offer it came from (the price-freeze rule).
--
-- Two composite FKs keep the graph tenant-tight the same way offers/cars do,
-- blocking a cross-tenant reference at the DB level rather than leaving it to
-- the application's WHERE clauses: (car_id, tenant_id) → cars, and
-- (offer_id, tenant_id) → offers when an offer is present.
CREATE TABLE repairs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    car_id uuid NOT NULL,

    -- Provenance: the offer this repair was converted from, or NULL for a
    -- repair that never had a quote (none created that way in Phase 8, but the
    -- column models the domain and keeps the door open without a later ALTER).
    offer_id uuid,

    -- Work lifecycle (ADR §Domain Model): open → in_progress → completed.
    -- open is the only editable state (items/notes/tax); the others freeze
    -- content, mirroring the offer's draft-only rule. The CHECK is the DB-level
    -- backstop; the store enforces the transition machine.
    status text NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'in_progress', 'completed')),

    -- Tax rate SNAPSHOTTED at conversion from the offer (which itself snapshotted
    -- the tenant default). Editable while open. Kept here for the same reason as
    -- on offers: a later change to any default never silently re-prices the job.
    tax_rate_bps integer NOT NULL CHECK (tax_rate_bps >= 0),

    -- Money snapshots in integer cents, recomputed server-side from the items on
    -- every open write and frozen on completion. Derived data, stored so reads
    -- never re-sum items.
    subtotal_cents bigint NOT NULL DEFAULT 0 CHECK (subtotal_cents >= 0),
    tax_cents bigint NOT NULL DEFAULT 0 CHECK (tax_cents >= 0),
    total_cents bigint NOT NULL DEFAULT 0 CHECK (total_cents >= 0),

    -- Odometer reading for the repair; it can be entered while editing or at
    -- completion and is also synchronized onto the car. Preserved on the repair
    -- so the service-history timeline can show mileage at each job.
    mileage integer NOT NULL DEFAULT 0 CHECK (mileage >= 0),

    notes text NOT NULL DEFAULT '',

    -- Stamped when the repair is completed; NULL while open/in_progress. Drives
    -- the "completed repairs" half of the derived service history (Phase 9).
    completed_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Composite FKs keep a repair and its car/offer in the same tenant at the DB
    -- level. RESTRICT per the deletion policy (repairs are never deleted; FKs
    -- never CASCADE). The offer FK is on the pair so a NULL offer_id simply
    -- satisfies it (Postgres skips the check when any FK column is NULL).
    FOREIGN KEY (car_id, tenant_id) REFERENCES cars (id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (offer_id, tenant_id) REFERENCES offers (id, tenant_id) ON DELETE RESTRICT,

    -- Target for repair_items' composite FK, so items stay in the repair's tenant.
    UNIQUE (id, tenant_id)
);

CREATE TABLE repair_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    repair_id uuid NOT NULL,

    -- Same classification vocabulary as offer_items: the copy is faithful.
    kind text NOT NULL DEFAULT 'part'
        CHECK (kind IN ('part', 'labor', 'other')),
    description text NOT NULL,
    quantity integer NOT NULL CHECK (quantity > 0),
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),
    line_total_cents bigint NOT NULL DEFAULT 0 CHECK (line_total_cents >= 0),
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),

    -- Composite FK keeps each item in its repair's tenant. RESTRICT: a repair
    -- cannot be dropped while items exist. Open edits replace item rows directly.
    FOREIGN KEY (repair_id, tenant_id) REFERENCES repairs (id, tenant_id) ON DELETE RESTRICT
);

-- The board lists a tenant's repairs newest first, usually filtered by status.
CREATE INDEX idx_repairs_tenant_status_created ON repairs(tenant_id, status, created_at DESC);

-- Service history (Phase 9) reads completed repairs for one car.
CREATE INDEX idx_repairs_tenant_car_created ON repairs(tenant_id, car_id, created_at DESC);

-- Provenance is 1:1: an offer converts to at most one repair. A partial unique
-- index is the DB backstop for the conversion gate (accept requires status=sent
-- and flips it to accepted, so a second conversion is already impossible — this
-- guarantees it even if that gate were ever bypassed).
CREATE UNIQUE INDEX idx_repairs_tenant_offer ON repairs(tenant_id, offer_id) WHERE offer_id IS NOT NULL;

-- Items are always fetched for one repair in editor order.
CREATE INDEX idx_repair_items_tenant_repair_sort ON repair_items(tenant_id, repair_id, sort_order);

-- +goose Down
DROP TABLE IF EXISTS repair_items;
DROP TABLE IF EXISTS repairs;
