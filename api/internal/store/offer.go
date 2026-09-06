package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrOfferNotDraft = errors.New("offer is not a draft")

var ErrInvalidStatusTransition = errors.New("invalid status transition")

var ErrOfferNotSendable = errors.New("offer is not in a sendable state")

type OfferEmailEnqueuer interface {
	EnqueueOfferEmail(ctx context.Context, tx pgx.Tx, tenantID, offerID string) error
}

type OfferStore struct {
	db *DB
}

func NewOfferStore(db *DB) *OfferStore {
	return &OfferStore{db: db}
}

const offerColumns = "id, document_number, tenant_id, car_id, status, send_status, sent_to, sent_at, tax_rate_bps, subtotal_cents, tax_cents, total_cents, cost_total_cents, cost_subtotal_cents, notes, created_at, updated_at"

func scanOffer(row pgx.Row) (*domain.Offer, error) {
	var o domain.Offer
	if err := row.Scan(
		&o.ID, &o.DocumentNumber, &o.TenantID, &o.CarID, &o.Status, &o.SendStatus, &o.SentTo, &o.SentAt,
		&o.TaxRateBps, &o.SubtotalCents, &o.TaxCents, &o.TotalCents,
		&o.CostTotalCents, &o.CostSubtotalCents, &o.Notes,
		&o.CreatedAt, &o.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &o, nil
}

const offerItemColumns = "id, tenant_id, offer_id, kind, description, quantity, unit_price_cents, line_total_cents, cost_cents, line_cost_cents, sort_order, created_at"

func scanOfferItem(row pgx.Row) (*domain.OfferItem, error) {
	var it domain.OfferItem
	if err := row.Scan(
		&it.ID, &it.TenantID, &it.OfferID, &it.Kind, &it.Description,
		&it.Quantity, &it.UnitPriceCents, &it.LineTotalCents,
		&it.CostCents, &it.LineCostCents, &it.SortOrder, &it.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &it, nil
}

type OfferListParams struct {
	Limit  int
	Offset int
}

type OfferSummary struct {
	Offer        *domain.Offer
	CarPlate     string
	CustomerName string
}

type OfferBoardParams struct {
	Status string
	Limit  int
	Offset int
}

// offerBoardSelect is the one query shape every offer list uses - the board, the
// dashboard's recent offers and its stale-offer panel - so the columns and the
// tenant-tight joins are written once. Callers append their own WHERE/ORDER.
const offerBoardSelect = `SELECT o.id, o.document_number, o.tenant_id, o.car_id, o.status, o.send_status, o.sent_to, o.sent_at,
	        o.tax_rate_bps, o.subtotal_cents, o.tax_cents, o.total_cents,
	        o.cost_total_cents, o.cost_subtotal_cents, o.notes,
	        o.created_at, o.updated_at,
	        c.plate, cust.name
	 FROM offers o
	 JOIN cars c ON c.id = o.car_id AND c.tenant_id = o.tenant_id
	 JOIN customers cust ON cust.id = c.customer_id AND cust.tenant_id = c.tenant_id
	 `

func scanOfferSummaries(rows pgx.Rows) ([]OfferSummary, error) {
	var out []OfferSummary
	for rows.Next() {
		var o domain.Offer
		var plate, customerName string
		if err := rows.Scan(
			&o.ID, &o.DocumentNumber, &o.TenantID, &o.CarID, &o.Status, &o.SendStatus, &o.SentTo, &o.SentAt,
			&o.TaxRateBps, &o.SubtotalCents, &o.TaxCents, &o.TotalCents,
			&o.CostTotalCents, &o.CostSubtotalCents, &o.Notes,
			&o.CreatedAt, &o.UpdatedAt,
			&plate, &customerName,
		); err != nil {
			return nil, fmt.Errorf("scan offer summary: %w", err)
		}
		out = append(out, OfferSummary{Offer: &o, CarPlate: plate, CustomerName: customerName})
	}
	return out, rows.Err()
}

func (s *OfferStore) ListAll(ctx context.Context, tenantID string, p OfferBoardParams) ([]OfferSummary, int, error) {
	var out []OfferSummary
	var total int

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		const where = `WHERE o.tenant_id = $1 AND ($2 = '' OR o.status = $2)`

		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM offers o `+where,
			tenantID, p.Status,
		).Scan(&total); err != nil {
			return fmt.Errorf("count offers: %w", err)
		}

		rows, err := tx.Query(ctx,
			offerBoardSelect+where+`
			 ORDER BY o.created_at DESC, o.id DESC
			 LIMIT $3 OFFSET $4`,
			tenantID, p.Status, p.Limit, p.Offset,
		)
		if err != nil {
			return fmt.Errorf("list offers board: %w", err)
		}
		defer rows.Close()

		out, err = scanOfferSummaries(rows)
		return err
	})
	return out, total, err
}

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

func (s *OfferStore) Create(ctx context.Context, o *domain.Offer) error {
	o.Status = domain.OfferStatusDraft
	o.SendStatus = domain.SendStatusPending
	o.Recompute()

	return s.db.WithTenant(ctx, o.TenantID, func(tx pgx.Tx) error {
		documentNumber, err := nextDocumentNumber(ctx, tx, o.TenantID, documentTypeOffer)
		if err != nil {
			return err
		}
		o.DocumentNumber = documentNumber

		err = tx.QueryRow(ctx, `
			INSERT INTO offers (id, document_number, tenant_id, car_id, status, send_status, tax_rate_bps, subtotal_cents, tax_cents, total_cents, cost_total_cents, cost_subtotal_cents, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING created_at, updated_at
		`, o.ID, o.DocumentNumber, o.TenantID, o.CarID, string(o.Status), string(o.SendStatus),
			o.TaxRateBps, o.SubtotalCents, o.TaxCents, o.TotalCents,
			o.CostTotalCents, o.CostSubtotalCents, o.Notes).
			Scan(&o.CreatedAt, &o.UpdatedAt)
		if err != nil {
			return fmt.Errorf("insert offer: %w", err)
		}
		return insertItems(ctx, tx, o)
	})
}

func (s *OfferStore) Update(ctx context.Context, o *domain.Offer) error {
	o.Recompute()

	return s.db.WithTenant(ctx, o.TenantID, func(tx pgx.Tx) error {
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
			SET tax_rate_bps = $3, subtotal_cents = $4, tax_cents = $5, total_cents = $6,
			    cost_total_cents = $7, cost_subtotal_cents = $8, notes = $9, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
			RETURNING updated_at
		`, o.ID, o.TenantID, o.TaxRateBps, o.SubtotalCents, o.TaxCents, o.TotalCents,
			o.CostTotalCents, o.CostSubtotalCents, o.Notes).
			Scan(&o.UpdatedAt)
	})
}

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

func (s *OfferStore) MarkSending(ctx context.Context, tenantID, id, sentTo string, enq OfferEmailEnqueuer) (*domain.Offer, error) {
	var offer *domain.Offer
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var status domain.OfferStatus
		var sendStatus domain.SendStatus
		err := tx.QueryRow(ctx,
			`SELECT status, send_status FROM offers WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			id, tenantID,
		).Scan(&status, &sendStatus)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		initialSend := status == domain.OfferStatusDraft
		retry := status == domain.OfferStatusSent && sendStatus == domain.SendStatusFailed
		if !initialSend && !retry {
			return ErrOfferNotSendable
		}

		if _, err := tx.Exec(ctx, `
			UPDATE offers
			SET status = $3, send_status = $4, sent_to = $5, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID, string(domain.OfferStatusSent), string(domain.SendStatusPending), sentTo); err != nil {
			return fmt.Errorf("mark offer sending: %w", err)
		}

		if err := enq.EnqueueOfferEmail(ctx, tx, tenantID, id); err != nil {
			return fmt.Errorf("enqueue offer email: %w", err)
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

func (s *OfferStore) MarkSent(ctx context.Context, tenantID, id string) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			UPDATE offers SET send_status = $3, sent_at = now(), updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID, string(domain.SendStatusSent))
		if err != nil {
			return fmt.Errorf("mark offer sent: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *OfferStore) MarkSendFailed(ctx context.Context, tenantID, id string) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			UPDATE offers SET send_status = $3, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID, string(domain.SendStatusFailed))
		if err != nil {
			return fmt.Errorf("mark offer send failed: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

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
			INSERT INTO offer_items (id, tenant_id, offer_id, kind, description, quantity, unit_price_cents, line_total_cents, cost_cents, line_cost_cents, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING created_at
		`, it.ID, it.TenantID, it.OfferID, string(it.Kind), it.Description,
			it.Quantity, it.UnitPriceCents, it.LineTotalCents,
			it.CostCents, it.LineCostCents, it.SortOrder).
			Scan(&it.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert offer item: %w", err)
		}
	}
	return nil
}
