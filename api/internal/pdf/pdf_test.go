package pdf

import (
	"bytes"
	"testing"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
)

// sampleData builds a fully-populated offer with Cyrillic content so the render
// exercises the embedded font as it would in production.
func sampleData() OfferData {
	created := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	return OfferData{
		Tenant: &domain.Tenant{
			Name:      "Автосервиз Пъклен",
			Address:   "ул. Витоша 15, София",
			VATNumber: "BG123456789",
			Currency:  "EUR",
		},
		Customer: &domain.Customer{
			Name:    "Иван Петров",
			Company: "Петров ЕООД",
			Email:   "ivan@example.com",
			Phone:   "+359 88 123 4567",
			Address: "бул. България 100, Пловдив",
		},
		Car: &domain.Car{
			Plate:   "CB1234AB",
			VIN:     "WVWZZZ1JZXW000001",
			Make:    "Volkswagen",
			Model:   "Golf",
			Year:    2018,
			Mileage: 145000,
		},
		Offer: &domain.Offer{
			ID:            "11112222-3333-4444-5555-666677778888",
			Status:        domain.OfferStatusSent,
			TaxRateBps:    1900,
			SubtotalCents: 15000,
			TaxCents:      2850,
			TotalCents:    17850,
			Notes:         "Офертата е валидна 30 дни. Цените са с включено ДДС.",
			CreatedAt:     created,
			Items: []domain.OfferItem{
				{Kind: domain.OfferItemKindPart, Description: "Накладки за спирачки", Quantity: 2, UnitPriceCents: 4500, LineTotalCents: 9000},
				{Kind: domain.OfferItemKindLabor, Description: "Монтаж", Quantity: 1, UnitPriceCents: 6000, LineTotalCents: 6000},
			},
		},
	}
}

func TestRenderOffer(t *testing.T) {
	r := NewRenderer()

	b, err := r.RenderOffer(sampleData())
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("empty pdf")
	}
	// The PDF magic number proves we produced a real document, not stray bytes.
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("not a pdf: first bytes %q", b[:min(8, len(b))])
	}
	// %%EOF trailer proves the document was closed, not truncated mid-stream.
	if !bytes.Contains(b, []byte("%%EOF")) {
		t.Fatal("pdf missing EOF trailer")
	}
}

// TestRenderOfferMinimal proves the renderer tolerates an offer with only the
// required fields — no notes, no optional customer/car fields — so a bare draft
// still produces a valid document (empty cells, no panic).
func TestRenderOfferMinimal(t *testing.T) {
	r := NewRenderer()
	d := OfferData{
		Tenant:   &domain.Tenant{Name: "Гараж", Currency: "EUR"},
		Customer: &domain.Customer{Name: "Клиент"},
		Car:      &domain.Car{Plate: "X1"},
		Offer: &domain.Offer{
			ID:         "abcdef01-0000-0000-0000-000000000000",
			TaxRateBps: 0,
			CreatedAt:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Items: []domain.OfferItem{
				{Kind: domain.OfferItemKindOther, Description: "Диагностика", Quantity: 1, UnitPriceCents: 3000, LineTotalCents: 3000},
			},
			SubtotalCents: 3000,
			TotalCents:    3000,
		},
	}
	b, err := r.RenderOffer(d)
	if err != nil {
		t.Fatalf("render minimal: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("minimal render not a pdf")
	}
}

func TestFormatCents(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0,00"},
		{5, "0,05"},
		{100, "1,00"},
		{15000, "150,00"},
		{123456, "1 234,56"},
		{100000000, "1 000 000,00"},
		{-4500, "-45,00"},
	}
	for _, c := range cases {
		if got := formatCents(c.in); got != c.want {
			t.Errorf("formatCents(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMoney(t *testing.T) {
	cases := []struct {
		cents    int64
		currency string
		want     string
	}{
		// Bulgaria is on the euro: EUR and an unset currency both render "€".
		{15000, "EUR", "150,00 €"},
		{123456, "", "1 234,56 €"},
		// A foreign currency keeps its ISO code; the lev is gone entirely.
		{15000, "USD", "150,00 USD"},
	}
	for _, c := range cases {
		if got := money(c.cents, c.currency); got != c.want {
			t.Errorf("money(%d, %q) = %q, want %q", c.cents, c.currency, got, c.want)
		}
	}
}

func TestPercent(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0%"},
		{1900, "19%"},
		{1950, "19,50%"},
		{10000, "100%"},
	}
	for _, c := range cases {
		if got := percent(c.in); got != c.want {
			t.Errorf("percent(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
