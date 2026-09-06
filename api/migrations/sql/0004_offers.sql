-- +goose Up

-- An offer (repair quote) is written for one car. As with cars→customers in
-- 0003, RLS keeps tenants' rows mutually invisible but does NOT stop a tenant
-- from *referencing* another tenant's car_id on insert (FK checks bypass RLS).
-- The airtight fix is again a COMPOSITE foreign key on (car_id, tenant_id): an
-- offer can then only ever point at a car in its own tenant. That FK needs a
-- unique key on exactly (id, tenant_id) on cars to target. The customer is
-- reached through the car, so offers carry no customer_id of their own — one
-- link, no drift.
ALTER TABLE cars ADD CONSTRAINT cars_id_tenant_key UNIQUE (id, tenant_id);

CREATE TABLE offers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    car_id uuid NOT NULL,

    -- Lifecycle status machine (ADR §Domain Model):
    -- draft → sent → accepted | rejected | expired. Enforced in the store; the
    -- CHECK is the DB-level backstop. Content is mutable only while 'draft'.
    status text NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'sent', 'accepted', 'rejected', 'expired')),

    -- Email-send lifecycle, surfaced in the UI for River-job visibility/retry
    -- (Phase 7). Meaningless until the offer is sent; 'pending' is the resting
    -- value. sent_to / sent_at record the actual delivery target and moment.
    send_status text NOT NULL DEFAULT 'pending'
        CHECK (send_status IN ('pending', 'sent', 'failed')),
    sent_to text NOT NULL DEFAULT '',
    sent_at timestamptz,

    -- Tax rate is snapshotted in basis points. The application sets Bulgaria's
    -- fixed standard rate; retaining the snapshot keeps historical documents
    -- self-contained.
    tax_rate_bps integer NOT NULL CHECK (tax_rate_bps >= 0),

    -- Money snapshots in integer cents. Item prices and total_cents include
    -- VAT; subtotal_cents is the net taxable amount and tax_cents is extracted
    -- once at document level. The server recomputes them on every draft write.
    -- They are derived data, kept here so reads and the PDF never re-sum items.
    subtotal_cents bigint NOT NULL DEFAULT 0 CHECK (subtotal_cents >= 0),
    tax_cents bigint NOT NULL DEFAULT 0 CHECK (tax_cents >= 0),
    total_cents bigint NOT NULL DEFAULT 0 CHECK (total_cents >= 0),

    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Composite FK: an offer and its car always share a tenant at the DB level,
    -- complementing the RLS policy below. RESTRICT per the deletion policy
    -- (offers are never deleted; FKs never CASCADE).
    FOREIGN KEY (car_id, tenant_id) REFERENCES cars (id, tenant_id) ON DELETE RESTRICT,

    -- Target for offer_items' composite FK, so items stay in the offer's tenant.
    UNIQUE (id, tenant_id)
);

CREATE TABLE offer_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    offer_id uuid NOT NULL,

    -- Item classification for the PDF grouping and later reporting.
    kind text NOT NULL DEFAULT 'part'
        CHECK (kind IN ('part', 'labor', 'other')),
    description text NOT NULL,

    -- Whole-unit quantity (parts count / labour hours as whole hours). Unit
    -- prices include VAT and stay integer cents, so line math is exact; VAT is
    -- extracted and rounded once at the offer level.
    quantity integer NOT NULL CHECK (quantity > 0),
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),

    -- Line total snapshot = unit_price_cents * quantity, computed server-side.
    line_total_cents bigint NOT NULL DEFAULT 0 CHECK (line_total_cents >= 0),

    -- Preserves the row order from the useFieldArray editor.
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),

    -- Composite FK keeps each item in its offer's tenant. RESTRICT: an offer
    -- cannot be dropped while items exist. Draft edits replace items by deleting
    -- the item rows directly (never the offer), so this never blocks normal use.
    FOREIGN KEY (offer_id, tenant_id) REFERENCES offers (id, tenant_id) ON DELETE RESTRICT
);

-- The app role receives privileges on new public tables via ALTER DEFAULT
-- PRIVILEGES in 0001, but we grant explicitly so each migration is
-- self-contained and the pattern is obvious when copied.
GRANT SELECT, INSERT, UPDATE, DELETE ON offers TO pitlane_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON offer_items TO pitlane_app;

-- Five-layer tenancy: FORCE RLS so even the table owner is constrained.
ALTER TABLE offers FORCE ROW LEVEL SECURITY;
ALTER TABLE offer_items FORCE ROW LEVEL SECURITY;

CREATE POLICY offers_isolation ON offers
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY offer_items_isolation ON offer_items
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- The offers list is scoped to one car and shows newest first.
CREATE INDEX idx_offers_tenant_car_created ON offers(tenant_id, car_id, created_at DESC);

-- Items are always fetched for one offer in editor order.
CREATE INDEX idx_offer_items_tenant_offer_sort ON offer_items(tenant_id, offer_id, sort_order);

-- +goose Down
DROP TABLE IF EXISTS offer_items;
DROP TABLE IF EXISTS offers;
ALTER TABLE cars DROP CONSTRAINT IF EXISTS cars_id_tenant_key;
