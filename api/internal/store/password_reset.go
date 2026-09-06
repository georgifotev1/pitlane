package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/jackc/pgx/v5"
)

const (
	resetThrottleMax    = 3
	resetThrottleWindow = time.Hour
)

var ErrResetThrottled = errors.New("too many password reset requests")

type PasswordResetTokenStore struct {
	db *DB
}

func NewPasswordResetTokenStore(db *DB) *PasswordResetTokenStore {
	return &PasswordResetTokenStore{db: db}
}

func (s *PasswordResetTokenStore) RequestReset(ctx context.Context, user *domain.User, tokenHash string, expiresAt time.Time) error {
	return s.db.WithTenant(ctx, user.TenantID, func(tx pgx.Tx) error {
		var recent int
		if err := tx.QueryRow(ctx, `
			SELECT count(*)
			FROM password_reset_tokens t
			JOIN users u ON u.id = t.user_id
			WHERE t.user_id = $1 AND u.tenant_id = $2
			  AND t.created_at > now() - $3::interval
		`, user.ID, user.TenantID, resetThrottleWindow.String()).Scan(&recent); err != nil {
			return fmt.Errorf("count recent reset tokens: %w", err)
		}
		if recent >= resetThrottleMax {
			return ErrResetThrottled
		}

		if _, err := tx.Exec(ctx, `
			UPDATE password_reset_tokens t SET expires_at = now()
			FROM users u
			WHERE t.user_id = $1 AND u.id = t.user_id AND u.tenant_id = $2
			  AND t.used_at IS NULL
		`, user.ID, user.TenantID); err != nil {
			return fmt.Errorf("expire previous reset tokens: %w", err)
		}
		ct, err := tx.Exec(ctx, `
			INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
			SELECT id, $2, $3 FROM users WHERE id = $1 AND tenant_id = $4
		`, user.ID, tokenHash, expiresAt, user.TenantID)
		if err != nil {
			return fmt.Errorf("insert reset token: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func (s *PasswordResetTokenStore) Consume(ctx context.Context, tokenHash, newPasswordHash string) (*domain.User, error) {
	row := s.db.pool.QueryRow(ctx, `
		SELECT id, user_id, tenant_id, expires_at, used_at
		FROM get_password_reset_token($1)
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
		ct, err := tx.Exec(ctx, `
			UPDATE password_reset_tokens t SET used_at = now()
			FROM users u
			WHERE t.id = $1 AND t.user_id = $2 AND u.id = t.user_id
			  AND u.tenant_id = $3 AND t.used_at IS NULL AND t.expires_at > now()
		`, tokenID, userID, tenantID)
		if err != nil {
			return fmt.Errorf("consume reset token: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrInvalidToken
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM password_reset_tokens t
			USING users u
			WHERE t.user_id = $1 AND t.id <> $2 AND u.id = t.user_id
			  AND u.tenant_id = $3
		`, userID, tokenID, tenantID); err != nil {
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
