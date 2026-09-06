package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrRepairNotOpen = errors.New("repair is not open")

var ErrInvalidRepairStatusTransition = errors.New("invalid repair status transition")

var ErrOfferNotAcceptable = errors.New("offer is not in an acceptable state")

type RepairStore struct {
	db *DB
}

func NewRepairStore(db *DB) *RepairStore {
	return &RepairStore{db: db}
}

const repairColumns = "id, document_number, tenant_id, car_id, offer_id, status, tax_rate_bps, subtotal_cents, tax_cents, total_cents, cost_total_cents, cost_subtotal_cents, mileage, notes, completed_at, created_at, updated_at"

func scanRepair(row pgx.Row) (*domain.Repair, error) {
	var r domain.Repair
	if err := row.Scan(
		&r.ID, &r.DocumentNumber, &r.TenantID, &r.CarID, &r.OfferID, &r.Status, &r.TaxRateBps,
		&r.SubtotalCents, &r.TaxCents, &r.TotalCents,
		&r.CostTotalCents, &r.CostSubtotalCents, &r.Mileage, &r.Notes,
		&r.CompletedAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

const repairItemColumns = "id, tenant_id, repair_id, kind, description, quantity, unit_price_cents, line_total_cents, cost_cents, line_cost_cents, sort_order, created_at"

func scanRepairItem(row pgx.Row) (*domain.RepairItem, error) {
	var it domain.RepairItem
	if err := row.Scan(
		&it.ID, &it.TenantID, &it.RepairID, &it.Kind, &it.Description,
		&it.Quantity, &it.UnitPriceCents, &it.LineTotalCents,
		&it.CostCents, &it.LineCostCents, &it.SortOrder, &it.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &it, nil
}

type RepairSummary struct {
	Repair       *domain.Repair
	CarPlate     string
	CustomerName string
}

type RepairListParams struct {
	Status         string
	CustomerSearch string
	Limit          int
	Offset         int
}

type RepairStats struct {
	Total     int
	Active    int
	Completed int
	// RevenueCents is what customers were billed, VAT included. ProfitCents is
	// what was left after the parts, VAT excluded on both sides.
	RevenueCents int64
	ProfitCents  int64
	HasCost      bool
}

// repairBoardSelect is the shared shape for every repair list (board, dashboard
// panels, stalled-work panel); callers append their own WHERE/ORDER.
const repairBoardSelect = `SELECT r.id, r.document_number, r.tenant_id, r.car_id, r.offer_id, r.status, r.tax_rate_bps,
	        r.subtotal_cents, r.tax_cents, r.total_cents,
	        r.cost_total_cents, r.cost_subtotal_cents, r.mileage, r.notes,
	        r.completed_at, r.created_at, r.updated_at,
	        c.plate, cust.name
	 FROM repairs r
	 JOIN cars c ON c.id = r.car_id AND c.tenant_id = r.tenant_id
	 JOIN customers cust ON cust.id = c.customer_id AND cust.tenant_id = c.tenant_id
	 `

func scanRepairSummaries(rows pgx.Rows) ([]RepairSummary, error) {
	var out []RepairSummary
	for rows.Next() {
		var r domain.Repair
		var plate, customerName string
		if err := rows.Scan(
			&r.ID, &r.DocumentNumber, &r.TenantID, &r.CarID, &r.OfferID, &r.Status, &r.TaxRateBps,
			&r.SubtotalCents, &r.TaxCents, &r.TotalCents,
			&r.CostTotalCents, &r.CostSubtotalCents, &r.Mileage, &r.Notes,
			&r.CompletedAt, &r.CreatedAt, &r.UpdatedAt,
			&plate, &customerName,
		); err != nil {
			return nil, fmt.Errorf("scan repair summary: %w", err)
		}
		out = append(out, RepairSummary{Repair: &r, CarPlate: plate, CustomerName: customerName})
	}
	return out, rows.Err()
}

func (s *RepairStore) List(ctx context.Context, tenantID string, p RepairListParams) ([]RepairSummary, int, error) {
	var out []RepairSummary
	var total int

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		const where = `WHERE r.tenant_id = $1
			AND ($2 = '' OR r.status = $2)
			AND ($3 = '' OR cust.name ILIKE '%' || $3 || '%'
			                  OR cust.company ILIKE '%' || $3 || '%')`

		if err := tx.QueryRow(ctx,
			`SELECT count(*)
			 FROM repairs r
			 JOIN cars c ON c.id = r.car_id AND c.tenant_id = r.tenant_id
			 JOIN customers cust ON cust.id = c.customer_id AND cust.tenant_id = c.tenant_id
			 `+where,
			tenantID, p.Status, p.CustomerSearch,
		).Scan(&total); err != nil {
			return fmt.Errorf("count repairs: %w", err)
		}

		rows, err := tx.Query(ctx,
			repairBoardSelect+where+`
			 ORDER BY r.created_at DESC, r.id DESC
			 LIMIT $4 OFFSET $5`,
			tenantID, p.Status, p.CustomerSearch, p.Limit, p.Offset,
		)
		if err != nil {
			return fmt.Errorf("list repairs: %w", err)
		}
		defer rows.Close()

		out, err = scanRepairSummaries(rows)
		return err
	})
	return out, total, err
}

func (s *RepairStore) Stats(ctx context.Context, tenantID string) (RepairStats, error) {
	var stats RepairStats
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*),
			       count(*) FILTER (WHERE status IN ('open', 'in_progress')),
			       count(*) FILTER (WHERE status = 'completed'),
			       COALESCE(sum(total_cents) FILTER (WHERE status = 'completed'), 0),
			       COALESCE(sum(subtotal_cents - cost_subtotal_cents) FILTER (WHERE status = 'completed'), 0),
			       COALESCE(sum(cost_total_cents) FILTER (WHERE status = 'completed'), 0) > 0
			FROM repairs
			WHERE tenant_id = $1
		`, tenantID).Scan(&stats.Total, &stats.Active, &stats.Completed, &stats.RevenueCents, &stats.ProfitCents, &stats.HasCost)
	})
	return stats, err
}

func (s *RepairStore) Get(ctx context.Context, tenantID, id string) (*domain.Repair, error) {
	var repair *domain.Repair
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		r, err := getRepairTx(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		repair = r
		return nil
	})
	return repair, err
}

func (s *RepairStore) GetByOffer(ctx context.Context, tenantID, offerID string) (*domain.Repair, error) {
	var repair *domain.Repair
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, `
			SELECT id FROM repairs WHERE tenant_id = $1 AND offer_id = $2
		`, tenantID, offerID).Scan(&id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		r, err := getRepairTx(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		repair = r
		return nil
	})
	return repair, err
}

func (s *RepairStore) CreateFromOffer(ctx context.Context, tenantID, offerID string) (*domain.Repair, error) {
	var repair *domain.Repair
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var status domain.OfferStatus
		err := tx.QueryRow(ctx,
			`SELECT status FROM offers WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			offerID, tenantID,
		).Scan(&status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != domain.OfferStatusDraft && status != domain.OfferStatusSent {
			return ErrOfferNotAcceptable
		}

		offer, err := getOfferTx(ctx, tx, tenantID, offerID)
		if err != nil {
			return err
		}

		r := &domain.Repair{
			ID:         uuid.NewString(),
			TenantID:   tenantID,
			CarID:      offer.CarID,
			OfferID:    &offerID,
			Status:     domain.RepairStatusOpen,
			TaxRateBps: offer.TaxRateBps,
			Notes:      offer.Notes,
			Items:      make([]domain.RepairItem, 0, len(offer.Items)),
		}
		for _, it := range offer.Items {
			r.Items = append(r.Items, domain.RepairItem{
				Kind:           it.Kind,
				Description:    it.Description,
				Quantity:       it.Quantity,
				UnitPriceCents: it.UnitPriceCents,
				CostCents:      it.CostCents,
			})
		}
		r.Recompute()

		documentNumber, err := nextDocumentNumber(ctx, tx, tenantID, documentTypeRepair)
		if err != nil {
			return err
		}
		r.DocumentNumber = documentNumber

		if err := tx.QueryRow(ctx, `
			INSERT INTO repairs (id, document_number, tenant_id, car_id, offer_id, status, tax_rate_bps, subtotal_cents, tax_cents, total_cents, cost_total_cents, cost_subtotal_cents, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING created_at, updated_at
		`, r.ID, r.DocumentNumber, r.TenantID, r.CarID, r.OfferID, string(r.Status),
			r.TaxRateBps, r.SubtotalCents, r.TaxCents, r.TotalCents,
			r.CostTotalCents, r.CostSubtotalCents, r.Notes).
			Scan(&r.CreatedAt, &r.UpdatedAt); err != nil {
			return fmt.Errorf("insert repair: %w", err)
		}
		if err := insertRepairItems(ctx, tx, r); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx,
			`UPDATE offers SET status = $3, updated_at = now() WHERE id = $1 AND tenant_id = $2`,
			offerID, tenantID, string(domain.OfferStatusAccepted),
		); err != nil {
			return fmt.Errorf("accept offer: %w", err)
		}

		repair = r
		return nil
	})
	return repair, err
}

func (s *RepairStore) Update(ctx context.Context, r *domain.Repair) error {
	r.Recompute()

	return s.db.WithTenant(ctx, r.TenantID, func(tx pgx.Tx) error {
		var status domain.RepairStatus
		var carID string
		var oldMileage int
		err := tx.QueryRow(ctx,
			`SELECT status, car_id, mileage FROM repairs WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			r.ID, r.TenantID,
		).Scan(&status, &carID, &oldMileage)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status != domain.RepairStatusOpen {
			return ErrRepairNotOpen
		}

		if _, err := tx.Exec(ctx,
			`DELETE FROM repair_items WHERE tenant_id = $1 AND repair_id = $2`,
			r.TenantID, r.ID,
		); err != nil {
			return fmt.Errorf("delete repair items: %w", err)
		}
		if err := insertRepairItems(ctx, tx, r); err != nil {
			return err
		}

		if err := tx.QueryRow(ctx, `
			UPDATE repairs
			SET tax_rate_bps = $3, subtotal_cents = $4, tax_cents = $5, total_cents = $6,
			    cost_total_cents = $7, cost_subtotal_cents = $8,
			    mileage = $9, notes = $10, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
			RETURNING updated_at
		`, r.ID, r.TenantID, r.TaxRateBps, r.SubtotalCents, r.TaxCents, r.TotalCents,
			r.CostTotalCents, r.CostSubtotalCents, r.Mileage, r.Notes).
			Scan(&r.UpdatedAt); err != nil {
			return err
		}

		if _, err := tx.Exec(ctx, `
			UPDATE cars
			SET mileage = CASE WHEN mileage = $3 THEN $4 ELSE GREATEST(mileage, $4) END,
			    updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, carID, r.TenantID, oldMileage, r.Mileage); err != nil {
			return fmt.Errorf("update car mileage: %w", err)
		}
		return nil
	})
}

func (s *RepairStore) UpdateMileage(ctx context.Context, tenantID, id string, mileage int) (*domain.Repair, error) {
	var repair *domain.Repair
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var carID string
		var oldMileage int
		if err := tx.QueryRow(ctx,
			`SELECT car_id, mileage FROM repairs WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			id, tenantID,
		).Scan(&carID, &oldMileage); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}

		if _, err := tx.Exec(ctx, `
			UPDATE repairs SET mileage = $3, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID, mileage); err != nil {
			return fmt.Errorf("update repair mileage: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cars
			SET mileage = CASE WHEN mileage = $3 THEN $4 ELSE GREATEST(mileage, $4) END,
			    updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, carID, tenantID, oldMileage, mileage); err != nil {
			return fmt.Errorf("update car mileage: %w", err)
		}

		r, err := getRepairTx(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		repair = r
		return nil
	})
	return repair, err
}

func (s *RepairStore) SetStatus(ctx context.Context, tenantID, id string, next domain.RepairStatus) (*domain.Repair, error) {
	var repair *domain.Repair
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var current domain.RepairStatus
		err := tx.QueryRow(ctx,
			`SELECT status FROM repairs WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			id, tenantID,
		).Scan(&current)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if !current.CanTransitionTo(next) {
			return ErrInvalidRepairStatusTransition
		}

		if _, err := tx.Exec(ctx,
			`UPDATE repairs SET status = $3, updated_at = now() WHERE id = $1 AND tenant_id = $2`,
			id, tenantID, string(next),
		); err != nil {
			return fmt.Errorf("update repair status: %w", err)
		}

		r, err := getRepairTx(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		repair = r
		return nil
	})
	return repair, err
}

func (s *RepairStore) Complete(ctx context.Context, tenantID, id string, mileage int) (*domain.Repair, error) {
	var repair *domain.Repair
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		var status domain.RepairStatus
		var carID string
		err := tx.QueryRow(ctx,
			`SELECT status, car_id FROM repairs WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			id, tenantID,
		).Scan(&status, &carID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		if status == domain.RepairStatusCompleted {
			return ErrRepairNotOpen
		}

		if _, err := tx.Exec(ctx, `
			UPDATE repairs
			SET status = $3, mileage = $4, completed_at = now(), updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID, string(domain.RepairStatusCompleted), mileage); err != nil {
			return fmt.Errorf("complete repair: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			UPDATE cars
			SET mileage = GREATEST(mileage, $3), updated_at = now()
			WHERE id = $1 AND tenant_id = $2
		`, carID, tenantID, mileage); err != nil {
			return fmt.Errorf("update car mileage: %w", err)
		}

		r, err := getRepairTx(ctx, tx, tenantID, id)
		if err != nil {
			return err
		}
		repair = r
		return nil
	})
	return repair, err
}

func getRepairTx(ctx context.Context, tx pgx.Tx, tenantID, id string) (*domain.Repair, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+repairColumns+` FROM repairs WHERE id = $1 AND tenant_id = $2`,
		id, tenantID,
	)
	r, err := scanRepair(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	items, err := loadRepairItems(ctx, tx, tenantID, id)
	if err != nil {
		return nil, err
	}
	r.Items = items
	return r, nil
}

func loadRepairItems(ctx context.Context, tx pgx.Tx, tenantID, repairID string) ([]domain.RepairItem, error) {
	rows, err := tx.Query(ctx,
		`SELECT `+repairItemColumns+` FROM repair_items WHERE tenant_id = $1 AND repair_id = $2 ORDER BY sort_order ASC, id ASC`,
		tenantID, repairID,
	)
	if err != nil {
		return nil, fmt.Errorf("query repair items: %w", err)
	}
	defer rows.Close()

	var items []domain.RepairItem
	for rows.Next() {
		it, err := scanRepairItem(rows)
		if err != nil {
			return nil, fmt.Errorf("scan repair item: %w", err)
		}
		items = append(items, *it)
	}
	return items, rows.Err()
}

func insertRepairItems(ctx context.Context, tx pgx.Tx, r *domain.Repair) error {
	for i := range r.Items {
		it := &r.Items[i]
		it.TenantID = r.TenantID
		it.RepairID = r.ID
		it.SortOrder = i
		if it.ID == "" {
			it.ID = uuid.NewString()
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO repair_items (id, tenant_id, repair_id, kind, description, quantity, unit_price_cents, line_total_cents, cost_cents, line_cost_cents, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING created_at
		`, it.ID, it.TenantID, it.RepairID, string(it.Kind), it.Description,
			it.Quantity, it.UnitPriceCents, it.LineTotalCents,
			it.CostCents, it.LineCostCents, it.SortOrder).
			Scan(&it.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert repair item: %w", err)
		}
	}
	return nil
}
