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
	PermissionOffersRead      Permission = "offers:read"
	PermissionOffersWrite     Permission = "offers:write"
	PermissionRepairsRead     Permission = "repairs:read"
	PermissionRepairsWrite    Permission = "repairs:write"
	PermissionHistoryRead     Permission = "history:read"
	PermissionHistoryWrite    Permission = "history:write"
	PermissionAttachmentsRead Permission = "attachments:read"
	PermissionAttachmentsWrite Permission = "attachments:write"
	PermissionUsersRead       Permission = "users:read"
	PermissionUsersWrite      Permission = "users:write"
	PermissionSettingsWrite   Permission = "settings:write"
)

// RolePermissions maps each role to the permissions it holds. The owner can do
// everything; admin and mechanic are subsets for the staff-invitation flow.
var RolePermissions = map[Role][]Permission{
	RoleOwner: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionCarsRead, PermissionCarsWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionHistoryRead, PermissionHistoryWrite,
		PermissionAttachmentsRead, PermissionAttachmentsWrite,
		PermissionUsersRead, PermissionUsersWrite,
		PermissionSettingsWrite,
	},
	RoleAdmin: {
		PermissionCustomersRead, PermissionCustomersWrite,
		PermissionCarsRead, PermissionCarsWrite,
		PermissionOffersRead, PermissionOffersWrite,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionHistoryRead, PermissionHistoryWrite,
		PermissionAttachmentsRead, PermissionAttachmentsWrite,
		PermissionUsersRead, PermissionUsersWrite,
	},
	RoleMechanic: {
		PermissionCustomersRead,
		PermissionCarsRead,
		PermissionOffersRead,
		PermissionRepairsRead, PermissionRepairsWrite,
		PermissionHistoryRead, PermissionHistoryWrite,
		PermissionAttachmentsRead, PermissionAttachmentsWrite,
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
// endpoint: sent → rejected | expired. Both targets are terminal. TWO
// transitions are deliberately absent from this generic machine, each owned by
// a dedicated endpoint that carries a side effect the machine cannot:
//   - draft → sent: only the send path (POST /offers/{id}/send, Phase 7) makes
//     an offer `sent`, so a sent offer always carries a recipient + dispatched
//     job — freeze-on-send ≡ emailed-PDF (ADR §14) holds by construction.
//   - sent → accepted: only the accept path (POST /offers/{id}/accept, Phase 8)
//     accepts an offer, and it does so by converting it into a repair in the
//     same transaction. So an `accepted` offer always has exactly one repair
//     (its price-frozen items copied) — provenance holds by construction.
//
// SetStatus therefore can neither send nor accept; only those paths can.
var offerTransitions = map[OfferStatus][]OfferStatus{
	OfferStatusSent: {OfferStatusRejected, OfferStatusExpired},
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

// RepairStatus is the lifecycle state of a repair (migration 0006 CHECK).
// Content is mutable only while open, mirroring an offer's draft-only rule.
type RepairStatus string

const (
	RepairStatusOpen       RepairStatus = "open"
	RepairStatusInProgress RepairStatus = "in_progress"
	RepairStatusCompleted  RepairStatus = "completed"
)

// Repair is the work performed on a Car. It is a separate entity from Offer:
// its Items are COPIED from the offer at conversion (price freeze), so editing
// a repair never mutates the offer it came from. Money is integer cents;
// Subtotal/Tax/Total are derived from Items by Recompute on every open write
// and freeze on completion. OfferID is the (optional) provenance link.
type Repair struct {
	ID            string
	TenantID      string
	CarID         string
	OfferID       *string
	Status        RepairStatus
	TaxRateBps    int
	SubtotalCents int64
	TaxCents      int64
	TotalCents    int64
	Mileage       int
	Notes         string
	Items         []RepairItem
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// RepairItem is one line of a repair. LineTotalCents is derived
// (UnitPriceCents * Quantity) and set by Recompute, never trusted from input.
type RepairItem struct {
	ID             string
	TenantID       string
	RepairID       string
	Kind           OfferItemKind
	Description    string
	Quantity       int
	UnitPriceCents int64
	LineTotalCents int64
	SortOrder      int
	CreatedAt      time.Time
}

// Recompute is the single source of truth for repair math. It mirrors
// Offer.Recompute exactly (shared TaxCents, round half up at the repair level),
// keeping the DB snapshot in step with the lines on every open write.
func (r *Repair) Recompute() {
	var subtotal int64
	for i := range r.Items {
		line := r.Items[i].UnitPriceCents * int64(r.Items[i].Quantity)
		r.Items[i].LineTotalCents = line
		subtotal += line
	}
	r.SubtotalCents = subtotal
	r.TaxCents = TaxCents(subtotal, r.TaxRateBps)
	r.TotalCents = subtotal + r.TaxCents
}

// repairTransitions is the allowed status machine for the generic repair status
// endpoint: open ↔ in_progress. Completion is deliberately absent: a repair
// only becomes `completed` through the dedicated complete path
// (POST /repairs/{id}/complete, Phase 8), which also records the odometer
// reading and advances the car's mileage in the same transaction. So a
// completed repair always carries a mileage reading and a completed_at — the
// side effect holds by construction, exactly as send/accept do for offers.
var repairTransitions = map[RepairStatus][]RepairStatus{
	RepairStatusOpen:       {RepairStatusInProgress},
	RepairStatusInProgress: {RepairStatusOpen},
}

// CanTransitionTo reports whether a repair may move from its current status to
// next via the generic status endpoint.
func (s RepairStatus) CanTransitionTo(next RepairStatus) bool {
	for _, allowed := range repairTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

// IsValidRepairStatus reports whether a string is one of the known statuses.
func IsValidRepairStatus(s string) bool {
	switch RepairStatus(s) {
	case RepairStatusOpen, RepairStatusInProgress, RepairStatusCompleted:
		return true
	}
	return false
}

// HistoryNote is a manual external-work entry attached to a car. It is the only
// table in the derived service-history model; the other half is completed repairs.
type HistoryNote struct {
	ID          string
	TenantID    string
	CarID       string
	Title       string
	Description string
	RecordedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Attachment is a photo or document stored in R2 (MinIO locally), linked to
// either a car or a repair. Exactly one of CarID or RepairID is set.
type Attachment struct {
	ID          string
	TenantID    string
	CarID       *string
	RepairID    *string
	Name        string
	StorageKey  string
	SizeBytes   int64
	ContentType string
	CreatedAt   time.Time
}
