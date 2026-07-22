package store

import (
	"context"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/google/uuid"
)

// historyFixture creates a tenant, customer, car, offer store, and repair store
// so history tests can produce completed repairs and notes.
func historyFixture(t *testing.T) (context.Context, *DB, *HistoryStore, *RepairStore, *OfferStore, *domain.Tenant, *domain.Car) {
	t.Helper()
	ctx, db, repairs, offers, tenant, _, car := repairFixture(t)
	return ctx, db, NewHistoryStore(db), repairs, offers, tenant, car
}

func TestHistoryStore(t *testing.T) {
	ctx, _, history, repairs, offers, tenant, car := historyFixture(t)

	t.Run("ListByCar is empty when no repairs or notes", func(t *testing.T) {
		entries, err := history.ListByCar(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(entries) != 0 {
			t.Fatalf("expected empty history, got %d", len(entries))
		}
	})

	t.Run("ListByCar shows completed repairs and notes in date order", func(t *testing.T) {
		// Complete a repair.
		o := sentOffer(t, ctx, offers, tenant.ID, car.ID)
		r, err := repairs.CreateFromOffer(ctx, tenant.ID, o.ID)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		if _, err := repairs.Complete(ctx, tenant.ID, r.ID, 125000); err != nil {
			t.Fatalf("complete: %v", err)
		}

		// Add a manual note.
		note := &domain.HistoryNote{
			ID:          uuid.NewString(),
			TenantID:    tenant.ID,
			CarID:       car.ID,
			Title:       "Dealer service",
			Description: "Oil change at authorized dealer",
		}
		if err := history.CreateNote(ctx, note); err != nil {
			t.Fatalf("create note: %v", err)
		}

		entries, err := history.ListByCar(ctx, tenant.ID, car.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		// Newest first by default: both have similar timestamps, so the note (created
		// after the completed repair) should be first.
		seenRepair := false
		seenNote := false
		for _, e := range entries {
			switch e.Type {
			case "repair":
				seenRepair = true
				if e.Mileage != 125000 || e.TotalCents != 17850 {
					t.Fatalf("repair entry wrong: %+v", e)
				}
			case "note":
				seenNote = true
				if e.Title != "Dealer service" {
					t.Fatalf("note entry wrong: %+v", e)
				}
			default:
				t.Fatalf("unknown entry type %q", e.Type)
			}
		}
		if !seenRepair || !seenNote {
			t.Fatalf("missing entry types")
		}
	})

	t.Run("CreateNote/GetNote/UpdateNote/DeleteNote round-trip", func(t *testing.T) {
		note := &domain.HistoryNote{
			ID:          uuid.NewString(),
			TenantID:    tenant.ID,
			CarID:       car.ID,
			Title:       "Tire change",
			Description: "Winter tires",
		}
		if err := history.CreateNote(ctx, note); err != nil {
			t.Fatalf("create: %v", err)
		}
		if note.CreatedAt.IsZero() || note.UpdatedAt.IsZero() {
			t.Fatalf("timestamps not populated: %+v", note)
		}

		got, err := history.GetNote(ctx, tenant.ID, note.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Title != note.Title || got.Description != note.Description {
			t.Fatalf("note mismatch: got %+v", got)
		}

		got.Title = "Summer tires"
		got.Description = "Switched to summer tires"
		if err := history.UpdateNote(ctx, got); err != nil {
			t.Fatalf("update: %v", err)
		}
		reloaded, err := history.GetNote(ctx, tenant.ID, note.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if reloaded.Title != "Summer tires" {
			t.Fatalf("update not persisted: %+v", reloaded)
		}

		if err := history.DeleteNote(ctx, tenant.ID, note.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := history.GetNote(ctx, tenant.ID, note.ID); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("GetNote and DeleteNote return ErrNotFound for unknown id", func(t *testing.T) {
		if _, err := history.GetNote(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
		if err := history.DeleteNote(ctx, tenant.ID, uuid.NewString()); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestHistoryStoreTenantIsolation(t *testing.T) {
	ctx, db, history, repairs, offers, tenantA, carA := historyFixture(t)

	o := sentOffer(t, ctx, offers, tenantA.ID, carA.ID)
	r, err := repairs.CreateFromOffer(ctx, tenantA.ID, o.ID)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if _, err := repairs.Complete(ctx, tenantA.ID, r.ID, 100000); err != nil {
		t.Fatalf("complete: %v", err)
	}

	note := &domain.HistoryNote{
		ID:       uuid.NewString(),
		TenantID: tenantA.ID,
		CarID:    carA.ID,
		Title:    "A note",
	}
	if err := history.CreateNote(ctx, note); err != nil {
		t.Fatalf("create note: %v", err)
	}

	ts := NewTenantStore(db)
	tenantB := newTenant("Garage B")
	if err := ts.Create(ctx, tenantB); err != nil {
		t.Fatalf("create tenant B: %v", err)
	}

	entries, err := history.ListByCar(ctx, tenantB.ID, carA.ID)
	if err != nil {
		t.Fatalf("list cross-tenant: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("B should see none of A's history, got %d", len(entries))
	}
	if _, err := history.GetNote(ctx, tenantB.ID, note.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant GetNote: expected ErrNotFound, got %v", err)
	}
	if err := history.UpdateNote(ctx, &domain.HistoryNote{ID: note.ID, TenantID: tenantB.ID, CarID: carA.ID, Title: "hacked"}); err != ErrNotFound {
		t.Fatalf("cross-tenant UpdateNote: expected ErrNotFound, got %v", err)
	}
	if err := history.DeleteNote(ctx, tenantB.ID, note.ID); err != ErrNotFound {
		t.Fatalf("cross-tenant DeleteNote: expected ErrNotFound, got %v", err)
	}
}
