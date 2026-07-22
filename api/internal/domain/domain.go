// Package domain holds the business types and rules that are independent of
// the API boundary. DTOs serialize; these do not.
package domain

import "time"

// Role enumerates the fixed account roles. Permission grants are derived from
// RolePermissions below.
type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleMechanic Role = "mechanic"
)

// Permission enumerates the coarse actions the SPA can gate. Object-level
// checks (e.g. "does this user belong to this tenant?") live in the store or
// handler, not here.
type Permission string

const (
	PermissionCustomersRead  Permission = "customers:read"
	PermissionCustomersWrite Permission = "customers:write"
	PermissionCarsRead       Permission = "cars:read"
	PermissionCarsWrite      Permission = "cars:write"
	PermissionOffersRead     Permission = "offers:read"
	PermissionOffersWrite    Permission = "offers:write"
	PermissionRepairsRead    Permission = "repairs:read"
	PermissionRepairsWrite   Permission = "repairs:write"
	PermissionUsersRead      Permission = "users:read"
	PermissionUsersWrite     Permission = "users:write"
	PermissionSettingsWrite  Permission = "settings:write"
)

// RolePermissions maps each role to the permissions it holds. The owner can do
// everything; admin and mechanic are subsets for the staff-invitation flow.
var RolePermissions = map[Role][]Permission{
	RoleOwner: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionCarsRead, PermissionCarsWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionUsersRead, PermissionUsersWrite,
		PermissionSettingsWrite,
	},
	RoleAdmin: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionCarsRead, PermissionCarsWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
	},
	RoleMechanic: {
		PermissionCustomersRead,
		PermissionCarsRead,
		PermissionOffersRead,
		PermissionRepairsRead, PermissionRepairsWrite,
	},
}

// HasPermission reports whether a role holds the given permission.
func HasPermission(role Role, p Permission) bool {
	for _, allowed := range RolePermissions[role] {
		if allowed == p {
			return true
		}
	}
	return false
}

// PermissionsFor returns the permission list for a role, or nil for unknown roles.
func PermissionsFor(role Role) []Permission {
	return RolePermissions[role]
}

// Tenant is the root entity that owns all data for one garage.
type Tenant struct {
	ID             string
	Name           string
	Address        string
	VATNumber      string
	LogoKey        string
	Currency       string
	Locale         string
	DefaultTaxRate int32
	Settings       map[string]any
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// User belongs to exactly one tenant. Email is globally unique.
type User struct {
	ID           string
	TenantID     string
	Email        string
	PasswordHash string
	Role         Role
	Name         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Customer is a person or company a garage does business with. It belongs to
// exactly one tenant. Optional contact fields are plain strings (empty when
// absent); ArchivedAt is the soft-delete marker (nil = active).
type Customer struct {
	ID         string
	TenantID   string
	Name       string
	Company    string
	Email      string
	Phone      string
	Address    string
	Notes      string
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Car is a vehicle belonging to a Customer within a tenant. plate is unique per
// tenant among active cars (see migration 0003). Optional fields are plain
// values (empty string / 0 = unknown); ArchivedAt is the soft-delete marker.
type Car struct {
	ID         string
	TenantID   string
	CustomerID string
	Plate      string
	VIN        string
	Make       string
	Model      string
	Year       int
	Mileage    int
	ArchivedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// IsValidRole reports whether a role string is one of the known roles.
func IsValidRole(s string) bool {
	switch Role(s) {
	case RoleOwner, RoleAdmin, RoleMechanic:
		return true
	}
	return false
}

// OfferStatus is the lifecycle state of an offer (migration 0004 CHECK).
// Content is mutable only while draft.
type OfferStatus string

const (
	OfferStatusDraft    OfferStatus = "draft"
	OfferStatusSent     OfferStatus = "sent"
	OfferStatusAccepted OfferStatus = "accepted"
	OfferStatusRejected OfferStatus = "rejected"
	OfferStatusExpired  OfferStatus = "expired"
)

// SendStatus tracks the email-delivery lifecycle, surfaced for River-job
// visibility (Phase 7). Meaningless until the offer is sent.
type SendStatus string

const (
	SendStatusPending SendStatus = "pending"
	SendStatusSent    SendStatus = "sent"
	SendStatusFailed  SendStatus = "failed"
)

// OfferItemKind classifies a line for PDF grouping and later reporting.
type OfferItemKind string

const (
	OfferItemKindPart  OfferItemKind = "part"
	OfferItemKindLabor OfferItemKind = "labor"
	OfferItemKindOther OfferItemKind = "other"
)

// Offer is a repair quote written for one Car (the customer is reached through
// the car). Money is integer cents; the Subtotal/Tax/Total snapshots are
// derived from Items and recomputed on every draft write via Recompute, so the
// stored figures always match the lines and freeze at send.
type Offer struct {
	ID            string
	TenantID      string
	CarID         string
	Status        OfferStatus
	SendStatus    SendStatus
	SentTo        string
	SentAt        *time.Time
	TaxRateBps    int
	SubtotalCents int64
	TaxCents      int64
	TotalCents    int64
	Notes         string
	Items         []OfferItem
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// OfferItem is one line of an offer. LineTotalCents is derived
// (UnitPriceCents * Quantity) and set by Recompute, never trusted from input.
type OfferItem struct {
	ID             string
	TenantID       string
	OfferID        string
	Kind           OfferItemKind
	Description    string
	Quantity       int
	UnitPriceCents int64
	LineTotalCents int64
	SortOrder      int
	CreatedAt      time.Time
}

// Recompute is the single source of truth for offer math. It sets each item's
// LineTotalCents from its unit price and quantity, sums them into
// SubtotalCents, applies the snapshotted tax rate, and totals. The store calls
// it on every draft write so the DB snapshot never drifts from the lines.
func (o *Offer) Recompute() {
	var subtotal int64
	for i := range o.Items {
		line := o.Items[i].UnitPriceCents * int64(o.Items[i].Quantity)
		o.Items[i].LineTotalCents = line
		subtotal += line
	}
	o.SubtotalCents = subtotal
	o.TaxCents = TaxCents(subtotal, o.TaxRateBps)
	o.TotalCents = subtotal + o.TaxCents
}

// TaxCents applies a basis-points rate to a cent amount, rounding half up to
// the nearest cent (1900 bps = 19%). Both inputs are non-negative (DB CHECKs),
// so half-up is unambiguous and rounding happens exactly once, at the offer
// level — line math stays exact.
func TaxCents(subtotalCents int64, taxRateBps int) int64 {
	return (subtotalCents*int64(taxRateBps) + 5000) / 10000
}

// offerTransitions is the allowed status machine for the generic status
// endpoint: sent → accepted | rejected | expired. accepted/rejected/expired
// are terminal. The draft → sent transition is deliberately absent: an offer
// only becomes `sent` by actually being emailed (POST /offers/{id}/send,
// Phase 7), so a `sent` offer always carries a recipient and a dispatched
// send job — the freeze-on-send ≡ emailed-PDF invariant (ADR §14) holds by
// construction. SetStatus therefore cannot send; only the send path can.
var offerTransitions = map[OfferStatus][]OfferStatus{
	OfferStatusSent: {OfferStatusAccepted, OfferStatusRejected, OfferStatusExpired},
}

// CanTransitionTo reports whether an offer may move from its current status to
// next.
func (s OfferStatus) CanTransitionTo(next OfferStatus) bool {
	for _, allowed := range offerTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// IsValidOfferStatus reports whether a string is one of the known statuses.
func IsValidOfferStatus(s string) bool {
	switch OfferStatus(s) {
	case OfferStatusDraft, OfferStatusSent, OfferStatusAccepted, OfferStatusRejected, OfferStatusExpired:
		return true
	}
	return false
}

// IsValidOfferItemKind reports whether a string is one of the known kinds.
func IsValidOfferItemKind(s string) bool {
	switch OfferItemKind(s) {
	case OfferItemKindPart, OfferItemKindLabor, OfferItemKindOther:
		return true
	}
	return false
}
