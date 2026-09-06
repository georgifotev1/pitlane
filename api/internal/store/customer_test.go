package store

import (
	"context"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

func newCustomer(tenantID, name string) *domain.Customer {
	return &domain.Customer{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Name:     name,
	}
}

func TestCustomerStore(t *testing.T) {
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := NewDB(tdb.Pool)
	cs := NewCustomerStore(db)
	ts := NewTenantStore(db)
	ctx := context.Background()

	tenant := newTenant("Garage")
	if err := ts.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	t.Run("Create populates timestamps and Get round-trips", func(t *testing.T) {
		c := newCustomer(tenant.ID, "Ivan Petrov")
		c.Company = "Petrov Ltd"
		c.Email = "ivan@example.com"
		c.Phone = "+359888123456"
		c.Address = "Sofia"
		c.Notes = "prefers mornings"

		if err := cs.Create(ctx, c); err != nil {
			t.Fatalf("create customer: %v", err)
		}
		if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
			t.Fatalf("expected timestamps populated, got %+v", c)
		}

		got, err := cs.Get(ctx, tenant.ID, c.ID)
		if err != nil {
			t.Fatalf("get customer: %v", err)
		}
		if got.Name != c.Name || got.Email != c.Email || got.Company != c.Company || got.Phone != c.Phone {
			t.Fatalf("customer mismatch: got %+v, want %+v", got, c)
		}
		if got.ArchivedAt != nil {
			t.Fatalf("new customer should not be archived")
		}
	})

	t.Run("Get unknown id returns ErrNotFound", func(t *testing.T) {
		_, err := cs.Get(ctx, tenant.ID, uuid.NewString())
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Update writes fields and bumps updated_at", func(t *testing.T) {
		c := newCustomer(tenant.ID, "Georgi")
		if err := cs.Create(ctx, c); err != nil {
			t.Fatalf("create: %v", err)
		}
		firstUpdated := c.UpdatedAt

		c.Name = "Georgi Dimitrov"
		c.Phone = "+359000"
		if err := cs.Update(ctx, c); err != nil {
			t.Fatalf("update: %v", err)
		}
		got, err := cs.Get(ctx, tenant.ID, c.ID)
		if err != nil {
			t.Fatalf("get after update: %v", err)
		}
		if got.Name != "Georgi Dimitrov" || got.Phone != "+359000" {
			t.Fatalf("update not persisted: %+v", got)
		}
		if !got.UpdatedAt.After(firstUpdated) {
			t.Fatalf("updated_at not bumped: first %v, now %v", firstUpdated, got.UpdatedAt)
		}
	})

	t.Run("Update unknown id returns ErrNotFound", func(t *testing.T) {
		ghost := newCustomer(tenant.ID, "Ghost")
		if err := cs.Update(ctx, ghost); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("Archive hides from default list, second archive is ErrNotFound", func(t *testing.T) {
		c := newCustomer(tenant.ID, "Archivable")
		if err := cs.Create(ctx, c); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := cs.Archive(ctx, tenant.ID, c.ID); err != nil {
			t.Fatalf("archive: %v", err)
		}
		got, err := cs.Get(ctx, tenant.ID, c.ID)
		if err != nil {
			t.Fatalf("get archived: %v", err)
		}
		if got.ArchivedAt == nil {
			t.Fatalf("archived_at should be set")
		}
		if err := cs.Archive(ctx, tenant.ID, c.ID); err != ErrNotFound {
			t.Fatalf("second archive: expected ErrNotFound, got %v", err)
		}
	})

	t.Run("List paginates, searches, and filters archived", func(t *testing.T) {
		lt := newTenant("ListGarage")
		if err := ts.Create(ctx, lt); err != nil {
			t.Fatalf("create list tenant: %v", err)
		}
		names := []string{"Alpha", "Bravo", "Charlie", "Delta"}
		for _, n := range names {
			c := newCustomer(lt.ID, n)
			c.Email = n + "@example.com"
			if err := cs.Create(ctx, c); err != nil {
				t.Fatalf("create %s: %v", n, err)
			}
		}

		all, total, err := cs.List(ctx, lt.ID, CustomerListParams{Limit: 2, Offset: 0})
		if err != nil {
			t.Fatalf("list page 1: %v", err)
		}
		if total != 4 {
			t.Fatalf("total: got %d, want 4", total)
		}
		if len(all) != 2 || all[0].Name != "Alpha" || all[1].Name != "Bravo" {
			t.Fatalf("page 1 order wrong: %+v", names2(all))
		}
		page2, _, err := cs.List(ctx, lt.ID, CustomerListParams{Limit: 2, Offset: 2})
		if err != nil {
			t.Fatalf("list page 2: %v", err)
		}
		if len(page2) != 2 || page2[0].Name != "Charlie" {
			t.Fatalf("page 2 wrong: %+v", names2(page2))
		}

		found, total, err := cs.List(ctx, lt.ID, CustomerListParams{Search: "brav", Limit: 10})
		if err != nil {
			t.Fatalf("search: %v", err)
		}
		if total != 1 || len(found) != 1 || found[0].Name != "Bravo" {
			t.Fatalf("search wrong: total %d, %+v", total, names2(found))
		}

		if err := cs.Archive(ctx, lt.ID, all[0].ID); err != nil {
			t.Fatalf("archive: %v", err)
		}
		active, activeTotal, err := cs.List(ctx, lt.ID, CustomerListParams{Limit: 10})
		if err != nil {
			t.Fatalf("list active: %v", err)
		}
		if activeTotal != 3 || len(active) != 3 {
			t.Fatalf("active total: got %d, want 3", activeTotal)
		}
		_, withArchivedTotal, err := cs.List(ctx, lt.ID, CustomerListParams{IncludeArchived: true, Limit: 10})
		if err != nil {
			t.Fatalf("list incl archived: %v", err)
		}
		if withArchivedTotal != 4 {
			t.Fatalf("incl-archived total: got %d, want 4", withArchivedTotal)
		}
	})

	t.Run("cross-tenant access is invisible", func(t *testing.T) {
		other := newTenant("Other")
		if err := ts.Create(ctx, other); err != nil {
			t.Fatalf("create other tenant: %v", err)
		}
		mine := newCustomer(tenant.ID, "Mine")
		if err := cs.Create(ctx, mine); err != nil {
			t.Fatalf("create mine: %v", err)
		}

		if _, err := cs.Get(ctx, other.ID, mine.ID); err != ErrNotFound {
			t.Fatalf("cross-tenant Get: expected ErrNotFound, got %v", err)
		}
		poison := *mine
		poison.TenantID = other.ID
		poison.Name = "Hacked"
		if err := cs.Update(ctx, &poison); err != ErrNotFound {
			t.Fatalf("cross-tenant Update: expected ErrNotFound, got %v", err)
		}
		if err := cs.Archive(ctx, other.ID, mine.ID); err != ErrNotFound {
			t.Fatalf("cross-tenant Archive: expected ErrNotFound, got %v", err)
		}
		got, err := cs.Get(ctx, tenant.ID, mine.ID)
		if err != nil {
			t.Fatalf("get mine: %v", err)
		}
		if got.Name != "Mine" || got.ArchivedAt != nil {
			t.Fatalf("customer was mutated cross-tenant: %+v", got)
		}
	})
}

func names2(cs []*domain.Customer) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
