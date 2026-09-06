package domain

import "time"

type RepairStatus string

const (
	RepairStatusOpen       RepairStatus = "open"
	RepairStatusInProgress RepairStatus = "in_progress"
	RepairStatusCompleted  RepairStatus = "completed"
)

type Repair struct {
	ID             string
	DocumentNumber string
	TenantID       string
	CarID          string
	OfferID        *string
	Status         RepairStatus
	TaxRateBps     int
	SubtotalCents  int64
	TaxCents       int64
	TotalCents     int64
	// Internal margin snapshots copied from the offer and editable while the
	// repair is open. See Offer for the VAT convention.
	CostTotalCents    int64
	CostSubtotalCents int64
	Mileage           int
	Notes             string
	Items             []RepairItem
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RepairItem struct {
	ID             string
	TenantID       string
	RepairID       string
	Kind           OfferItemKind
	Description    string
	Quantity       int
	UnitPriceCents int64
	LineTotalCents int64
	CostCents      int64
	LineCostCents  int64
	SortOrder      int
	CreatedAt      time.Time
}

func (r *Repair) Recompute() {
	var total, cost int64
	for i := range r.Items {
		line := r.Items[i].UnitPriceCents * int64(r.Items[i].Quantity)
		lineCost := r.Items[i].CostCents * int64(r.Items[i].Quantity)
		r.Items[i].LineTotalCents = line
		r.Items[i].LineCostCents = lineCost
		total += line
		cost += lineCost
	}
	r.TotalCents = total
	r.TaxCents = TaxCents(total, r.TaxRateBps)
	r.SubtotalCents = total - r.TaxCents
	r.CostTotalCents = cost
	r.CostSubtotalCents = NetCents(cost, r.TaxRateBps)
}

// ProfitCents mirrors Offer.ProfitCents: net revenue less net supplier cost.
func (r *Repair) ProfitCents() int64 { return r.SubtotalCents - r.CostSubtotalCents }

func (r *Repair) MarginBps() int { return MarginBps(r.ProfitCents(), r.SubtotalCents) }

// Completion uses a dedicated path that also records mileage.
var repairTransitions = map[RepairStatus][]RepairStatus{
	RepairStatusOpen:       {RepairStatusInProgress},
	RepairStatusInProgress: {RepairStatusOpen},
}

func (s RepairStatus) CanTransitionTo(next RepairStatus) bool {
	for _, allowed := range repairTransitions[s] {
		if allowed == next {
			return true
		}
	}
	return false
}

func IsValidRepairStatus(value string) bool {
	switch RepairStatus(value) {
	case RepairStatusOpen, RepairStatusInProgress, RepairStatusCompleted:
		return true
	}
	return false
}
