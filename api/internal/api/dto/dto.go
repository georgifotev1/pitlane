// Package dto defines the API boundary types. Only these types serialize to
// JSON; tygo generates the frontend's TypeScript types from them.
package dto

import "time"

type HealthResponse struct {
	Status      string `json:"status"`
	Environment string `json:"environment"`
}

type SignupRequest struct {
	TenantName string `json:"tenantName"`
	UserName   string `json:"userName"`
	Email      string `json:"email"`
	Password   string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID          string   `json:"id"`
	TenantID    string   `json:"tenantId"`
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Permissions []string `json:"permissions"`
}

type SignupResponse struct {
	User UserResponse `json:"user"`
}

type MeResponse struct {
	User UserResponse `json:"user"`
}

// ListMetadata is the pagination envelope carried alongside every list
// response: `{"customers": [...], "metadata": {...}}`. Reused by all list
// endpoints (ADR §8 offset/page pagination with metadata).
type ListMetadata struct {
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Total    int `json:"total"`
}

// CustomerResponse is the customer as seen by the client. ArchivedAt is null
// for active customers (tygo maps *time.Time → string | null).
type CustomerResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Company    string     `json:"company"`
	Email      string     `json:"email"`
	Phone      string     `json:"phone"`
	Address    string     `json:"address"`
	Notes      string     `json:"notes"`
	ArchivedAt *time.Time `json:"archivedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// CustomerListResponse is the full body of the list endpoint. The client reads
// the whole object (both keys), unlike single-entity envelopes.
type CustomerListResponse struct {
	Customers []CustomerResponse `json:"customers"`
	Metadata  ListMetadata       `json:"metadata"`
}

// CreateCustomerRequest / UpdateCustomerRequest share a shape today, but stay
// distinct types so they can diverge without a breaking rename (Create may gain
// server-only defaults; Update is a full PUT replace).
type CreateCustomerRequest struct {
	Name    string `json:"name"`
	Company string `json:"company"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	Notes   string `json:"notes"`
}

type UpdateCustomerRequest struct {
	Name    string `json:"name"`
	Company string `json:"company"`
	Email   string `json:"email"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
	Notes   string `json:"notes"`
}

// CarResponse is a car as seen by the client. It belongs to a customer
// (CustomerID); ArchivedAt is null for active cars. year/mileage are 0 when
// unknown (tygo maps them to number).
type CarResponse struct {
	ID         string     `json:"id"`
	CustomerID string     `json:"customerId"`
	Plate      string     `json:"plate"`
	VIN        string     `json:"vin"`
	Make       string     `json:"make"`
	Model      string     `json:"model"`
	Year       int        `json:"year"`
	Mileage    int        `json:"mileage"`
	ArchivedAt *time.Time `json:"archivedAt"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// CarListResponse is the full body of the list endpoint (both keys read by the
// client), mirroring CustomerListResponse.
type CarListResponse struct {
	Cars     []CarResponse `json:"cars"`
	Metadata ListMetadata  `json:"metadata"`
}

// CreateCarRequest / UpdateCarRequest stay distinct types even though they share
// a shape today (matching the customer pattern). CustomerID is not in the body:
// on create it comes from the nested route path; it is immutable on update.
type CreateCarRequest struct {
	Plate   string `json:"plate"`
	VIN     string `json:"vin"`
	Make    string `json:"make"`
	Model   string `json:"model"`
	Year    int    `json:"year"`
	Mileage int    `json:"mileage"`
}

type UpdateCarRequest struct {
	Plate   string `json:"plate"`
	VIN     string `json:"vin"`
	Make    string `json:"make"`
	Model   string `json:"model"`
	Year    int    `json:"year"`
	Mileage int    `json:"mileage"`
}

// OfferItemResponse is one line of an offer. Money is integer cents (tygo maps
// int64 → number). lineTotalCents is derived server-side (unitPrice × quantity).
type OfferItemResponse struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unitPriceCents"`
	LineTotalCents int64  `json:"lineTotalCents"`
	SortOrder      int    `json:"sortOrder"`
}

// OfferResponse is an offer as seen by the client. All money is integer cents;
// the subtotal/tax/total snapshots are computed server-side and frozen at send.
// sentAt is null until the offer is emailed (Phase 7). Items is empty on list
// responses (detail fetch loads the lines).
type OfferResponse struct {
	ID            string              `json:"id"`
	CarID         string              `json:"carId"`
	Status        string              `json:"status"`
	SendStatus    string              `json:"sendStatus"`
	SentTo        string              `json:"sentTo"`
	SentAt        *time.Time          `json:"sentAt"`
	TaxRateBps    int                 `json:"taxRateBps"`
	SubtotalCents int64               `json:"subtotalCents"`
	TaxCents      int64               `json:"taxCents"`
	TotalCents    int64               `json:"totalCents"`
	Notes         string              `json:"notes"`
	Items         []OfferItemResponse `json:"items"`
	CreatedAt     time.Time           `json:"createdAt"`
	UpdatedAt     time.Time           `json:"updatedAt"`
}

// OfferListResponse is the full body of the list endpoint (both keys read by
// the client), mirroring CarListResponse.
type OfferListResponse struct {
	Offers   []OfferResponse `json:"offers"`
	Metadata ListMetadata    `json:"metadata"`
}

// OfferItemRequest is one line in a create/update payload. lineTotalCents and
// the offer totals are never accepted from the client — they are recomputed.
type OfferItemRequest struct {
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unitPriceCents"`
}

// CreateOfferRequest creates a draft offer under the car named in the route.
// The tax rate is snapshotted from the tenant default at creation, so it is not
// part of the create body.
type CreateOfferRequest struct {
	Notes string             `json:"notes"`
	Items []OfferItemRequest `json:"items"`
}

// UpdateOfferRequest is a full replace of a draft offer's editable fields.
// taxRateBps is editable while the offer is a draft (the create snapshot can be
// adjusted before sending).
type UpdateOfferRequest struct {
	Notes      string             `json:"notes"`
	TaxRateBps int                `json:"taxRateBps"`
	Items      []OfferItemRequest `json:"items"`
}

// UpdateOfferStatusRequest advances an offer's lifecycle status (the post-send
// transitions: accepted | rejected | expired). Sending is a separate endpoint.
type UpdateOfferStatusRequest struct {
	Status string `json:"status"`
}

// SendOfferRequest emails a draft offer to a customer (or retries a failed
// send). The recipient is prefilled from the customer on the client but is
// editable, so it travels in the body and is validated as an email address.
type SendOfferRequest struct {
	Recipient string `json:"recipient"`
}

// RepairItemResponse is one line of a repair. Same shape as OfferItemResponse:
// the copy made at conversion is faithful. Money is integer cents.
type RepairItemResponse struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Description    string `json:"description"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unitPriceCents"`
	LineTotalCents int64  `json:"lineTotalCents"`
	SortOrder      int    `json:"sortOrder"`
}

// RepairResponse is a repair as seen by the client. offerId is null for a
// repair with no source quote; completedAt is null until completion. Money is
// integer cents, snapshotted server-side and frozen on completion. mileage is
// the odometer reading captured at completion (0 while open/in_progress).
type RepairResponse struct {
	ID            string               `json:"id"`
	CarID         string               `json:"carId"`
	OfferID       *string              `json:"offerId"`
	Status        string               `json:"status"`
	TaxRateBps    int                  `json:"taxRateBps"`
	SubtotalCents int64                `json:"subtotalCents"`
	TaxCents      int64                `json:"taxCents"`
	TotalCents    int64                `json:"totalCents"`
	Mileage       int                  `json:"mileage"`
	Notes         string               `json:"notes"`
	Items         []RepairItemResponse `json:"items"`
	CompletedAt   *time.Time           `json:"completedAt"`
	CreatedAt     time.Time            `json:"createdAt"`
	UpdatedAt     time.Time            `json:"updatedAt"`
}

// RepairSummaryResponse is one row of the tenant-wide repairs board. It carries
// the car plate and customer name (joined server-side) so the board renders
// without extra round-trips, but not the line items (the detail fetch loads
// those).
type RepairSummaryResponse struct {
	ID           string     `json:"id"`
	CarID        string     `json:"carId"`
	CarPlate     string     `json:"carPlate"`
	CustomerName string     `json:"customerName"`
	OfferID      *string    `json:"offerId"`
	Status       string     `json:"status"`
	TotalCents   int64      `json:"totalCents"`
	Mileage      int        `json:"mileage"`
	CompletedAt  *time.Time `json:"completedAt"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// RepairListResponse is the full body of the board endpoint (both keys read by
// the client), mirroring the other list responses.
type RepairListResponse struct {
	Repairs  []RepairSummaryResponse `json:"repairs"`
	Metadata ListMetadata            `json:"metadata"`
}

// UpdateRepairRequest is a full replace of an open repair's editable fields.
// Items reuse OfferItemRequest — identical shape, and the repair's item set is
// replaced wholesale just like an offer's. taxRateBps is editable while open.
type UpdateRepairRequest struct {
	Notes      string             `json:"notes"`
	TaxRateBps int                `json:"taxRateBps"`
	Items      []OfferItemRequest `json:"items"`
}

// UpdateRepairStatusRequest drives the generic lifecycle (open ↔ in_progress).
// Completion is a separate endpoint (it records the odometer reading).
type UpdateRepairStatusRequest struct {
	Status string `json:"status"`
}

// CompleteRepairRequest finishes a repair, recording the odometer reading that
// is also written onto the car.
type CompleteRepairRequest struct {
	Mileage int `json:"mileage"`
}

// HistoryEntryResponse is one row in the service-history timeline. It is a
// union: either a completed repair or a manual history note.
type HistoryEntryResponse struct {
	Type        string `json:"type"`
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	RecordedAt  string `json:"recordedAt"`
	Mileage     int    `json:"mileage"`
	TotalCents  int64  `json:"totalCents"`
}

// HistoryResponse is the full timeline for a car.
type HistoryResponse struct {
	History []HistoryEntryResponse `json:"history"`
}

// HistoryNoteResponse is a single manual history note.
type HistoryNoteResponse struct {
	ID          string `json:"id"`
	CarID       string `json:"carId"`
	Title       string `json:"title"`
	Description string `json:"description"`
	RecordedAt  string `json:"recordedAt"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// CreateHistoryNoteRequest / UpdateHistoryNoteRequest share a shape.
type CreateHistoryNoteRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	RecordedAt  string `json:"recordedAt"`
}

type UpdateHistoryNoteRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	RecordedAt  string `json:"recordedAt"`
}

// AttachmentResponse is the metadata for an uploaded file.
type AttachmentResponse struct {
	ID          string  `json:"id"`
	CarID       *string `json:"carId"`
	RepairID    *string `json:"repairId"`
	Name        string  `json:"name"`
	SizeBytes   int64   `json:"sizeBytes"`
	ContentType string  `json:"contentType"`
	CreatedAt   string  `json:"createdAt"`
}

// AttachmentListResponse is the list of attachments for a car or repair.
type AttachmentListResponse struct {
	Attachments []AttachmentResponse `json:"attachments"`
}
