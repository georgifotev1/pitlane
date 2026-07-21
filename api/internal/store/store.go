// Package store is the raw database/sql access layer. Every tenant-owned query
// includes tenant_id and runs inside WithTenant, which sets the app role and
// app.tenant_id for Postgres RLS (ADR §Multi-Tenancy).
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a query returns zero rows where one was expected.
var ErrNotFound = errors.New("not found")

// DB wraps the pgx pool and provides the RLS-aware transaction helper.
type DB struct {
	pool *pgxpool.Pool
}

// NewDB wraps a pool.
func NewDB(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// WithTenant runs fn inside a transaction that has switched into the
// pitlane_app role and set app.tenant_id. This enforces RLS regardless of the
// connection user (the app role in production, the superuser in tests).
func (db *DB) WithTenant(ctx context.Context, tenantID string, fn func(pgx.Tx) error) error {
	if tenantID == "" {
		return errors.New("tenantID is required")
	}

	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, "SET LOCAL ROLE pitlane_app"); err != nil {
		return fmt.Errorf("set role: %w", err)
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID); err != nil {
		return fmt.Errorf("set tenant id: %w", err)
	}

	if err := fn(tx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// TenantStore handles the root tenant entity.
type TenantStore struct {
	db *DB
}

// NewTenantStore builds a store.
func NewTenantStore(db *DB) *TenantStore {
	return &TenantStore{db: db}
}

// Create inserts a tenant under its own tenant_id.
func (s *TenantStore) Create(ctx context.Context, tenant *domain.Tenant) error {
	return s.db.WithTenant(ctx, tenant.ID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO tenants (id, name, address, vat_number, logo_key, currency, locale, default_tax_rate, settings)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, tenant.ID, tenant.Name, tenant.Address, tenant.VATNumber, tenant.LogoKey,
			tenant.Currency, tenant.Locale, tenant.DefaultTaxRate, tenant.Settings)
		return err
	})
}

// GetByID returns a tenant by ID, scoped to the tenant itself.
func (s *TenantStore) GetByID(ctx context.Context, tenantID string) (*domain.Tenant, error) {
	var tenant *domain.Tenant
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, name, address, vat_number, logo_key, currency, locale, default_tax_rate, settings, created_at, updated_at
			FROM tenants
			WHERE id = $1
		`, tenantID)
		var t domain.Tenant
		if err := row.Scan(&t.ID, &t.Name, &t.Address, &t.VATNumber, &t.LogoKey,
			&t.Currency, &t.Locale, &t.DefaultTaxRate, &t.Settings, &t.CreatedAt, &t.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		tenant = &t
		return nil
	})
	return tenant, err
}

// UserStore handles users. Email is globally unique, but all other access is
// tenant-scoped.
type UserStore struct {
	db *DB
}

// NewUserStore builds a store.
func NewUserStore(db *DB) *UserStore {
	return &UserStore{db: db}
}

// Create inserts a user under the tenant.
func (s *UserStore) Create(ctx context.Context, user *domain.User) error {
	return s.db.WithTenant(ctx, user.TenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO users (id, tenant_id, email, password_hash, role, name)
			VALUES ($1, $2, lower($3), $4, $5, $6)
		`, user.ID, user.TenantID, user.Email, user.PasswordHash, string(user.Role), user.Name)
		return err
	})
}

// GetByEmail looks up a user by email globally. It uses the SECURITY DEFINER
// function get_user_by_email, which is the only controlled RLS bypass in the
// schema because the login endpoint does not yet know the tenant context.
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT id, tenant_id, email, password_hash, role, name, created_at, updated_at
		FROM get_user_by_email($1)
	`, email)
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, ErrNotFound
	}
	var u domain.User
	if err := rows.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &u.Name, &u.CreatedAt, &u.UpdatedAt); err != nil {
		return nil, err
	}
	if rows.Next() {
		return nil, errors.New("multiple users found for email")
	}
	return &u, nil
}

// GetByID returns a user scoped to the tenant.
func (s *UserStore) GetByID(ctx context.Context, tenantID, userID string) (*domain.User, error) {
	var user *domain.User
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, email, password_hash, role, name, created_at, updated_at
			FROM users
			WHERE id = $1 AND tenant_id = $2
		`, userID, tenantID)
		var u domain.User
		if err := row.Scan(&u.ID, &u.TenantID, &u.Email, &u.PasswordHash, &u.Role, &u.Name, &u.CreatedAt, &u.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		user = &u
		return nil
	})
	return user, err
}

// AuditLogStore writes the audit trail.
type AuditLogStore struct {
	db *DB
}

// NewAuditLogStore builds a store.
func NewAuditLogStore(db *DB) *AuditLogStore {
	return &AuditLogStore{db: db}
}

// Insert records a mutating action.
func (s *AuditLogStore) Insert(ctx context.Context, tenantID, userID, action, entityType, entityID string, payload map[string]any) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var eid any
		if entityID != "" {
			eid = entityID
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO audit_log (tenant_id, user_id, action, entity_type, entity_id, payload)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, tenantID, userID, action, entityType, eid, payload)
		return err
	})
}
