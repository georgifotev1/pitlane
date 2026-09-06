package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/store"
)

func costedOffer() *domain.Offer {
	o := &domain.Offer{
		ID:             "offer",
		DocumentNumber: "OF-2026-000001",
		CarID:          "car",
		TaxRateBps:     domain.StandardVATRateBPS,
		Items: []domain.OfferItem{
			{Kind: domain.OfferItemKindPart, Description: "Накладки", Quantity: 2, UnitPriceCents: 6000, CostCents: 2345},
			{Kind: domain.OfferItemKindLabor, Description: "Монтаж", Quantity: 1, UnitPriceCents: 5000},
		},
	}
	o.Recompute()
	return o
}

// The garage's buying price is the one number the customer must never see, so
// the templates they can receive are checked for it directly.
func TestCustomerFacingTemplatesNeverShowSupplierCost(t *testing.T) {
	cache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}

	tenant := &domain.Tenant{Name: "Гараж", Currency: "EUR"}
	customer := &domain.Customer{ID: "customer", Name: "Клиент"}
	car := &domain.Car{ID: "car", CustomerID: customer.ID, Plate: "CA1234AB"}
	offer := costedOffer()

	data := &templateData{
		Form: forms.New(nil), Tenant: tenant, Customer: customer, Car: car, Offer: offer,
		Document: offerDocument(offer, tenant.Currency),
	}

	var output bytes.Buffer
	if err := cache["offer-print.page.html"].ExecuteTemplate(&output, "base", data); err != nil {
		t.Fatal(err)
	}
	sheet := output.String()
	for _, forbidden := range []string{"23,45", "46,90", "Доставна", "печалба", "Печалба", "марж"} {
		if strings.Contains(sheet, forbidden) {
			t.Errorf("print sheet leaks internal cost data: contains %q", forbidden)
		}
	}
	if !strings.Contains(sheet, "120,00") {
		t.Error("print sheet is missing the line total the customer pays")
	}
}

func TestOfferPageShowsMarginToTheOwner(t *testing.T) {
	cache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}

	tenant := &domain.Tenant{Name: "Гараж", Currency: "EUR"}
	customer := &domain.Customer{ID: "customer", Name: "Клиент"}
	car := &domain.Car{ID: "car", CustomerID: customer.ID, Plate: "CA1234AB"}
	offer := costedOffer()

	data := &templateData{
		Form: forms.New(nil), Tenant: tenant, Customer: customer, Car: car, Offer: offer,
		Document: offerDocument(offer, tenant.Currency),
		Margin:   offerMargin(offer, tenant.Currency),
	}

	var output bytes.Buffer
	if err := cache["offer.page.html"].ExecuteTemplate(&output, "base", data); err != nil {
		t.Fatal(err)
	}
	page := output.String()
	if !strings.Contains(page, "Вашата печалба") {
		t.Error("offer page is missing the margin panel")
	}
	if !strings.Contains(page, "39,08") {
		t.Errorf("offer page is missing the net supplier cost: %s", page)
	}
}

// Without a supplier price, profit would equal the whole invoice. The boards say
// so with a dash instead of reporting a 100% margin.
func TestBoardHidesMarginWhenNoCostRecorded(t *testing.T) {
	offer := &domain.Offer{ID: "offer", DocumentNumber: "OF-1", TaxRateBps: domain.StandardVATRateBPS,
		Items: []domain.OfferItem{{Kind: domain.OfferItemKindLabor, Description: "Труд", Quantity: 1, UnitPriceCents: 12000}}}
	offer.Recompute()

	board := offerBoard([]store.OfferSummary{{Offer: offer, CarPlate: "CA1234AB", CustomerName: "Клиент"}}, "EUR")
	if board.Rows[0].HasCost {
		t.Fatal("row reports a cost that was never entered")
	}

	costed := costedOffer()
	board = offerBoard([]store.OfferSummary{{Offer: costed, CarPlate: "CA1234AB", CustomerName: "Клиент"}}, "EUR")
	if !board.Rows[0].HasCost {
		t.Fatal("row hides a cost that was entered")
	}
	if board.Rows[0].ProfitCents != costed.ProfitCents() {
		t.Fatalf("profit = %d; want %d", board.Rows[0].ProfitCents, costed.ProfitCents())
	}
}

func TestReadOfferFormParsesSupplierCost(t *testing.T) {
	form, offer := postOfferForm(t, map[string][]string{
		"item_kind":        {"part", "labor"},
		"item_description": {"Накладки", "Монтаж"},
		"item_quantity":    {"2", "1"},
		"item_price":       {"60,00", "50,00"},
		"item_cost":        {"23,45", ""},
	})
	if !form.Valid() {
		t.Fatalf("form errors: %v", form.Errors)
	}
	if offer.Items[0].CostCents != 2345 {
		t.Fatalf("part cost = %d; want 2345", offer.Items[0].CostCents)
	}
	if offer.Items[1].CostCents != 0 {
		t.Fatalf("labour cost = %d; want 0 for an empty field", offer.Items[1].CostCents)
	}
	// The store recomputes the money snapshots on write; do the same here to
	// check the per-line costs roll up.
	offer.Recompute()
	if offer.CostTotalCents != 4690 {
		t.Fatalf("offer cost total = %d; want 4690", offer.CostTotalCents)
	}
}

func TestReadOfferFormRejectsInvalidSupplierCost(t *testing.T) {
	form, _ := postOfferForm(t, map[string][]string{
		"item_kind":        {"part"},
		"item_description": {"Накладки"},
		"item_quantity":    {"1"},
		"item_price":       {"60,00"},
		"item_cost":        {"дузина"},
	})
	if form.Valid() {
		t.Fatal("form accepted a supplier cost that is not a number")
	}
}

func postOfferForm(t *testing.T, values map[string][]string) (*forms.Form, *domain.Offer) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/cars/car/offers/new", strings.NewReader(url.Values(values).Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	form, offer, ok := (&application{}).readOfferForm(httptest.NewRecorder(), r)
	if !ok {
		t.Fatal("readOfferForm rejected the request")
	}
	return form, offer
}
