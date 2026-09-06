package domain

import "time"

type OfferStatus string

const (
	OfferStatusDraft    OfferStatus = "draft"
	OfferStatusSent     OfferStatus = "sent"
	OfferStatusAccepted OfferStatus = "accepted"
	OfferStatusRejected OfferStatus = "rejected"
	OfferStatusExpired  OfferStatus = "expired"
)

type SendStatus string

const (
	SendStatusPending SendStatus = "pending"
	SendStatusSent    SendStatus = "sent"
	SendStatusFailed  SendStatus = "failed"
)

type OfferItemKind string

const (
	OfferItemKindPart  OfferItemKind = "part"
	OfferItemKindLabor OfferItemKind = "labor"
	OfferItemKindOther OfferItemKind = "other"
)

type Offer struct {
	ID             string
	DocumentNumber string
	TenantID       string
	CarID          string
	Status         OfferStatus
	SendStatus     SendStatus
	SentTo         string
	SentAt         *time.Time
	TaxRateBps     int
	SubtotalCents  int64
	TaxCents       int64
	TotalCents     int64
	// Cost snapshots are internal margin data: what the garage paid its
	// suppliers, VAT-inclusive (CostTotalCents) and net (CostSubtotalCents).
	// They never appear on the customer's copy of the offer.
	CostTotalCents    int64
	CostSubtotalCents int64
	Notes             string
	Items             []OfferItem
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type OfferItem struct {
	ID             string
	TenantID       string
	OfferID        string
	Kind           OfferItemKind
	Description    string
	Quantity       int
	UnitPriceCents int64
	LineTotalCents int64
	// CostCents is the unit purchase price paid to the supplier, VAT-inclusive
	// like UnitPriceCents. Labour lines normally leave it at zero.
	CostCents     int64
	LineCostCents int64
	SortOrder     int
	CreatedAt     time.Time
}

func (o *Offer) Recompute() {
	var total, cost int64
	for i := range o.Items {
		line := o.Items[i].UnitPriceCents * int64(o.Items[i].Quantity)
		lineCost := o.Items[i].CostCents * int64(o.Items[i].Quantity)
		o.Items[i].LineTotalCents = line
		o.Items[i].LineCostCents = lineCost
		total += line
		cost += lineCost
	}
	o.TotalCents = total
	o.TaxCents = TaxCents(total, o.TaxRateBps)
	o.SubtotalCents = total - o.TaxCents
	o.CostTotalCents = cost
	o.CostSubtotalCents = NetCents(cost, o.TaxRateBps)
}

// ProfitCents is what the garage keeps: net revenue less net supplier cost.
// Both sides are VAT-exclusive, because VAT is collected for the state and
// input VAT on parts is deductible - neither is the garage's money.
func (o *Offer) ProfitCents() int64 { return o.SubtotalCents - o.CostSubtotalCents }

func (o *Offer) MarginBps() int { return MarginBps(o.ProfitCents(), o.SubtotalCents) }

// Generic transitions exclude acceptance, which creates a repair atomically.
var offerTransitions = map[OfferStatus][]OfferStatus{
	OfferStatusSent: {OfferStatusRejected, OfferStatusExpired},
}

func (s OfferStatus) CanTransitionTo(next OfferStatus) bool {
	for _, allowed := range offerTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

func IsValidOfferStatus(value string) bool {
	switch OfferStatus(value) {
	case OfferStatusDraft, OfferStatusSent, OfferStatusAccepted, OfferStatusRejected, OfferStatusExpired:
		return true
	}
	return false
}

func IsValidOfferItemKind(value string) bool {
	switch OfferItemKind(value) {
	case OfferItemKindPart, OfferItemKindLabor, OfferItemKindOther:
		return true
	}
	return false
}
