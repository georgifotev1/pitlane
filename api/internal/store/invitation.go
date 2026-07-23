package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrDuplicateInvitation is returned when a pending invitation already exists
// for this email in this tenant (the partial unique index from migration
// 0008). The handler maps it to a 422 on the email field.
var ErrDuplicateInvitation = errors.New("pending invitation already exists for this email")

// InviteEmailEnqueuer enqueues the invitation-email job. Create calls it
// INSIDE the same transaction that inserts the invitation, so an invitation
// never exists without a dispatched email attempt (River's transactional
// enqueue, ADR §17). The plaintext token travels only into the job args.
type InviteEmailEnqueuer interface {
	EnqueueInviteEmail(ctx context.Context, tx pgx.Tx, tenantID, invitationID, email, role, token string) error
}

// InvitationStore manages staff invitations: tenant-scoped CRUD plus the
// token-based accept path. Tokens are SHA-256 hashed at rest, single-use
// (accepted_at), and expiring.
type InvitationStore struct {
	db *DB
}

// NewInvitationStore builds a store.
func NewInvitationStore(db *DB) *InvitationStore {
	return &InvitationStore{db: db}
}

// invitationColumns is the canonical select order; scanInvitation reads it.
const invitationColumns = "id, tenant_id, email, role, token_hash, invited_by, expires_at, accepted_at, created_at"

func scanInvitation(row pgx.Row) (*domain.Invitation, error) {
	var inv domain.Invitation
	if err := row.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.TokenHash,
		&inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); err != nil {
		return nil, err
	}
	return &inv, nil
}

// Create inserts a pending invitation and enqueues the invite email in the
// same transaction. A second pending invite for the same email returns
// ErrDuplicateInvitation. The inviter must have checked the email is not
// already a registered user (the handler does, via GetByEmail); the DB-level
// backstop for the race lives in Accept.
func (s *InvitationStore) Create(ctx context.Context, inv *domain.Invitation, plainToken string, enq InviteEmailEnqueuer) error {
	return s.db.WithTenant(ctx, inv.TenantID, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			INSERT INTO invitations (id, tenant_id, email, role, token_hash, invited_by, expires_at)
			VALUES ($1, $2, lower($3), $4, $5, $6, $7)
			RETURNING created_at
		`, inv.ID, inv.TenantID, inv.Email, string(inv.Role), inv.TokenHash, inv.InvitedBy, inv.ExpiresAt).
			Scan(&inv.CreatedAt)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return ErrDuplicateInvitation
			}
			return fmt.Errorf("insert invitation: %w", err)
		}

		if err := enq.EnqueueInviteEmail(ctx, tx, inv.TenantID, inv.ID, inv.Email, string(inv.Role), plainToken); err != nil {
			return fmt.Errorf("enqueue invite email: %w", err)
		}
		return nil
	})
}

// ListPending returns the tenant's unaccepted invitations, newest first.
// Accepted rows are history and never listed; revoked rows are deleted.
func (s *InvitationStore) ListPending(ctx context.Context, tenantID string) ([]*domain.Invitation, error) {
	var invitations []*domain.Invitation
	err := s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT `+invitationColumns+`
			FROM invitations
			WHERE tenant_id = $1 AND accepted_at IS NULL
			ORDER BY created_at DESC, id DESC
		`, tenantID)
		if err != nil {
			return fmt.Errorf("list invitations: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			inv, err := scanInvitation(rows)
			if err != nil {
				return fmt.Errorf("scan invitation: %w", err)
			}
			invitations = append(invitations, inv)
		}
		return rows.Err()
	})
	return invitations, err
}

// Delete revokes a pending invitation by removing its row (hard delete — an
// invitation carries no history worth keeping, and the unique index then lets
// the email be re-invited). Returns ErrNotFound when the id is unknown to the
// tenant.
func (s *InvitationStore) Delete(ctx context.Context, tenantID, id string) error {
	return s.db.WithTenant(ctx, tenantID, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			DELETE FROM invitations WHERE id = $1 AND tenant_id = $2
		`, id, tenantID)
		if err != nil {
			return fmt.Errorf("delete invitation: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// GetByToken looks up an invitation by its token hash via the SECURITY
// DEFINER function — the accept endpoint is public, so no tenant context
// exists yet. Expiry and single-use are enforced in Accept; here the lookup
// alone distinguishes unknown tokens (ErrNotFound).
func (s *InvitationStore) GetByToken(ctx context.Context, tokenHash string) (*domain.Invitation, error) {
	inv, err := scanInvitation(s.db.pool.QueryRow(ctx, `
		SELECT `+invitationColumns+` FROM get_invitation_by_token($1)
	`, tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lookup invitation: %w", err)
	}
	return inv, nil
}

// Accept consumes an invitation: in one transaction it marks the invitation
// accepted (race-safe single-use) and inserts the new user with the invited
// role. Returns ErrInvalidToken when the invitation is expired or already
// accepted, and ErrDuplicateEmail when the email registered elsewhere in the
// meantime (23505 backstop for the handler's GetByEmail pre-check).
func (s *InvitationStore) Accept(ctx context.Context, inv *domain.Invitation, user *domain.User) error {
	if inv.AcceptedAt != nil {
		return ErrInvalidToken
	}
	return s.db.WithTenant(ctx, inv.TenantID, func(tx pgx.Tx) error {
		ct, err := tx.Exec(ctx, `
			UPDATE invitations SET accepted_at = now()
			WHERE id = $1 AND accepted_at IS NULL
		`, inv.ID)
		if err != nil {
			return fmt.Errorf("mark invitation accepted: %w", err)
		}
		if ct.RowsAffected() == 0 {
			return ErrInvalidToken
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, tenant_id, email, password_hash, role, name)
			VALUES ($1, $2, lower($3), $4, $5, $6)
		`, user.ID, inv.TenantID, user.Email, user.PasswordHash, string(inv.Role), user.Name)
		return mapUserWriteError(err)
	})
}
