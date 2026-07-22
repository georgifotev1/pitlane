-- +goose Up

-- Service history is DERIVED: completed repairs for a car plus optional manual
-- notes for external work. history_notes is the only new table here; the
-- completed repairs half is already in the repairs table.
--
-- A history note belongs to exactly one car, within one tenant. Composite FK on
-- (car_id, tenant_id) keeps the graph tenant-tight (FK checks bypass RLS, so a
-- plain car_id FK would not block another tenant's car).
CREATE TABLE history_notes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    car_id uuid NOT NULL,

    -- The manual entry: a title and a longer description of external work done
    -- elsewhere (e.g., at a dealer or another garage). RecordedAt is the date the
    -- work happened, defaulting to now() so quick notes just work.
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    recorded_at timestamptz NOT NULL DEFAULT now(),

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    FOREIGN KEY (car_id, tenant_id) REFERENCES cars (id, tenant_id) ON DELETE RESTRICT,
    UNIQUE (id, tenant_id)
);

-- Attachments are photos/documents tied to either a car or a repair. Exactly one
-- of car_id/repair_id must be set (the CHECK below). The object bytes live in
-- R2 (MinIO locally), keyed under the tenant; this table stores metadata.
CREATE TABLE attachments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,

    -- Exactly one of car_id or repair_id must be set. The composite FKs keep the
    -- attachment in the same tenant as the car/repair it belongs to.
    car_id uuid,
    repair_id uuid,
    CHECK (num_nonnulls(car_id, repair_id) = 1),

    -- Display name and the S3/R2 key (tenant-prefixed in the app layer).
    name text NOT NULL,
    storage_key text NOT NULL,

    -- Size in bytes and the sniffed content type (client-supplied type is
    -- ignored; we trust http.DetectContentType on the first bytes).
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    content_type text NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    FOREIGN KEY (car_id, tenant_id) REFERENCES cars (id, tenant_id) ON DELETE RESTRICT,
    FOREIGN KEY (repair_id, tenant_id) REFERENCES repairs (id, tenant_id) ON DELETE RESTRICT,
    UNIQUE (id, tenant_id)
);

GRANT SELECT, INSERT, UPDATE, DELETE ON history_notes TO pitlane_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON attachments TO pitlane_app;

-- Five-layer tenancy.
ALTER TABLE history_notes FORCE ROW LEVEL SECURITY;
ALTER TABLE history_notes ENABLE ROW LEVEL SECURITY;
ALTER TABLE attachments FORCE ROW LEVEL SECURITY;
ALTER TABLE attachments ENABLE ROW LEVEL SECURITY;

CREATE POLICY history_notes_isolation ON history_notes
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

CREATE POLICY attachments_isolation ON attachments
    USING (tenant_id = current_setting('app.tenant_id')::uuid)
    WITH CHECK (tenant_id = current_setting('app.tenant_id')::uuid);

-- History reads completed repairs and notes for one car, ordered by date.
CREATE INDEX idx_history_notes_tenant_car_recorded ON history_notes(tenant_id, car_id, recorded_at DESC);

-- Attachments are listed per car or per repair.
CREATE INDEX idx_attachments_tenant_car ON attachments(tenant_id, car_id) WHERE car_id IS NOT NULL;
CREATE INDEX idx_attachments_tenant_repair ON attachments(tenant_id, repair_id) WHERE repair_id IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS attachments;
DROP TABLE IF EXISTS history_notes;
