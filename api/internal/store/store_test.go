package store

import (
	"context"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
)

func newTenant(name string) *domain.Tenant {
	return &domain.Tenant{
		ID:             uuid.NewString(),
		Name:           name,
		Currency:       "EUR",
		Locale:         "en",
		DefaultTaxRate: 1900,
		Settings:       map[string]any{},
	}
}

func newUser(tenantID, email, name, role string) *domain.User {
	return &domain.User{
		ID:       uuid.NewString(),
		TenantID: tenantID,
		Email:    email,
		Name:     name,
		Role:     domain.Role(role),
	}
}

func TestTenantStore(t *testing.T) {
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	ts := NewTenantStore(NewDB(tdb.Pool))
	ctx := context.Background()

	t.Run("Create and GetByID", func(t *testing.T) {
		want := newTenant("Pitlane Garage")
		if err := ts.Create(ctx, want); err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		got, err := ts.GetByID(ctx, want.ID)
		if err != nil {
			t.Fatalf("get tenant: %v", err)
		}
		if got.ID != want.ID || got.Name != want.Name {
			t.Fatalf("tenant mismatch: got %+v, want %+v", got, want)
		}
	})

	t.Run("GetByID cross-tenant returns not found", func(t *testing.T) {
		a := newTenant("A")
		b := newTenant("B")
		if err := ts.Create(ctx, a); err != nil {
			t.Fatalf("create a: %v", err)
		}
		if err := ts.Create(ctx, b); err != nil {
			t.Fatalf("create b: %v", err)
		}
		_, err := ts.GetByID(ctx, a.ID)
		if err != nil {
			t.Fatalf("get a: %v", err)
		}
		if _, err := ts.GetByID(ctx, "00000000-0000-0000-0000-000000000000"); err == nil {
			t.Fatalf("expected not found for fake id")
		}
	})
}

func TestUserStore(t *testing.T) {
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	us := NewUserStore(NewDB(tdb.Pool))
	ts := NewTenantStore(NewDB(tdb.Pool))
	ctx := context.Background()

	tenant := newTenant("Garage")
	if err := ts.Create(ctx, tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}

	t.Run("Create and GetByID", func(t *testing.T) {
		u := newUser(tenant.ID, "owner@example.com", "Owner", "owner")
		u.PasswordHash = "fake-hash"
		if err := us.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		got, err := us.GetByID(ctx, tenant.ID, u.ID)
		if err != nil {
			t.Fatalf("get user: %v", err)
		}
		if got.Email != u.Email || got.Role != u.Role {
			t.Fatalf("user mismatch: got %+v, want %+v", got, u)
		}
	})

	t.Run("GetByID cross-tenant not found", func(t *testing.T) {
		otherTenant := newTenant("Other")
		if err := ts.Create(ctx, otherTenant); err != nil {
			t.Fatalf("create other tenant: %v", err)
		}
		u := newUser(otherTenant.ID, "other@example.com", "Other", "owner")
		u.PasswordHash = "fake-hash"
		if err := us.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		_, err := us.GetByID(ctx, tenant.ID, u.ID)
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("GetByEmail global lookup", func(t *testing.T) {
		u := newUser(tenant.ID, "lookup@example.com", "Lookup", "mechanic")
		u.PasswordHash = "fake-hash"
		if err := us.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		got, err := us.GetByEmail(ctx, "LOOKUP@EXAMPLE.COM")
		if err != nil {
			t.Fatalf("get by email: %v", err)
		}
		if got.Email != "lookup@example.com" || got.Role != domain.Role("mechanic") {
			t.Fatalf("email lookup mismatch: got %+v", got)
		}
		_, err = us.GetByEmail(ctx, "missing@example.com")
		if err != ErrNotFound {
			t.Fatalf("expected ErrNotFound for missing email, got %v", err)
		}
	})
}
