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
	"github.com/jackc/pgx/v5"
)

// recordingResetEnqueuer captures the enqueue calls RequestReset makes, so
// tests can assert the in-tx dispatch without a real River client.
type recordingResetEnqueuer struct {
	calls []string // plaintext tokens, in call order
	err   error
}

func (e *recordingResetEnqueuer) EnqueuePasswordResetEmail(_ context.Context, _ pgx.Tx, _, _, email, _, token string) error {
	if e.err != nil {
		return e.err
	}
	e.calls = append(e.calls, email+"|"+token)
	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func TestPasswordResetTokenStore(t *testing.T) {
	tdb := testdb.New(t)
	t.Cleanup(func() { tdb.Cleanup(t) })

	db := NewDB(tdb.Pool)
	ts := NewTenantStore(db)
	us := NewUserStore(db)
	rs := NewPasswordResetTokenStore(db)
	ctx := context.Background()

	seedUser := func(t *testing.T, email string) (tenantID string, userID string) {
		t.Helper()
		tenant := newTenant("Reset Garage " + email)
		owner := newUser(tenant.ID, email, "Owner", "owner")
		owner.PasswordHash = "old-hash"
		if err := ts.CreateWithOwner(ctx, tenant, owner); err != nil {
			t.Fatalf("seed tenant: %v", err)
		}
		return tenant.ID, owner.ID
	}

	t.Run("RequestReset inserts a token and enqueues the email", func(t *testing.T) {
		tenantID, userID := seedUser(t, "req@example.com")
		user, err := us.GetByID(ctx, tenantID, userID)
		if err != nil {
			t.Fatalf("get user: %v", err)
		}
		enq := &recordingResetEnqueuer{}
		expires := time.Now().Add(time.Hour)
		if err := rs.RequestReset(ctx, user, hashToken("tok-1"), "tok-1", expires, enq); err != nil {
			t.Fatalf("request reset: %v", err)
		}
		if len(enq.calls) != 1 || enq.calls[0] != "req@example.com|tok-1" {
			t.Fatalf("expected one enqueue with plaintext token, got %v", enq.calls)
		}

		// Only the hash is persisted — never the plaintext token.
		var storedHash string
		err = tdb.Pool.QueryRow(ctx,
			"SELECT token_hash FROM password_reset_tokens WHERE user_id = $1", userID).Scan(&storedHash)
		if err != nil {
			t.Fatalf("read token row: %v", err)
		}
		if storedHash != hashToken("tok-1") {
			t.Fatalf("stored hash mismatch: %q", storedHash)
		}
	})

	t.Run("RequestReset replaces an unused token", func(t *testing.T) {
		tenantID, userID := seedUser(t, "replace@example.com")
		user, _ := us.GetByID(ctx, tenantID, userID)
		enq := &recordingResetEnqueuer{}
		if err := rs.RequestReset(ctx, user, hashToken("tok-old"), "tok-old", time.Now().Add(time.Hour), enq); err != nil {
			t.Fatalf("first request: %v", err)
		}
		if err := rs.RequestReset(ctx, user, hashToken("tok-new"), "tok-new", time.Now().Add(time.Hour), enq); err != nil {
			t.Fatalf("second request: %v", err)
		}

		// The old token is gone: consuming it must fail.
		if _, err := rs.Consume(ctx, hashToken("tok-old"), "new-hash"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("old token must be invalid, got %v", err)
		}
		if _, err := rs.Consume(ctx, hashToken("tok-new"), "new-hash"); err != nil {
			t.Fatalf("new token must consume, got %v", err)
		}
	})

	t.Run("RequestReset throttles after the per-hour cap", func(t *testing.T) {
		tenantID, userID := seedUser(t, "throttle@example.com")
		user, _ := us.GetByID(ctx, tenantID, userID)
		enq := &recordingResetEnqueuer{}
		for i := range resetThrottleMax {
			tok := fmt.Sprintf("tok-%d", i)
			if err := rs.RequestReset(ctx, user, hashToken(tok), tok, time.Now().Add(time.Hour), enq); err != nil {
				t.Fatalf("request %d: %v", i, err)
			}
		}
		// The cap counts live rows created in the window — superseded rows
		// included, which is exactly why RequestReset expires rather than
		// deletes them. The next request exceeds it.
		err := rs.RequestReset(ctx, user, hashToken("tok-x"), "tok-x", time.Now().Add(time.Hour), enq)
		if !errors.Is(err, ErrResetThrottled) {
			t.Fatalf("expected ErrResetThrottled, got %v", err)
		}
		if len(enq.calls) != resetThrottleMax {
			t.Fatalf("throttled request must not enqueue, got %d calls", len(enq.calls))
		}
	})

	t.Run("RequestReset rolls back the token when enqueue fails", func(t *testing.T) {
		tenantID, userID := seedUser(t, "rollback@example.com")
		user, _ := us.GetByID(ctx, tenantID, userID)
		enq := &recordingResetEnqueuer{err: errors.New("river down")}
		err := rs.RequestReset(ctx, user, hashToken("tok-rb"), "tok-rb", time.Now().Add(time.Hour), enq)
		if err == nil {
			t.Fatal("expected enqueue failure")
		}
		var n int
		if err := tdb.Pool.QueryRow(ctx,
			"SELECT count(*) FROM password_reset_tokens WHERE user_id = $1", userID).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Fatalf("token row must have rolled back, got %d", n)
		}
	})

	t.Run("Consume sets the password, kills other tokens, is single-use", func(t *testing.T) {
		tenantID, userID := seedUser(t, "consume@example.com")
		user, _ := us.GetByID(ctx, tenantID, userID)
		enq := &recordingResetEnqueuer{}
		// Two requests: tok-a is superseded by tok-b. Consuming tok-b must
		// succeed; both rows must then be gone (used one kept as history is
		// fine — the check below counts only LIVE tokens).
		if err := rs.RequestReset(ctx, user, hashToken("tok-a"), "tok-a", time.Now().Add(time.Hour), enq); err != nil {
			t.Fatalf("request a: %v", err)
		}
		if err := rs.RequestReset(ctx, user, hashToken("tok-b"), "tok-b", time.Now().Add(time.Hour), enq); err != nil {
			t.Fatalf("request b: %v", err)
		}
		// The superseded token no longer consumes.
		if _, err := rs.Consume(ctx, hashToken("tok-a"), "stale-hash"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("superseded token must fail, got %v", err)
		}

		updated, err := rs.Consume(ctx, hashToken("tok-b"), "brand-new-hash")
		if err != nil {
			t.Fatalf("consume: %v", err)
		}
		if updated.ID != userID || updated.TenantID != tenantID {
			t.Fatalf("wrong user returned: %+v", updated)
		}
		if updated.PasswordHash != "brand-new-hash" || updated.PasswordChangedAt == nil {
			t.Fatalf("password not rotated: %+v", updated)
		}

		var n int
		if err := tdb.Pool.QueryRow(ctx,
			"SELECT count(*) FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL AND expires_at > now()", userID).Scan(&n); err != nil {
			t.Fatalf("count live: %v", err)
		}
		if n != 0 {
			t.Fatalf("no live tokens may remain after consume, got %d", n)
		}

		// Second use of the same token fails.
		if _, err := rs.Consume(ctx, hashToken("tok-b"), "another-hash"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("second use must fail, got %v", err)
		}
	})

	t.Run("Consume rejects unknown and expired tokens", func(t *testing.T) {
		if _, err := rs.Consume(ctx, hashToken("never-issued"), "x"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("unknown token: got %v", err)
		}

		tenantID, userID := seedUser(t, "expired@example.com")
		user, _ := us.GetByID(ctx, tenantID, userID)
		enq := &recordingResetEnqueuer{}
		if err := rs.RequestReset(ctx, user, hashToken("tok-exp"), "tok-exp", time.Now().Add(-time.Minute), enq); err != nil {
			t.Fatalf("request: %v", err)
		}
		if _, err := rs.Consume(ctx, hashToken("tok-exp"), "x"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expired token: got %v", err)
		}
	})
}
