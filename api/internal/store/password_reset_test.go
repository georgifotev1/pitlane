package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gfotev/pitlane/internal/testdb"
)

func resetTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestPasswordResetTokenStore(t *testing.T) {
	tdb := testdb.New(t)
	db := NewDB(tdb.Pool)
	tenants := NewTenantStore(db)
	users := NewUserStore(db)
	resets := NewPasswordResetTokenStore(db)
	ctx := context.Background()

	seedUser := func(email string) *struct{ tenantID, userID string } {
		t.Helper()
		tenant := newTenant("Reset Garage " + email)
		owner := newUser(tenant.ID, email, "Owner", "owner")
		owner.PasswordHash = "old-hash"
		if err := tenants.CreateWithOwner(ctx, tenant, owner); err != nil {
			t.Fatalf("seed user: %v", err)
		}
		return &struct{ tenantID, userID string }{tenant.ID, owner.ID}
	}

	t.Run("request stores only the hash and supersedes older tokens", func(t *testing.T) {
		ids := seedUser("request@example.com")
		user, err := users.GetByID(ctx, ids.tenantID, ids.userID)
		if err != nil {
			t.Fatal(err)
		}
		if err := resets.RequestReset(ctx, user, resetTokenHash("old"), time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if err := resets.RequestReset(ctx, user, resetTokenHash("new"), time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		if _, err := resets.Consume(ctx, resetTokenHash("old"), "new-hash"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("superseded token returned %v", err)
		}
		updated, err := resets.Consume(ctx, resetTokenHash("new"), "new-hash")
		if err != nil {
			t.Fatal(err)
		}
		if updated.PasswordHash != "new-hash" || updated.PasswordChangedAt == nil {
			t.Fatalf("password was not updated: %+v", updated)
		}
		if _, err := resets.Consume(ctx, resetTokenHash("new"), "again"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("reused token returned %v", err)
		}
	})

	t.Run("request is tenant scoped", func(t *testing.T) {
		mine := seedUser("mine@example.com")
		other := seedUser("other@example.com")
		user, _ := users.GetByID(ctx, mine.tenantID, mine.userID)
		user.TenantID = other.tenantID
		if err := resets.RequestReset(ctx, user, resetTokenHash("cross-tenant"), time.Now().Add(time.Hour)); !errors.Is(err, ErrNotFound) {
			t.Fatalf("cross-tenant request returned %v", err)
		}
	})

	t.Run("request is throttled", func(t *testing.T) {
		ids := seedUser("throttle@example.com")
		user, _ := users.GetByID(ctx, ids.tenantID, ids.userID)
		for i := range resetThrottleMax {
			if err := resets.RequestReset(ctx, user, resetTokenHash(fmt.Sprintf("token-%d", i)), time.Now().Add(time.Hour)); err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
		}
		if err := resets.RequestReset(ctx, user, resetTokenHash("too-many"), time.Now().Add(time.Hour)); !errors.Is(err, ErrResetThrottled) {
			t.Fatalf("got %v; want ErrResetThrottled", err)
		}
	})

	t.Run("unknown and expired tokens are rejected", func(t *testing.T) {
		if _, err := resets.Consume(ctx, resetTokenHash("unknown"), "x"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("unknown token returned %v", err)
		}
		ids := seedUser("expired@example.com")
		user, _ := users.GetByID(ctx, ids.tenantID, ids.userID)
		if err := resets.RequestReset(ctx, user, resetTokenHash("expired"), time.Now().Add(-time.Minute)); err != nil {
			t.Fatal(err)
		}
		if _, err := resets.Consume(ctx, resetTokenHash("expired"), "x"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expired token returned %v", err)
		}
	})
}
