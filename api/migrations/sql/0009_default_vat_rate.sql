-- +goose Up

-- Bulgaria's standard VAT rate is fixed at 20% in the application. Keep the
-- legacy tenant column pinned to that rate; owners can no longer configure it.
ALTER TABLE tenants ALTER COLUMN default_tax_rate SET DEFAULT 2000;
UPDATE tenants
SET default_tax_rate = 2000,
    updated_at = now()
WHERE default_tax_rate <> 2000;

-- +goose Down

ALTER TABLE tenants ALTER COLUMN default_tax_rate SET DEFAULT 1900;
-- Existing tenant values cannot be restored safely.
