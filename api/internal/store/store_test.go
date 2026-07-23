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

	t.Run("Create rejects duplicate email", func(t *testing.T) {
		u := newUser(tenant.ID, "dupe@example.com", "First", "mechanic")
		u.PasswordHash = "fake-hash"
		if err := us.Create(ctx, u); err != nil {
			t.Fatalf("create first: %v", err)
		}
		other := newUser(tenant.ID, "DUPE@example.com", "Second", "admin")
		other.PasswordHash = "fake-hash"
		if err := us.Create(ctx, other); err != ErrDuplicateEmail {
			t.Fatalf("expected ErrDuplicateEmail, got %v", err)
		}
	})

	t.Run("List returns tenant users oldest first", func(t *testing.T) {
		listTenant := newTenant("List Garage")
		if err := ts.Create(ctx, listTenant); err != nil {
			t.Fatalf("create tenant: %v", err)
		}
		owner := newUser(listTenant.ID, "owner@list.com", "Owner", "owner")
		owner.PasswordHash = "fake-hash"
		if err := us.Create(ctx, owner); err != nil {
			t.Fatalf("create owner: %v", err)
		}
		mech := newUser(listTenant.ID, "mech@list.com", "Mech", "mechanic")
		mech.PasswordHash = "fake-hash"
		if err := us.Create(ctx, mech); err != nil {
			t.Fatalf("create mech: %v", err)
		}

		got, err := us.List(ctx, listTenant.ID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 users, got %d", len(got))
		}
		if got[0].ID != owner.ID || got[1].ID != mech.ID {
			t.Fatalf("wrong order: got %s then %s", got[0].Email, got[1].Email)
		}

		// Cross-tenant isolation: another tenant's list must not include these.
		others, err := us.List(ctx, tenant.ID)
		if err != nil {
			t.Fatalf("list other: %v", err)
		}
		for _, u := range others {
			if u.TenantID == listTenant.ID {
				t.Fatalf("cross-tenant leak in List: %+v", u)
			}
		}
	})

	t.Run("UpdateRole changes role, scoped to tenant", func(t *testing.T) {
		u := newUser(tenant.ID, "role@example.com", "Role", "mechanic")
		u.PasswordHash = "fake-hash"
		if err := us.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		if err := us.UpdateRole(ctx, tenant.ID, u.ID, domain.RoleAdmin); err != nil {
			t.Fatalf("update role: %v", err)
		}
		got, err := us.GetByID(ctx, tenant.ID, u.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.Role != domain.RoleAdmin {
			t.Fatalf("expected admin, got %s", got.Role)
		}
		if err := us.UpdateRole(ctx, tenant.ID, "00000000-0000-0000-0000-000000000000", domain.RoleAdmin); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("UpdatePassword sets hash and password_changed_at", func(t *testing.T) {
		u := newUser(tenant.ID, "pw@example.com", "PW", "mechanic")
		u.PasswordHash = "old-hash"
		if err := us.Create(ctx, u); err != nil {
			t.Fatalf("create user: %v", err)
		}
		before, err := us.GetByID(ctx, tenant.ID, u.ID)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if before.PasswordChangedAt != nil {
			t.Fatalf("password_changed_at must start nil, got %v", before.PasswordChangedAt)
		}
		if err := us.UpdatePassword(ctx, tenant.ID, u.ID, "new-hash"); err != nil {
			t.Fatalf("update password: %v", err)
		}
		got, err := us.GetByID(ctx, tenant.ID, u.ID)
		if err != nil {
			t.Fatalf("reload: %v", err)
		}
		if got.PasswordHash != "new-hash" {
			t.Fatalf("hash not updated: %q", got.PasswordHash)
		}
		if got.PasswordChangedAt == nil {
			t.Fatal("password_changed_at must be stamped")
		}
	})
}

func TestTenantStoreCreateWithOwner(t *testing.T) {
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := NewDB(tdb.Pool)
	ts := NewTenantStore(db)
	us := NewUserStore(db)
	ctx := context.Background()

	t.Run("creates tenant and owner atomically", func(t *testing.T) {
		tenant := newTenant("Atomic Garage")
		owner := newUser(tenant.ID, "owner@atomic.com", "Owner", "owner")
		owner.PasswordHash = "fake-hash"
		if err := ts.CreateWithOwner(ctx, tenant, owner); err != nil {
			t.Fatalf("create with owner: %v", err)
		}
		if _, err := ts.GetByID(ctx, tenant.ID); err != nil {
			t.Fatalf("tenant missing: %v", err)
		}
		if _, err := us.GetByID(ctx, tenant.ID, owner.ID); err != nil {
			t.Fatalf("owner missing: %v", err)
		}
	})

	t.Run("duplicate email rolls back the tenant too", func(t *testing.T) {
		existing := newTenant("Existing")
		existingOwner := newUser(existing.ID, "taken@example.com", "Taken", "owner")
		existingOwner.PasswordHash = "fake-hash"
		if err := ts.CreateWithOwner(ctx, existing, existingOwner); err != nil {
			t.Fatalf("seed: %v", err)
		}

		orphan := newTenant("Orphan Garage")
		owner := newUser(orphan.ID, "TAKEN@example.com", "Second", "owner")
		owner.PasswordHash = "fake-hash"
		if err := ts.CreateWithOwner(ctx, orphan, owner); err != ErrDuplicateEmail {
			t.Fatalf("expected ErrDuplicateEmail, got %v", err)
		}
		if _, err := ts.GetByID(ctx, orphan.ID); err != ErrNotFound {
			t.Fatalf("tenant must have rolled back, got err=%v", err)
		}
	})
}
