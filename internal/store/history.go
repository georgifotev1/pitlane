package store

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type HistoryEntry struct {
	Type        string
	ID          string
	Title       string
	Description string
	RecordedAt  time.Time
	Mileage     int
	TotalCents  int64
}

type HistoryStore struct {
	db *DB
}

func NewHistoryStore(db *DB) *HistoryStore {
	return &HistoryStore{db: db}
}

func (s *HistoryStore) ListByCar(ctx context.Context, tenantID, carID string) ([]HistoryEntry, error) {
	var entries []HistoryEntry

	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		repairRows, err := tx.Query(ctx, `
			SELECT id, mileage, notes, total_cents, completed_at
			FROM repairs
			WHERE tenant_id = $1 AND car_id = $2 AND status = $3
			ORDER BY completed_at DESC, id DESC
		`, tenantID, carID, string(domain.RepairStatusCompleted))
		if err != nil {
			return fmt.Errorf("query completed repairs: %w", err)
		}
		defer repairRows.Close()

		for repairRows.Next() {
			var id, notes string
			var mileage int
			var totalCents int64
			var completedAt time.Time
			if err := repairRows.Scan(&id, &mileage, &notes, &totalCents, &completedAt); err != nil {
				return fmt.Errorf("scan repair history: %w", err)
			}
			entries = append(entries, HistoryEntry{
				Type:        "repair",
				ID:          id,
				Title:       "",
				Description: notes,
				RecordedAt:  completedAt,
				Mileage:     mileage,
				TotalCents:  totalCents,
			})
		}
		if err := repairRows.Err(); err != nil {
			return err
		}

		noteRows, err := tx.Query(ctx, `
			SELECT id, title, description, recorded_at
			FROM history_notes
			WHERE tenant_id = $1 AND car_id = $2
			ORDER BY recorded_at DESC, id DESC
		`, tenantID, carID)
		if err != nil {
			return fmt.Errorf("query history notes: %w", err)
		}
		defer noteRows.Close()

		for noteRows.Next() {
			var id, title, description string
			var recordedAt time.Time
			if err := noteRows.Scan(&id, &title, &description, &recordedAt); err != nil {
				return fmt.Errorf("scan history note: %w", err)
			}
			entries = append(entries, HistoryEntry{
				Type:        "note",
				ID:          id,
				Title:       title,
				Description: description,
				RecordedAt:  recordedAt,
			})
		}
		if err := noteRows.Err(); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	sortHistoryEntries(entries)
	if len(entries) > 500 {
		entries = entries[:500]
	}
	return entries, nil
}

func sortHistoryEntries(entries []HistoryEntry) {
	// Service history is a reverse-chronological timeline: the work an owner is
	// most likely looking for should always be at the top. Sort the combined
	// result rather than relying on the separate repair and note query orders.
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].RecordedAt.After(entries[j].RecordedAt)
	})
}

func (s *HistoryStore) CreateNote(ctx context.Context, n *domain.HistoryNote) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	return s.db.WithTenant(ctx, n.TenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			INSERT INTO history_notes (id, tenant_id, car_id, title, description, recorded_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING created_at, updated_at
		`, n.ID, n.TenantID, n.CarID, n.Title, n.Description, n.RecordedAt).
			Scan(&n.CreatedAt, &n.UpdatedAt)
	})
}

func (s *HistoryStore) GetNote(ctx context.Context, tenantID, id string) (*domain.HistoryNote, error) {
	var note *domain.HistoryNote
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `
			SELECT id, tenant_id, car_id, title, description, recorded_at, created_at, updated_at
			FROM history_notes
			WHERE id = $1 AND tenant_id = $2
		`, id, tenantID)
		var n domain.HistoryNote
		if err := row.Scan(&n.ID, &n.TenantID, &n.CarID, &n.Title, &n.Description, &n.RecordedAt, &n.CreatedAt, &n.UpdatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		note = &n
		return nil
	})
	return note, err
}

func (s *HistoryStore) UpdateNote(ctx context.Context, n *domain.HistoryNote) error {
	return s.db.WithTenant(ctx, n.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE history_notes
			SET title = $3, description = $4, recorded_at = $5, updated_at = now()
			WHERE id = $1 AND tenant_id = $2
			RETURNING updated_at
		`, n.ID, n.TenantID, n.Title, n.Description, n.RecordedAt).
			Scan(&n.UpdatedAt)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrNotFound
			}
			return err
		}
		return nil
	})
}

func (s *HistoryStore) DeleteNote(ctx context.Context, tenantID, id string) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		res, err := tx.Exec(ctx, `
			DELETE FROM history_notes
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
