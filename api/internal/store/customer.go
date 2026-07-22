package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/jackc/pgx/v5"
)

// CustomerStore is the pattern-setting entity store every later entity copies:
// column list + scan helper co-located (ADR scan discipline), every method runs
// inside WithTenant, every query filters on tenant_id even on primary-key
// lookups.
type CustomerStore struct {
	db *DB
}

// NewCustomerStore builds a store.
func NewCustomerStore(db *DB) *CustomerStore {
	return &CustomerStore{db: db}
}

// customerColumns is the canonical select order. scanCustomer below reads in
// exactly this order — keep them in sync.
const customerColumns = "id, tenant_id, name, company, email, phone, address, notes, archived_at, created_at, updated_at"

// scanCustomer reads one row in customerColumns order.
func scanCustomer(row pgx.Row) (*domain.Customer, error) {
	var c domain.Customer
	if err := row.Scan(
		&c.ID, &c.TenantID, &c.Name, &c.Company, &c.Email, &c.Phone,
		&c.Address, &c.Notes, &c.ArchivedAt, &c.CreatedAt, &c.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &c, nil
}

// CustomerListParams controls the List query. Zero Limit means "no rows"; the
// handler is responsible for clamping to a sane page size.
type CustomerListParams struct {
	Search          string
	IncludeArchived bool
	Limit           int
	Offset          int
}

// List returns a page of customers plus the total count matching the filter
// (before pagination), so the handler can build pagination metadata. Both the
// page query and the count run in the same tenant transaction.
func (s *CustomerStore) List(ctx context.Context, tenantID string, p CustomerListParams) ([]*domain.Customer, int, error) {
	var customers []*domain.Customer
	var total int

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		// $2 = search term (empty means "match all"); $3 = include archived.
		// The ILIKE wildcards are concatenated in SQL so the term stays a bound
		// parameter — no fmt.Sprintf near SQL (house law).
		const where = `
			WHERE tenant_id = $1
			  AND ($2 = '' OR name ILIKE '%' || $2 || '%'
			                OR company ILIKE '%' || $2 || '%'
			                OR email ILIKE '%' || $2 || '%'
			                OR phone ILIKE '%' || $2 || '%')
			  AND ($3 OR archived_at IS NULL)`

		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM customers`+where,
			tenantID, p.Search, p.IncludeArchived,
		).Scan(&total); err != nil {
			return fmt.Errorf("count customers: %w", err)
		}

		rows, err := tx.Query(ctx,
			`SELECT `+customerColumns+` FROM customers`+where+`
			 ORDER BY name ASC, id ASC
			 LIMIT $4 OFFSET $5`,
			tenantID, p.Search, p.IncludeArchived, p.Limit, p.Offset,
		)
		if err != nil {
			return fmt.Errorf("list customers: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			c, err := scanCustomer(rows)
			if err != nil {
				return fmt.Errorf("scan customer: %w", err)
			}
			customers = append(customers, c)
		}
		return rows.Err()
	})
	return customers, total, err
}

// Get returns one customer scoped to the tenant, or ErrNotFound.
func (s *CustomerStore) Get(ctx context.Context, tenantID, id string) (*domain.Customer, error) {
	var customer *domain.Customer
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+customerColumns+` FROM customers WHERE id = $1 AND tenant_id = $2`,
			id, tenantID,
		)
		c, err := scanCustomer(row)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		customer = c
		return nil
	})
	return customer, err
}

// Create inserts a customer and populates server-assigned timestamps via
// RETURNING. The caller supplies the ID (uuid generated in the handler, matching
// the user-creation pattern).
func (s *CustomerStore) Create(ctx context.Context, c *domain.Customer) error {
	return s.db.WithTenant(ctx, c.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO customers (id, tenant_id, name, company, email, phone, address, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			RETURNING created_at, updated_at
		`, c.ID, c.TenantID, c.Name, c.Company, c.Email, c.Phone, c.Address, c.Notes).
			Scan(&c.CreatedAt, &c.UpdatedAt)
	})
}

// Update writes all mutable fields (PUT semantics) and bumps updated_at.
// Returns ErrNotFound if the customer does not exist in this tenant.
func (s *CustomerStore) Update(ctx context.Context, c *domain.Customer) error {
	return s.db.WithTenant(ctx, c.TenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			UPDATE customers
			SET name = $3, company = $4, email = $5, phone = $6, address = $7, notes = $8, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
			RETURNING updated_at
		`, c.ID, c.TenantID, c.Name, c.Company, c.Email, c.Phone, c.Address, c.Notes)
		if err := row.Scan(&c.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		return nil
	})
}

// Archive soft-deletes a customer by stamping archived_at. Archiving an already
// archived (or nonexistent) customer returns ErrNotFound, so the handler can map
// it to a clean 404.
func (s *CustomerStore) Archive(ctx context.Context, tenantID, id string) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE customers
			SET archived_at = now(), updated_at = now()
			WHERE id = $1 AND tenant_id = $2 AND archived_at IS NULL
		`, id, tenantID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}
