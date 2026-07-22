package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrRepairNotOpen is returned when a write targets a repair that is no longer
// open. Repair content is immutable once work starts (ADR §Domain Model),
// mirroring the offer's draft-only rule. The handler maps it to a 409.
var ErrRepairNotOpen = errors.New("repair is not open")

// ErrInvalidRepairStatusTransition is returned when a status change is not
// allowed by the lifecycle machine (domain.repairTransitions). Handler → 409.
var ErrInvalidRepairStatusTransition = errors.New("invalid repair status transition")

// ErrOfferNotAcceptable is returned when a conversion is attempted on an offer
// that is not in the `sent` state (a draft has not been quoted to the customer;
// an accepted/rejected/expired offer is terminal). Handler → 409.
var ErrOfferNotAcceptable = errors.New("offer is not in an acceptable state")

// RepairStore follows the OfferStore pattern: column list + scan helper
// co-located, every method runs inside WithTenant, every query filters on
// tenant_id. A repair owns its items, so open writes replace the item set
// wholesale inside the same transaction and Recompute keeps the money snapshots
// in step. Repairs are born by converting an offer (CreateFromOffer); there is
// no from-scratch create in this phase.
type RepairStore struct {
	db *DB
}

// NewRepairStore builds a store.
func NewRepairStore(db *DB) *RepairStore {
	return &RepairStore{db: db}
}

// repairColumns is the canonical select order; scanRepair reads in this order.
const repairColumns = "id, tenant_id, car_id, offer_id, status, tax_rate_bps, subtotal_cents, tax_cents, total_cents, mileage, notes, completed_at, created_at, updated_at"

func scanRepair(row pgx.Row) (*domain.Repair, error) {
	var r domain.Repair
	if err := row.Scan(
		&r.ID, &r.TenantID, &r.CarID, &r.OfferID, &r.Status, &r.TaxRateBps,
		&r.SubtotalCents, &r.TaxCents, &r.TotalCents, &r.Mileage, &r.Notes,
		&r.CompletedAt, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &r, nil
}

// repairItemColumns is the canonical select order; scanRepairItem reads it.
const repairItemColumns = "id, tenant_id, repair_id, kind, description, quantity, unit_price_cents, line_total_cents, sort_order, created_at"

func scanRepairItem(row pgx.Row) (*domain.RepairItem, error) {
	var it domain.RepairItem
	if err := row.Scan(
		&it.ID, &it.TenantID, &it.RepairID, &it.Kind, &it.Description,
		&it.Quantity, &it.UnitPriceCents, &it.LineTotalCents, &it.SortOrder, &it.CreatedAt,
	); err != nil {
		return nil, err
	}
	return &it, nil
}

// RepairSummary is a board row: the repair plus the car plate and customer name
// it belongs to, joined in for display so the client need not fan out N reads.
// Items are not loaded for the board (Get loads them for the detail view).
type RepairSummary struct {
	Repair       *domain.Repair
	CarPlate     string
	CustomerName string
}

// RepairListParams controls the board query. Status "" means "all statuses";
// zero Limit means "no rows" (the handler clamps to a sane page size).
type RepairListParams struct {
	Status string
	Limit  int
	Offset int
}

// List returns a tenant-wide page of repairs (newest first), optionally
// filtered by status, plus the total count matching the filter. Each row is
// enriched with the car plate and customer name via joins that stay in-tenant
// (composite keys), so the board renders without extra round-trips.
func (s *RepairStore) List(ctx context.Context, tenantID string, p RepairListParams) ([]RepairSummary, int, error) {
	var out []RepairSummary
	var total int

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		// $1 tenant, $2 status ('' = any).
		const where = `WHERE r.tenant_id = $1 AND ($2 = '' OR r.status = $2)`

		if err := tx.QueryRow(ctx,
			`SELECT count(*) FROM repairs r `+where,
			tenantID, p.Status,
		).Scan(&total); err != nil {
			return fmt.Errorf("count repairs: %w", err)
		}

		rows, err := tx.Query(ctx,
			`SELECT r.id, r.tenant_id, r.car_id, r.offer_id, r.status, r.tax_rate_bps,
			        r.subtotal_cents, r.tax_cents, r.total_cents, r.mileage, r.notes,
			        r.completed_at, r.created_at, r.updated_at,
			        c.plate, cust.name
			 FROM repairs r
			 JOIN cars c ON c.id = r.car_id AND c.tenant_id = r.tenant_id
			 JOIN customers cust ON cust.id = c.customer_id AND cust.tenant_id = c.tenant_id
			 `+where+`
			 ORDER BY r.created_at DESC, r.id DESC
			 LIMIT $3 OFFSET $4`,
			tenantID, p.Status, p.Limit, p.Offset,
		)
		if err != nil {
			return fmt.Errorf("list repairs: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var r domain.Repair
			var plate, customerName string
			if err := rows.Scan(
				&r.ID, &r.TenantID, &r.CarID, &r.OfferID, &r.Status, &r.TaxRateBps,
				&r.SubtotalCents, &r.TaxCents, &r.TotalCents, &r.Mileage, &r.Notes,
				&r.CompletedAt, &r.CreatedAt, &r.UpdatedAt,
				&plate, &customerName,
			); err != nil {
				return fmt.Errorf("scan repair summary: %w", err)
			}
			out = append(out, RepairSummary{Repair: &r, CarPlate: plate, CustomerName: customerName})
		}
		return rows.Err()
	})
	return out, total, err
}

// Get returns one repair with its items in editor order, scoped to the tenant,
// or ErrNotFound.
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

// CreateFromOffer is the offer→repair conversion, and the ONLY way a repair is
// born. In one transaction it: locks the offer, gates that it is `sent`, copies
// its line items into a new open repair (fresh item IDs — a copy, never a
// shared reference, so later repair edits never touch the offer), links
// provenance (offer_id), snapshots the tax rate + notes, and flips the offer to
// `accepted`. Because accept and convert are the same atomic step, an accepted
// offer always has exactly one repair (ADR §Domain Model).
//
// The offer row is locked FOR UPDATE (inside getOfferTx's reload path we re-read
// it; the lock is taken explicitly first) so a concurrent send/accept cannot
// race the gate. Returns ErrNotFound if the offer is absent, or
// ErrOfferNotAcceptable if it is not `sent`.
func (s *RepairStore) CreateFromOffer(ctx context.Context, tenantID, offerID string) (*domain.Repair, error) {
	var repair *domain.Repair
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		// Lock the offer and gate on status before copying anything, so a
		// concurrent accept/send cannot slip past and double-convert.
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
		if status != domain.OfferStatusSent {
			return ErrOfferNotAcceptable
		}

		// Load the offer with its (frozen) items to copy from.
		offer, err := getOfferTx(ctx, tx, tenantID, offerID)
		if err != nil {
			return err
		}

		// Build the repair as a faithful copy: same car, tax rate, notes, and
		// a fresh item per offer line (Recompute reproduces the frozen totals).
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
			})
		}
		r.Recompute()

		if err := tx.QueryRow(ctx, `
			INSERT INTO repairs (id, tenant_id, car_id, offer_id, status, tax_rate_bps, subtotal_cents, tax_cents, total_cents, notes)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING created_at, updated_at
		`, r.ID, r.TenantID, r.CarID, r.OfferID, string(r.Status),
			r.TaxRateBps, r.SubtotalCents, r.TaxCents, r.TotalCents, r.Notes).
			Scan(&r.CreatedAt, &r.UpdatedAt); err != nil {
			return fmt.Errorf("insert repair: %w", err)
		}
		if err := insertRepairItems(ctx, tx, r); err != nil {
			return err
		}

		// Accept the offer in the same tx: convert ≡ accept.
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

// Update rewrites an open repair's editable fields (tax rate, notes) and
// replaces its item set, recomputing totals. It is open-only: a repair that has
// started (or completed) returns ErrRepairNotOpen, and a missing repair returns
// ErrNotFound. car_id and offer_id are immutable.
func (s *RepairStore) Update(ctx context.Context, r *domain.Repair) error {
	r.Recompute()

	return s.db.WithTenant(ctx, r.TenantID, func(tx pgx.Tx) error {
		// Lock and gate on status before touching anything, so a concurrent
		// status change can't slip a write past the open-only rule.
		var status domain.RepairStatus
		err := tx.QueryRow(ctx,
			`SELECT status FROM repairs WHERE id = $1 AND tenant_id = $2 FOR UPDATE`,
			r.ID, r.TenantID,
		).Scan(&status)
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

		return tx.QueryRow(ctx, `
			UPDATE repairs
			SET tax_rate_bps = $3, subtotal_cents = $4, tax_cents = $5, total_cents = $6, notes = $7, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
			RETURNING updated_at
		`, r.ID, r.TenantID, r.TaxRateBps, r.SubtotalCents, r.TaxCents, r.TotalCents, r.Notes).
			Scan(&r.UpdatedAt)
	})
}

// SetStatus advances a repair through the generic lifecycle machine
// (open ↔ in_progress), enforcing the allowed transitions. Completion is NOT
// reachable here — that is the Complete path. Returns ErrNotFound if absent, or
// ErrInvalidRepairStatusTransition if the move is not allowed. On success it
// returns the reloaded repair with items.
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

// Complete finishes a repair: status → completed, completed_at stamped, the
// odometer reading recorded on the repair AND advanced onto the car — all in
// one transaction, so a completed repair always carries a mileage reading and
// the car's mileage reflects the latest job. The car mileage only ever moves
// forward (GREATEST), since an odometer does not run backwards; a lower reading
// is kept on the repair record but does not roll the car back.
//
// Completable from open or in_progress; a repair that is already completed
// returns ErrRepairNotOpen (the "not writable" signal), and a missing repair
// returns ErrNotFound. On success it returns the reloaded repair with items.
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

		// Advance the car's odometer, never backwards.
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

// getRepairTx loads one repair plus its items within an existing tenant tx.
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

// loadRepairItems reads a repair's items in editor order within an existing tx.
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

// insertRepairItems writes a repair's items, stamping tenant_id, repair_id, and
// sort_order from slice position. The store owns item identity because items
// are replaced wholesale on every open write, so it assigns any missing IDs.
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
			INSERT INTO repair_items (id, tenant_id, repair_id, kind, description, quantity, unit_price_cents, line_total_cents, sort_order)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING created_at
		`, it.ID, it.TenantID, it.RepairID, string(it.Kind), it.Description,
			it.Quantity, it.UnitPriceCents, it.LineTotalCents, it.SortOrder).
			Scan(&it.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert repair item: %w", err)
		}
	}
	return nil
}
