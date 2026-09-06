-- +goose Up

-- UUIDs remain the canonical internal identifiers used by foreign keys and URLs.
-- Human-facing offer and repair numbers use independent, tenant-scoped annual
-- sequences (for example OF-2026-000001 and RP-2026-000001).
CREATE TABLE document_counters (
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    document_type text NOT NULL CHECK (document_type IN ('offer', 'repair')),
    year integer NOT NULL CHECK (year BETWEEN 2000 AND 9999),
    last_number bigint NOT NULL CHECK (last_number > 0),
    PRIMARY KEY (tenant_id, document_type, year)
);

ALTER TABLE offers ADD COLUMN document_number text;
ALTER TABLE repairs ADD COLUMN document_number text;

-- Preserve existing data by numbering documents in creation order within each
-- tenant and calendar year. UUID order makes ties deterministic.
WITH ranked AS (
    SELECT id,
           'OF-' || EXTRACT(YEAR FROM created_at)::integer || '-' ||
               lpad(row_number() OVER (
                   PARTITION BY tenant_id, EXTRACT(YEAR FROM created_at)
                   ORDER BY created_at, id
               )::text, 6, '0') AS document_number
    FROM offers
)
UPDATE offers o
SET document_number = ranked.document_number
FROM ranked
WHERE ranked.id = o.id;

WITH ranked AS (
    SELECT id,
           'RP-' || EXTRACT(YEAR FROM created_at)::integer || '-' ||
               lpad(row_number() OVER (
                   PARTITION BY tenant_id, EXTRACT(YEAR FROM created_at)
                   ORDER BY created_at, id
               )::text, 6, '0') AS document_number
    FROM repairs
)
UPDATE repairs r
SET document_number = ranked.document_number
FROM ranked
WHERE ranked.id = r.id;

-- Start each counter after the backfilled documents so new numbers cannot
-- collide with existing rows.
INSERT INTO document_counters (tenant_id, document_type, year, last_number)
SELECT tenant_id, 'offer', EXTRACT(YEAR FROM created_at)::integer, count(*)
FROM offers
GROUP BY tenant_id, EXTRACT(YEAR FROM created_at)
ON CONFLICT (tenant_id, document_type, year) DO UPDATE
SET last_number = GREATEST(document_counters.last_number, EXCLUDED.last_number);

INSERT INTO document_counters (tenant_id, document_type, year, last_number)
SELECT tenant_id, 'repair', EXTRACT(YEAR FROM created_at)::integer, count(*)
FROM repairs
GROUP BY tenant_id, EXTRACT(YEAR FROM created_at)
ON CONFLICT (tenant_id, document_type, year) DO UPDATE
SET last_number = GREATEST(document_counters.last_number, EXCLUDED.last_number);

ALTER TABLE offers
    ALTER COLUMN document_number SET NOT NULL,
    ADD CONSTRAINT offers_document_number_format
        CHECK (document_number ~ '^OF-[0-9]{4}-[0-9]{6,}$'),
    ADD CONSTRAINT offers_tenant_document_number_key
        UNIQUE (tenant_id, document_number);

ALTER TABLE repairs
    ALTER COLUMN document_number SET NOT NULL,
    ADD CONSTRAINT repairs_document_number_format
        CHECK (document_number ~ '^RP-[0-9]{4}-[0-9]{6,}$'),
    ADD CONSTRAINT repairs_tenant_document_number_key
        UNIQUE (tenant_id, document_number);




-- +goose Down
ALTER TABLE repairs DROP COLUMN IF EXISTS document_number;
ALTER TABLE offers DROP COLUMN IF EXISTS document_number;
DROP TABLE IF EXISTS document_counters;
