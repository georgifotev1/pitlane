package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/testdb"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// recordingInviteEnqueuer captures invite-email dispatches (plaintext token
// included), so tests can assert the in-tx enqueue without a River client.
type recordingInviteEnqueuer struct {
	calls []string // email|role|token, in call order
	err   error
}

func (e *recordingInviteEnqueuer) EnqueueInviteEmail(_ context.Context, _ pgx.Tx, _, _, email, role, token string) error {
	if e.err != nil {
		return e.err
	}
	e.calls = append(e.calls, email+"|"+role+"|"+token)
	return nil
}

func TestInvitationStore(t *testing.T) {
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := NewDB(tdb.Pool)
	ts := NewTenantStore(db)
	us := NewUserStore(db)
	is := NewInvitationStore(db)
	ctx := context.Background()

	seed := func(t *testing.T, email string) (tenantID string, ownerID string) {
		t.Helper()
		tenant := newTenant("Invite Garage " + email)
		owner := newUser(tenant.ID, email, "Owner", "owner")
		owner.PasswordHash = "fake-hash"
		if err := ts.CreateWithOwner(ctx, tenant, owner); err != nil {
			t.Fatalf("seed tenant: %v", err)
		}
		return tenant.ID, owner.ID
	}

	newInvite := func(tenantID, invitedBy, email string, role domain.Role, token string, expires time.Time) *domain.Invitation {
		return &domain.Invitation{
			ID:        uuid.NewString(),
			TenantID:  tenantID,
			Email:     email,
			Role:      role,
			TokenHash: hashToken(token),
			InvitedBy: invitedBy,
			ExpiresAt: expires,
		}
	}

	t.Run("Create inserts and enqueues; duplicate pending rejected", func(t *testing.T) {
		tenantID, ownerID := seed(t, "owner@dup-invite.com")
		enq := &recordingInviteEnqueuer{}
		inv := newInvite(tenantID, ownerID, "Mech@Example.com", domain.RoleMechanic, "tok-1", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, inv, "tok-1", enq); err != nil {
			t.Fatalf("create: %v", err)
		}
		if inv.CreatedAt.IsZero() {
			t.Fatal("created_at not stamped")
		}
		if len(enq.calls) != 1 || enq.calls[0] != "Mech@Example.com|mechanic|tok-1" {
			t.Fatalf("expected one enqueue, got %v", enq.calls)
		}
		// Email is stored lower-cased so the pending-unique index is case-proof.
		var stored string
		if err := tdb.Pool.QueryRow(ctx, "SELECT email FROM invitations WHERE id = $1", inv.ID).Scan(&stored); err != nil {
			t.Fatalf("read email: %v", err)
		}
		if stored != "mech@example.com" {
			t.Fatalf("email not lower-cased: %q", stored)
		}

		second := newInvite(tenantID, ownerID, "mech@example.COM", domain.RoleAdmin, "tok-2", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, second, "tok-2", enq); !errors.Is(err, ErrDuplicateInvitation) {
			t.Fatalf("expected ErrDuplicateInvitation, got %v", err)
		}
	})

	t.Run("Create rolls back the invitation when enqueue fails", func(t *testing.T) {
		tenantID, ownerID := seed(t, "owner@rb-invite.com")
		enq := &recordingInviteEnqueuer{err: errors.New("river down")}
		inv := newInvite(tenantID, ownerID, "a@b.com", domain.RoleMechanic, "tok-rb", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, inv, "tok-rb", enq); err == nil {
			t.Fatal("expected enqueue failure")
		}
		got, err := is.ListPending(ctx, tenantID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("invitation must have rolled back, got %d", len(got))
		}
	})

	t.Run("ListPending is tenant-scoped and hides accepted", func(t *testing.T) {
		tenantA, ownerA := seed(t, "owner@list-a.com")
		tenantB, ownerB := seed(t, "owner@list-b.com")
		enq := &recordingInviteEnqueuer{}

		invA := newInvite(tenantA, ownerA, "a1@x.com", domain.RoleMechanic, "tok-a1", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, invA, "tok-a1", enq); err != nil {
			t.Fatalf("create a1: %v", err)
		}
		invB := newInvite(tenantB, ownerB, "b1@x.com", domain.RoleAdmin, "tok-b1", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, invB, "tok-b1", enq); err != nil {
			t.Fatalf("create b1: %v", err)
		}

		gotA, err := is.ListPending(ctx, tenantA)
		if err != nil {
			t.Fatalf("list A: %v", err)
		}
		if len(gotA) != 1 || gotA[0].Email != "a1@x.com" {
			t.Fatalf("A must see only its own invite, got %+v", gotA)
		}

		// Accept A's invite — it must disappear from the pending list.
		user := &domain.User{ID: uuid.NewString(), Email: "a1@x.com", PasswordHash: "h", Name: "A One"}
		if err := is.Accept(ctx, invA, user); err != nil {
			t.Fatalf("accept: %v", err)
		}
		gotA, err = is.ListPending(ctx, tenantA)
		if err != nil {
			t.Fatalf("list A after accept: %v", err)
		}
		if len(gotA) != 0 {
			t.Fatalf("accepted invite must not be pending, got %+v", gotA)
		}
	})

	t.Run("Delete revokes and re-invite then works", func(t *testing.T) {
		tenantID, ownerID := seed(t, "owner@del-invite.com")
		enq := &recordingInviteEnqueuer{}
		inv := newInvite(tenantID, ownerID, "gone@x.com", domain.RoleMechanic, "tok-del", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, inv, "tok-del", enq); err != nil {
			t.Fatalf("create: %v", err)
		}
		if err := is.Delete(ctx, tenantID, inv.ID); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := is.GetByToken(ctx, hashToken("tok-del")); !errors.Is(err, ErrNotFound) {
			t.Fatalf("revoked token must not resolve, got %v", err)
		}
		// The unique index no longer blocks a fresh invite for the same email.
		again := newInvite(tenantID, ownerID, "gone@x.com", domain.RoleAdmin, "tok-del2", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, again, "tok-del2", enq); err != nil {
			t.Fatalf("re-invite after revoke: %v", err)
		}
		if err := is.Delete(ctx, tenantID, uuid.NewString()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("delete unknown id must be ErrNotFound, got %v", err)
		}
	})

	t.Run("GetByToken resolves via the pre-tenant lookup", func(t *testing.T) {
		tenantID, ownerID := seed(t, "owner@bytoken.com")
		enq := &recordingInviteEnqueuer{}
		inv := newInvite(tenantID, ownerID, "tok@x.com", domain.RoleAdmin, "tok-lookup", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, inv, "tok-lookup", enq); err != nil {
			t.Fatalf("create: %v", err)
		}
		got, err := is.GetByToken(ctx, hashToken("tok-lookup"))
		if err != nil {
			t.Fatalf("get by token: %v", err)
		}
		if got.ID != inv.ID || got.TenantID != tenantID || got.Role != domain.RoleAdmin {
			t.Fatalf("lookup mismatch: %+v", got)
		}
		if _, err := is.GetByToken(ctx, hashToken("never-issued")); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown token must be ErrNotFound, got %v", err)
		}
	})

	t.Run("Accept creates the user with the invited role, single-use", func(t *testing.T) {
		tenantID, ownerID := seed(t, "owner@accept.com")
		enq := &recordingInviteEnqueuer{}
		inv := newInvite(tenantID, ownerID, "new@x.com", domain.RoleMechanic, "tok-acc", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, inv, "tok-acc", enq); err != nil {
			t.Fatalf("create: %v", err)
		}

		user := &domain.User{ID: uuid.NewString(), Email: "New@X.com", PasswordHash: "bcrypt-hash", Name: "New Mech"}
		if err := is.Accept(ctx, inv, user); err != nil {
			t.Fatalf("accept: %v", err)
		}
		got, err := us.GetByEmail(ctx, "new@x.com")
		if err != nil {
			t.Fatalf("user not created: %v", err)
		}
		if got.Role != domain.RoleMechanic || got.TenantID != tenantID || got.Name != "New Mech" {
			t.Fatalf("user mismatch: %+v", got)
		}

		// Second accept of the same invitation fails.
		again := &domain.User{ID: uuid.NewString(), Email: "new@x.com", PasswordHash: "h", Name: "Dupe"}
		if err := is.Accept(ctx, inv, again); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("second accept must be ErrInvalidToken, got %v", err)
		}
	})

	t.Run("Accept rejects a duplicate email via the DB backstop", func(t *testing.T) {
		tenantID, ownerID := seed(t, "owner@race.com")
		enq := &recordingInviteEnqueuer{}
		// The invited email registers elsewhere after the invite was sent.
		other := newTenant("Other Garage")
		otherOwner := newUser(other.ID, "raced@x.com", "Raced", "owner")
		otherOwner.PasswordHash = "h"
		if err := ts.CreateWithOwner(ctx, other, otherOwner); err != nil {
			t.Fatalf("seed other: %v", err)
		}

		inv := newInvite(tenantID, ownerID, "raced@x.com", domain.RoleMechanic, "tok-race", time.Now().Add(72*time.Hour))
		if err := is.Create(ctx, inv, "tok-race", enq); err != nil {
			t.Fatalf("create: %v", err)
		}
		user := &domain.User{ID: uuid.NewString(), Email: "raced@x.com", PasswordHash: "h", Name: "Late"}
		if err := is.Accept(ctx, inv, user); !errors.Is(err, ErrDuplicateEmail) {
			t.Fatalf("expected ErrDuplicateEmail, got %v", err)
		}

		// The invitation must NOT be consumed by the failed accept: the whole
		// tx rolled back, so it is still pending.
		pending, err := is.ListPending(ctx, tenantID)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(pending) != 1 {
			t.Fatalf("failed accept must leave the invite pending, got %d", len(pending))
		}
	})
}
