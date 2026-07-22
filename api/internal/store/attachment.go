package store

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AttachmentStore manages attachment metadata in Postgres. The actual bytes are
// stored in S3/R2/MinIO by the filestore package; this store keeps the metadata
// and ensures tenant scoping.
type AttachmentStore struct {
	db *DB
}

// NewAttachmentStore builds a store.
func NewAttachmentStore(db *DB) *AttachmentStore {
	return &AttachmentStore{db: db}
}

// AttachmentStorageKey returns the tenant-prefixed object key for an attachment.
// The key is deterministic given the tenant, attachment ID, and filename, so the
// same attachment always maps to the same object.
func AttachmentStorageKey(tenantID, attachmentID, filename string) string {
	ext := path.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	return fmt.Sprintf("tenants/%s/attachments/%s%s", tenantID, attachmentID, ext)
}

// ListByCar returns the attachments linked to a car, newest first.
func (s *AttachmentStore) ListByCar(ctx context.Context, tenantID, carID string) ([]domain.Attachment, error) {
	var out []domain.Attachment
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant_id, car_id, repair_id, name, storage_key, size_bytes, content_type, created_at
			FROM attachments
			WHERE tenant_id = $1 AND car_id = $2
			ORDER BY created_at DESC, id DESC
		`, tenantID, carID)
		if err != nil {
			return fmt.Errorf("query car attachments: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var a domain.Attachment
			if err := rows.Scan(&a.ID, &a.TenantID, &a.CarID, &a.RepairID, &a.Name, &a.StorageKey, &a.SizeBytes, &a.ContentType, &a.CreatedAt); err != nil {
				return fmt.Errorf("scan attachment: %w", err)
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// ListByRepair returns the attachments linked to a repair, newest first.
func (s *AttachmentStore) ListByRepair(ctx context.Context, tenantID, repairID string) ([]domain.Attachment, error) {
	var out []domain.Attachment
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, tenant_id, car_id, repair_id, name, storage_key, size_bytes, content_type, created_at
			FROM attachments
			WHERE tenant_id = $1 AND repair_id = $2
			ORDER BY created_at DESC, id DESC
		`, tenantID, repairID)
		if err != nil {
			return fmt.Errorf("query repair attachments: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var a domain.Attachment
			if err := rows.Scan(&a.ID, &a.TenantID, &a.CarID, &a.RepairID, &a.Name, &a.StorageKey, &a.SizeBytes, &a.ContentType, &a.CreatedAt); err != nil {
				return fmt.Errorf("scan attachment: %w", err)
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// Create inserts attachment metadata. The caller must already have stored the
// bytes in object storage under a.StorageKey.
func (s *AttachmentStore) Create(ctx context.Context, a *domain.Attachment) error {
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	return s.db.WithTenant(ctx, a.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO attachments (id, tenant_id, car_id, repair_id, name, storage_key, size_bytes, content_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING created_at
		`, a.ID, a.TenantID, a.CarID, a.RepairID, a.Name, a.StorageKey, a.SizeBytes, a.ContentType).
			Scan(&a.CreatedAt)
	})
}

// Get returns one attachment or ErrNotFound.
func (s *AttachmentStore) Get(ctx context.Context, tenantID, id string) (*domain.Attachment, error) {
	var a *domain.Attachment
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, car_id, repair_id, name, storage_key, size_bytes, content_type, created_at
			FROM attachments
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID)
		var att domain.Attachment
		if err := row.Scan(&att.ID, &att.TenantID, &att.CarID, &att.RepairID, &att.Name, &att.StorageKey, &att.SizeBytes, &att.ContentType, &att.CreatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		a = &att
		return nil
	})
	return a, err
}

// Delete removes attachment metadata. The caller is responsible for deleting the
// object from object storage afterwards.
func (s *AttachmentStore) Delete(ctx context.Context, tenantID, id string) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		res, err := tx.Exec(ctx, `
			DELETE FROM attachments
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID)
		if err != nil {
			return err
		}
		if res.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
