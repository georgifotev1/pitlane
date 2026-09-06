-- +goose Up

-- Margin tracking. A garage buys a part from a distributor for one price and
-- sells it to the car owner for another; until now only the sell price was
-- recorded, so the app could report turnover but never profit.
--
-- cost_cents is the UNIT purchase price the garage pays, VAT-inclusive, exactly
-- like unit_price_cents on the same row. line_cost_cents is its line total
-- (cost x quantity), stored for the same reason line_total_cents is: reads and
-- reports never re-multiply. Labour and other lines usually carry 0 cost, which
-- is why the default is 0 and no backfill is needed.
--
-- This data is INTERNAL. It is never rendered on the customer-facing offer
-- print view or PDF.
ALTER TABLE offer_items
    ADD COLUMN cost_cents bigint NOT NULL DEFAULT 0 CHECK (cost_cents >= 0),
    ADD COLUMN line_cost_cents bigint NOT NULL DEFAULT 0 CHECK (line_cost_cents >= 0);

ALTER TABLE repair_items
    ADD COLUMN cost_cents bigint NOT NULL DEFAULT 0 CHECK (cost_cents >= 0),
    ADD COLUMN line_cost_cents bigint NOT NULL DEFAULT 0 CHECK (line_cost_cents >= 0);

-- Document-level cost snapshots, mirroring subtotal/tax/total: cost_total_cents
-- is VAT-inclusive (what actually leaves the bank account) and
-- cost_subtotal_cents is the net amount, so profit is a plain subtraction of two
-- net figures (subtotal_cents - cost_subtotal_cents) with no VAT arithmetic at
-- read time. Both are derived from the items and recomputed server-side on every
-- write, like every other money snapshot here.
ALTER TABLE offers
    ADD COLUMN cost_total_cents bigint NOT NULL DEFAULT 0 CHECK (cost_total_cents >= 0),
    ADD COLUMN cost_subtotal_cents bigint NOT NULL DEFAULT 0 CHECK (cost_subtotal_cents >= 0);

ALTER TABLE repairs
    ADD COLUMN cost_total_cents bigint NOT NULL DEFAULT 0 CHECK (cost_total_cents >= 0),
    ADD COLUMN cost_subtotal_cents bigint NOT NULL DEFAULT 0 CHECK (cost_subtotal_cents >= 0);

-- The dashboard reads revenue per calendar month from completed repairs, which
-- the existing (tenant_id, status, created_at) index cannot serve: completion
-- time is what counts as income, not creation time.
CREATE INDEX idx_repairs_tenant_completed ON repairs(tenant_id, completed_at DESC)
    WHERE status = 'completed';

-- +goose Down
DROP INDEX IF EXISTS idx_repairs_tenant_completed;
ALTER TABLE repairs     DROP COLUMN IF EXISTS cost_subtotal_cents, DROP COLUMN IF EXISTS cost_total_cents;
ALTER TABLE offers      DROP COLUMN IF EXISTS cost_subtotal_cents, DROP COLUMN IF EXISTS cost_total_cents;
ALTER TABLE repair_items DROP COLUMN IF EXISTS line_cost_cents, DROP COLUMN IF EXISTS cost_cents;
ALTER TABLE offer_items  DROP COLUMN IF EXISTS line_cost_cents, DROP COLUMN IF EXISTS cost_cents;
