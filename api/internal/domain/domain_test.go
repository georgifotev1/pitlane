package domain

import "testing"

func TestTaxCents(t *testing.T) {
	tests := []struct {
		name  string
		total int64
		bps   int
		want  int64
	}{
		{"zero total", 0, 2000, 0},
		{"zero rate", 10000, 0, 0},
		{"20% included in 120.00", 12000, 2000, 2000},
		{"20% included in 30.00", 3000, 2000, 500},
		{"19% included in 150.00", 15000, 1900, 2395},
		{"included tax rounds half up", 3, 2000, 1},
		{"included tax rounds down", 2, 2000, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TaxCents(tt.total, tt.bps); got != tt.want {
				t.Fatalf("TaxCents(%d, %d) = %d, want %d", tt.total, tt.bps, got, tt.want)
			}
		})
	}
}

func TestOfferRecompute(t *testing.T) {
	o := &Offer{
		TaxRateBps: 1900,
		Items: []OfferItem{
			{UnitPriceCents: 2500, Quantity: 3},
			{UnitPriceCents: 999, Quantity: 1},
			{UnitPriceCents: 500, Quantity: 2},
		},
	}
	o.Recompute()

	wantLines := []int64{7500, 999, 1000}
	for i, want := range wantLines {
		if o.Items[i].LineTotalCents != want {
			t.Fatalf("item[%d] line total = %d, want %d", i, o.Items[i].LineTotalCents, want)
		}
	}
	if o.SubtotalCents != 7982 {
		t.Fatalf("subtotal = %d, want 7982", o.SubtotalCents)
	}
	if o.TaxCents != 1517 {
		t.Fatalf("tax = %d, want 1517", o.TaxCents)
	}
	if o.TotalCents != 9499 {
		t.Fatalf("total = %d, want 9499", o.TotalCents)
	}
}

func TestOfferRecomputeEmpty(t *testing.T) {
	o := &Offer{TaxRateBps: 1900}
	o.Recompute()
	if o.SubtotalCents != 0 || o.TaxCents != 0 || o.TotalCents != 0 {
		t.Fatalf("empty offer should total zero, got %+v", o)
	}
}

func TestOfferStatusTransitions(t *testing.T) {
	allow := []struct{ from, to OfferStatus }{
		{OfferStatusSent, OfferStatusRejected},
		{OfferStatusSent, OfferStatusExpired},
	}
	for _, tc := range allow {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Errorf("expected %s → %s allowed", tc.from, tc.to)
		}
	}

	deny := []struct{ from, to OfferStatus }{
		{OfferStatusDraft, OfferStatusSent},
		{OfferStatusSent, OfferStatusAccepted},
		{OfferStatusDraft, OfferStatusAccepted},
		{OfferStatusDraft, OfferStatusDraft},
		{OfferStatusSent, OfferStatusDraft},
		{OfferStatusAccepted, OfferStatusSent},
		{OfferStatusRejected, OfferStatusSent},
		{OfferStatusExpired, OfferStatusSent},
	}
	for _, tc := range deny {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("expected %s → %s denied", tc.from, tc.to)
		}
	}
}

func TestRepairRecompute(t *testing.T) {
	r := &Repair{
		TaxRateBps: 1900,
		Items: []RepairItem{
			{Quantity: 2, UnitPriceCents: 4500},
			{Quantity: 1, UnitPriceCents: 6000},
		},
	}
	r.Recompute()
	if r.SubtotalCents != 12605 || r.TaxCents != 2395 || r.TotalCents != 15000 {
		t.Fatalf("totals wrong: sub=%d tax=%d total=%d", r.SubtotalCents, r.TaxCents, r.TotalCents)
	}
	if r.Items[0].LineTotalCents != 9000 || r.Items[1].LineTotalCents != 6000 {
		t.Fatalf("line totals wrong: %+v", r.Items)
	}
}

func TestRepairStatusTransitions(t *testing.T) {
	allow := []struct{ from, to RepairStatus }{
		{RepairStatusOpen, RepairStatusInProgress},
		{RepairStatusInProgress, RepairStatusOpen},
	}
	for _, tc := range allow {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Errorf("expected %s → %s allowed", tc.from, tc.to)
		}
	}

	deny := []struct{ from, to RepairStatus }{
		{RepairStatusOpen, RepairStatusCompleted},
		{RepairStatusInProgress, RepairStatusCompleted},
		{RepairStatusOpen, RepairStatusOpen},
		{RepairStatusCompleted, RepairStatusOpen},
		{RepairStatusCompleted, RepairStatusInProgress},
	}
	for _, tc := range deny {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("expected %s → %s denied", tc.from, tc.to)
		}
	}
}

func TestRecomputeTracksSupplierCostAndProfit(t *testing.T) {
	// A part bought for 40,00 and sold for 60,00; labour at 50,00 with no cost.
	o := &Offer{
		TaxRateBps: StandardVATRateBPS,
		Items: []OfferItem{
			{Kind: OfferItemKindPart, Quantity: 2, UnitPriceCents: 6000, CostCents: 4000},
			{Kind: OfferItemKindLabor, Quantity: 1, UnitPriceCents: 5000},
		},
	}
	o.Recompute()

	if o.TotalCents != 17000 {
		t.Fatalf("total = %d; want 17000", o.TotalCents)
	}
	if o.Items[0].LineCostCents != 8000 {
		t.Fatalf("line cost = %d; want 8000", o.Items[0].LineCostCents)
	}
	if o.CostTotalCents != 8000 {
		t.Fatalf("cost total = %d; want 8000", o.CostTotalCents)
	}
	// Both sides are compared net of VAT: 17000 and 8000 inclusive of 20% are
	// 14167 and 6667 net, leaving 7500.
	if o.CostSubtotalCents != 6667 {
		t.Fatalf("net cost = %d; want 6667", o.CostSubtotalCents)
	}
	if got, want := o.ProfitCents(), o.SubtotalCents-o.CostSubtotalCents; got != want {
		t.Fatalf("profit = %d; want %d", got, want)
	}
	// 7500 kept on 14167 of net revenue: 52.93%.
	if got := o.MarginBps(); got != 5293 {
		t.Fatalf("margin = %d bps; want 5293", got)
	}
}

func TestRecomputeWithoutCostReportsNoSpend(t *testing.T) {
	r := &Repair{
		TaxRateBps: StandardVATRateBPS,
		Items:      []RepairItem{{Kind: OfferItemKindLabor, Quantity: 1, UnitPriceCents: 12000}},
	}
	r.Recompute()

	if r.CostTotalCents != 0 || r.CostSubtotalCents != 0 {
		t.Fatalf("cost = %d/%d; want 0/0", r.CostTotalCents, r.CostSubtotalCents)
	}
	if r.ProfitCents() != r.SubtotalCents {
		t.Fatalf("profit = %d; want the full net revenue %d", r.ProfitCents(), r.SubtotalCents)
	}
}

func TestNetCentsIsTheComplementOfTaxCents(t *testing.T) {
	for _, gross := range []int64{0, 1, 99, 12345, 999999} {
		if got := NetCents(gross, StandardVATRateBPS) + TaxCents(gross, StandardVATRateBPS); got != gross {
			t.Fatalf("net+tax = %d; want %d", got, gross)
		}
	}
}
