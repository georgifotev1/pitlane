package domain

import "testing"

func TestTaxCents(t *testing.T) {
	tests := []struct {
		name     string
		subtotal int64
		bps      int
		want     int64
	}{
		{"zero subtotal", 0, 1900, 0},
		{"zero rate", 10000, 0, 0},
		{"exact 19% of 100.00", 10000, 1900, 1900},
		{"exact 20% of 50.00", 5000, 2000, 1000},
		// 19% of 1.00 = 0.19 exactly.
		{"small exact", 100, 1900, 19},
		// 19% of 1.05 = 0.1995 → rounds half up to 0.20 (20 cents).
		{"rounds half up", 105, 1900, 20},
		// 19% of 0.50 = 0.095 → 0.10? 50*1900=95000, +5000=100000, /10000=10.
		{"half rounds up to 10", 50, 1900, 10},
		// 7.5% of 133 cents = 9.975 → 10. 133*750=99750,+5000=104750,/10000=10.
		{"fractional bps rounds up", 133, 750, 10},
		// 19% of 3.33 = 0.6327 → 0.63. 333*1900=632700,+5000=637700,/10000=63.
		{"rounds down", 333, 1900, 63},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TaxCents(tt.subtotal, tt.bps); got != tt.want {
				t.Fatalf("TaxCents(%d, %d) = %d, want %d", tt.subtotal, tt.bps, got, tt.want)
			}
		})
	}
}

func TestOfferRecompute(t *testing.T) {
	o := &Offer{
		TaxRateBps: 1900,
		Items: []OfferItem{
			{UnitPriceCents: 2500, Quantity: 3}, // 75.00
			{UnitPriceCents: 999, Quantity: 1},  // 9.99
			{UnitPriceCents: 500, Quantity: 2},  // 10.00
		},
	}
	o.Recompute()

	wantLines := []int64{7500, 999, 1000}
	for i, want := range wantLines {
		if o.Items[i].LineTotalCents != want {
			t.Fatalf("item[%d] line total = %d, want %d", i, o.Items[i].LineTotalCents, want)
		}
	}
	// subtotal = 94.99; tax 19% = 18.0481 → 1805; total = 113.04.
	if o.SubtotalCents != 9499 {
		t.Fatalf("subtotal = %d, want 9499", o.SubtotalCents)
	}
	if o.TaxCents != 1805 {
		t.Fatalf("tax = %d, want 1805", o.TaxCents)
	}
	if o.TotalCents != 11304 {
		t.Fatalf("total = %d, want 11304", o.TotalCents)
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
		{OfferStatusDraft, OfferStatusSent},
		{OfferStatusSent, OfferStatusAccepted},
		{OfferStatusSent, OfferStatusRejected},
		{OfferStatusSent, OfferStatusExpired},
	}
	for _, tc := range allow {
		if !tc.from.CanTransitionTo(tc.to) {
			t.Errorf("expected %s → %s allowed", tc.from, tc.to)
		}
	}

	deny := []struct{ from, to OfferStatus }{
		{OfferStatusDraft, OfferStatusAccepted}, // must be sent first
		{OfferStatusDraft, OfferStatusDraft},    // no-op is not a transition
		{OfferStatusSent, OfferStatusDraft},     // cannot un-send
		{OfferStatusAccepted, OfferStatusSent},  // terminal
		{OfferStatusRejected, OfferStatusSent},  // terminal
		{OfferStatusExpired, OfferStatusSent},   // terminal
	}
	for _, tc := range deny {
		if tc.from.CanTransitionTo(tc.to) {
			t.Errorf("expected %s → %s denied", tc.from, tc.to)
		}
	}
}
