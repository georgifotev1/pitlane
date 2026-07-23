package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/jackc/pgx/v5"
)

// Per-email reset throttle (ADR decision 18, the Postgres half): at most this
// many tokens per user per window. The in-IP rate limiter is the other half.
const (
	resetThrottleMax    = 3
	resetThrottleWindow = time.Hour
)

// ErrResetThrottled is returned when a user has too many recent reset tokens.
// The handler swallows it into the same enumeration-safe 204 as every other
// outcome — the legitimate user simply stops receiving emails for a while.
var ErrResetThrottled = errors.New("too many password reset requests")

// PasswordResetEmailEnqueuer enqueues the reset-email job. RequestReset calls
// it INSIDE the same transaction that inserts the token, so a token row never
// exists without a dispatched email attempt (River's transactional enqueue,
// ADR §17). The plaintext token travels only into the job args — the table
// holds the SHA-256 hash.
type PasswordResetEmailEnqueuer interface {
	EnqueuePasswordResetEmail(ctx context.Context, tx pgx.Tx, tenantID, userID, email, userName, token string) error
}

// PasswordResetTokenStore manages the reset-token lifecycle. Tokens are
// SHA-256 hashed at rest, single-use, and expire (ADR §Security).
type PasswordResetTokenStore struct {
	db *DB
}

// NewPasswordResetTokenStore builds a store.
func NewPasswordResetTokenStore(db *DB) *PasswordResetTokenStore {
	return &PasswordResetTokenStore{db: db}
}

// RequestReset records a reset request for an existing user and enqueues the
// email atomically. At most one active token exists per user: any still-unused
// predecessors are SUPERSEDED (expired in place) rather than deleted — the
// throttle above counts live rows created in the window, and deleting them
// would let an impatient user reset the counter with every request.
// Returns ErrResetThrottled when the user is over the per-hour limit (no row
// written, no job enqueued).
func (s *PasswordResetTokenStore) RequestReset(ctx context.Context, user *domain.User, tokenHash, plainToken string, expiresAt time.Time, enq PasswordResetEmailEnqueuer) error {
	return s.db.WithTenant(ctx, user.TenantID, func(tx pgx.Tx) error {
		var recent int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM password_reset_tokens
			WHERE user_id = $1 AND created_at > now() - $2::interval
		`, user.ID, resetThrottleWindow.String()).Scan(&recent); err != nil {
			return fmt.Errorf("count recent reset tokens: %w", err)
		}
		if recent >= resetThrottleMax {
			return ErrResetThrottled
		}

		// Supersede (don't delete) still-unused predecessors: only the newest
		// token can be consumed, and the rows keep counting toward the throttle.
		if _, err := tx.Exec(ctx, `
			UPDATE password_reset_tokens SET expires_at = now()
			WHERE user_id = $1 AND used_at IS NULL
		`, user.ID); err != nil {
			return fmt.Errorf("supersede unused reset tokens: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
			VALUES ($1, $2, $3)
		`, user.ID, tokenHash, expiresAt); err != nil {
			return fmt.Errorf("insert reset token: %w", err)
		}

		if err := enq.EnqueuePasswordResetEmail(ctx, tx, user.TenantID, user.ID, user.Email, user.Name, plainToken); err != nil {
			return fmt.Errorf("enqueue reset email: %w", err)
		}
		return nil
	})
}

// Consume validates a token by its hash and, in one transaction, marks it
// used, deletes every other token for the user, and sets the new password
// (stamping password_changed_at, which invalidates all existing sessions).
// Returns the updated user. Any validation failure is ErrInvalidToken — the
// caller cannot tell unknown/expired/used apart, by design.
func (s *PasswordResetTokenStore) Consume(ctx context.Context, tokenHash, newPasswordHash string) (*domain.User, error) {
	// Pre-tenant lookup via the SECURITY DEFINER function (the reset endpoint
	// is public, so no tenant context exists yet).
	row := s.db.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, expires_at, used_at FROM get_password_reset_token($1)
	`, tokenHash)
	var tokenID, userID, tenantID string
	var expiresAt time.Time
	var usedAt *time.Time
	if err := row.Scan(&tokenID, &userID, &tenantID, &expiresAt, &usedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, fmt.Errorf("lookup reset token: %w", err)
	}
	if usedAt != nil || time.Now().After(expiresAt) {
		return nil, ErrInvalidToken
	}

	var user *domain.User
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		// Single-use, race-safe: the update only lands if the token is still
		// unused, so a concurrent consume of the same token loses.
		ct, err := tx.Exec(ctx, `
			UPDATE password_reset_tokens SET used_at = now()
			WHERE id = $1 AND used_at IS NULL
		`, tokenID)
		if err != nil {
			return fmt.Errorf("mark reset token used: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrInvalidToken
		}

		// ADR §Security: success deletes all other tokens for this user.
		if _, err := tx.Exec(ctx, `
			DELETE FROM password_reset_tokens WHERE user_id = $1 AND id <> $2
		`, userID, tokenID); err != nil {
			return fmt.Errorf("delete other reset tokens: %w", err)
		}

		if err := updatePasswordTx(ctx, tx, tenantID, userID, newPasswordHash); err != nil {
			return err
		}

		u, err := scanUser(tx.QueryRow(ctx,
			`SELECT `+userColumns+` FROM users WHERE id = $1 AND tenant_id = $2`,
			userID, tenantID))
		if err != nil {
			return fmt.Errorf("reload user: %w", err)
		}
		user = u
		return nil
	})
	if err != nil {
		return nil, err
	}
	return user, nil
}
