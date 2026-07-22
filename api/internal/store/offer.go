package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrOfferNotDraft is returned when a write targets an offer that is no longer
// a draft. Offer content is immutable once sent (ADR §Domain Model). The
// handler maps it to a 409.
var ErrOfferNotDraft = errors.New("offer is not a draft")

// ErrInvalidStatusTransition is returned when a status change is not allowed by
// the lifecycle machine (domain.offerTransitions). The handler maps it to a 409.
var ErrInvalidStatusTransition = errors.New("invalid status transition")

// OfferStore follows the CarStore pattern: column list + scan helper co-located,
// every method runs inside WithTenant, every query filters on tenant_id. An
// offer owns its items, so writes replace the item set wholesale inside the
// same transaction and Recompute keeps the money snapshots in step.
type OfferStore struct {
	db *DB
}

// NewOfferStore builds a store.
func NewOfferStore(db *DB) *OfferStore {
	return &OfferStore{db: db}
}

// offerColumns is the canonical select order; scanOffer reads in this order.
const offerColumns = "id, tenant_id, car_id, status, send_status, sent_to, sent_at, tax_rate_bps, subtotal_cents, tax_cents, total_cents, notes, created_at, updated_at"

func scanOffer(row pgx.Row) (*domain.Offer, error) {
	var o domain.Offer
	if err := row.Scan(
		&o.ID, &o.TenantID, &o.CarID, &o.Status, &o.SendStatus, &o.SentTo, &o.SentAt,
		&o.TaxRateBps, &o.SubtotalCents, &o.TaxCents, &o.TotalCents, &o.Notes,
		&o.CreatedAt, &o.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &o, nil
}

// offerItemColumns is the canonical select order; scanOfferItem reads it.
const offerItemColumns = "id, tenant_id, offer_id, kind, description, quantity, unit_price_cents, line_total_cents, sort_order, created_at"

func scanOfferItem(row pgx.Row) (*domain.OfferItem, error) {
	var it domain.OfferItem
	if err := row.Scan(
		&it.ID, &it.TenantID, &it.OfferID, &it.Kind, &it.Description,
		&it.Quantity, &it.UnitPriceCents, &it.LineTotalCents, &it.SortOrder, &it.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &it, nil
}

// OfferListParams controls the List query. Zero Limit means "no rows"; the
// handler clamps to a sane page size.
type OfferListParams struct {
	Limit  int
	Offset int
}

// List returns a page of a car's offers (newest first) plus the total count,
// both scoped to tenant_id AND car_id. Items are not loaded for the list view.
func (s *OfferStore) List(ctx context.Context, tenantID, carID string, p OfferListParams) ([]*domain.Offer, int, error) {
	var offers []*domain.Offer
	var total int

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		const where = `WHERE tenant_id = $1 AND car_id = $2`

		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM offers `+where,
			tenantID, carID,
		).Scan(&total); err != nil {
			return fmt.Errorf("count offers: %w", err)
		}

		rows, err := tx.Query(ctx,
			`SELECT `+offerColumns+` FROM offers `+where+`
			 ORDER BY created_at DESC, id DESC
			 LIMIT $3 OFFSET $4`,
			tenantID, carID, p.Limit, p.Offset,
		)
		if err != nil {
			return fmt.Errorf("list offers: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			o, err := scanOffer(rows)
			if err != nil {
				return fmt.Errorf("scan offer: %w", err)
			}
			offers = append(offers, o)
		}
		return rows.Err()
	})
	return offers, total, err
}

// Get returns one offer with its items in editor order, scoped to the tenant,
// or ErrNotFound.
func (s *OfferStore) Get(ctx context.Context, tenantID, id string) (*domain.Offer, error) {
	var offer *domain.Offer
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		o, err := getOfferTx(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		offer = o
		return nil
	})
	return offer, err
}

// Create inserts a draft offer and its items in one transaction. Status and
// send_status are forced to their initial values, and totals are recomputed
// server-side from the items — client-supplied money is never trusted.
func (s *OfferStore) Create(ctx context.Context, o *domain.Offer) error {
	o.Status = domain.OfferStatusDraft
	o.SendStatus = domain.SendStatusPending
	o.Recompute()

	return s.db.WithTenant(ctx, o.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO offers (id, tenant_id, car_id, status, send_status, tax_rate_bps, subtotal_cents, tax_cents, total_cents, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING created_at, updated_at
		`, o.ID, o.TenantID, o.CarID, string(o.Status), string(o.SendStatus),
			o.TaxRateBps, o.SubtotalCents, o.TaxCents, o.TotalCents, o.Notes).
			Scan(&o.CreatedAt, &o.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert offer: %w", err)
		}
		return insertItems(ctx, tx, o)
	})
}

// Update rewrites a draft offer's editable fields and replaces its item set,
// recomputing totals. It is draft-only: a sent (or later) offer returns
// ErrOfferNotDraft, and a missing offer returns ErrNotFound. car_id is
// immutable (an offer does not move between cars).
func (s *OfferStore) Update(ctx context.Context, o *domain.Offer) error {
	o.Recompute()

	return s.db.WithTenant(ctx, o.TenantID, func(tx pgx.Tx) error {
		// Lock the row and gate on status before touching anything, so a
		// concurrent send can't slip a write past the immutability rule.
		var status domain.OfferStatus
		err := tx.QueryRow(ctx,
			`SELECT status FROM offers WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			o.ID, o.TenantID,
		).Scan(&status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != domain.OfferStatusDraft {
			return ErrOfferNotDraft
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM offer_items WHERE tenant_id = $1 AND offer_id = $2`,
			o.TenantID, o.ID,
		); err != nil {
			return fmt.Errorf("delete offer items: %w", err)
		}
		if err := insertItems(ctx, tx, o); err != nil {
			return err
		}

		return tx.QueryRow(ctx, `
			UPDATE offers
			SET tax_rate_bps = $3, subtotal_cents = $4, tax_cents = $5, total_cents = $6, notes = $7, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
			RETURNING updated_at
		`, o.ID, o.TenantID, o.TaxRateBps, o.SubtotalCents, o.TaxCents, o.TotalCents, o.Notes).
			Scan(&o.UpdatedAt)
	})
}

// SetStatus advances an offer's lifecycle status, enforcing the transition
// machine. Returns ErrNotFound if the offer is absent, or
// ErrInvalidStatusTransition if the move is not allowed from the current state.
// On success it returns the reloaded offer with items. Email delivery
// (send_status / sent_at) is out of scope here — that is the Phase 7 job.
func (s *OfferStore) SetStatus(ctx context.Context, tenantID, id string, next domain.OfferStatus) (*domain.Offer, error) {
	var offer *domain.Offer
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var current domain.OfferStatus
		err := tx.QueryRow(ctx,
			`SELECT status FROM offers WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			id, tenantID,
		).Scan(&current)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if !current.CanTransitionTo(next) {
			return ErrInvalidStatusTransition
		}

		if _, err := tx.Exec(ctx,
			`UPDATE offers SET status = $3, updated_at = now() WHERE id = $1 AND tenant_id = $2`,
			id, tenantID, string(next),
		); err != nil {
			return fmt.Errorf("update offer status: %w", err)
		}

		o, err := getOfferTx(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		offer = o
		return nil
	})
	return offer, err
}

// getOfferTx loads one offer plus its items within an existing tenant tx.
func getOfferTx(ctx context.Context, tx pgx.Tx, tenantID, id string) (*domain.Offer, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+offerColumns+` FROM offers WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	o, err := scanOffer(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	items, err := loadItems(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	o.Items = items
	return o, nil
}

// loadItems reads an offer's items in editor order within an existing tx.
func loadItems(ctx context.Context, tx pgx.Tx, tenantID, offerID string) ([]domain.OfferItem, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+offerItemColumns+` FROM offer_items WHERE tenant_id = $1 AND offer_id = $2 ORDER BY sort_order ASC, id ASC`,
		tenantID, offerID,
	)
	if err != nil {
		return nil, fmt.Errorf("query offer items: %w", err)
	}
	defer rows.Close()

	var items []domain.OfferItem
	for rows.Next() {
		it, err := scanOfferItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan offer item: %w", err)
		}
		items = append(items, *it)
	}
	return items, rows.Err()
}

// insertItems writes an offer's items, stamping tenant_id, offer_id, and
// sort_order from slice position. The store owns item identity because items
// are replaced wholesale on every draft write, so it assigns any missing IDs.
func insertItems(ctx context.Context, tx pgx.Tx, o *domain.Offer) error {
	for i := range o.Items {
		it := &o.Items[i]
		it.TenantID = o.TenantID
		it.OfferID = o.ID
		it.SortOrder = i
		if it.ID == "" {
			it.ID = uuid.NewString()
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO offer_items (id, tenant_id, offer_id, kind, description, quantity, unit_price_cents, line_total_cents, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING created_at
		`, it.ID, it.TenantID, it.OfferID, string(it.Kind), it.Description,
			it.Quantity, it.UnitPriceCents, it.LineTotalCents, it.SortOrder).
			Scan(&it.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert offer item: %w", err)
		}
	}
	return nil
}
