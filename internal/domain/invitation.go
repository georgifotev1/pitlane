package domain

import "time"

const InvitationTokenTTL = 72 * time.Hour

type Invitation struct {
	ID         string
	TenantID   string
	Email      string
	Role       Role
	TokenHash  string
	InvitedBy  string
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	CreatedAt  time.Time
}
