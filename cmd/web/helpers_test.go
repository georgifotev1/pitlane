package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/store"
)

func TestTemplateCache(t *testing.T) {
	cache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}
	if len(cache) != 22 {
		t.Fatalf("got %d templates; want 22", len(cache))
	}

	tenant := &domain.Tenant{Currency: "EUR", Name: "Garage"}
	customer := &domain.Customer{ID: "customer", Name: "Customer"}
	car := &domain.Car{ID: "car", CustomerID: customer.ID, Plate: "CA1234AB"}
	offer := &domain.Offer{ID: "offer", DocumentNumber: "OF-2026-000001", CarID: car.ID, Items: []domain.OfferItem{{Kind: domain.OfferItemKindPart, Description: "Oil", Quantity: 1, UnitPriceCents: 1000, LineTotalCents: 1000}}}
	repair := &domain.Repair{ID: "repair", DocumentNumber: "RP-2026-000001", CarID: car.ID, Status: domain.RepairStatusOpen, Items: []domain.RepairItem{{Kind: domain.OfferItemKindLabor, Description: "Work", Quantity: 1, UnitPriceCents: 2000, LineTotalCents: 2000}}}
	data := &templateData{
		CurrentUser: "Owner", GarageName: tenant.Name, Form: forms.New(nil), Tenant: tenant,
		User: &domain.User{Email: "owner@example.com"}, NameForm: forms.New(nil), PasswordForm: forms.New(nil),
		Customer: customer, Customers: []*domain.Customer{customer}, Car: car,
		Cars: []store.CarSummary{{Car: car, CustomerName: customer.Name}}, CustomerCars: []*domain.Car{car},
		Note: &domain.HistoryNote{ID: "note", CarID: car.ID}, Offer: offer,
		Offers:    []store.OfferSummary{{Offer: offer, CarPlate: car.Plate, CustomerName: customer.Name}},
		CarOffers: []*domain.Offer{offer}, Repair: repair,
		Repairs: []store.RepairSummary{{Repair: repair, CarPlate: car.Plate, CustomerName: customer.Name}},
		Editor:  newItemEditor(offerEditorTitle, offerEditorHint, blankOfferRows(8), nil),
	}
	offerSummaries := []store.OfferSummary{{Offer: offer, CarPlate: car.Plate, CustomerName: customer.Name}}
	repairSummaries := []store.RepairSummary{{Repair: repair, CarPlate: car.Plate, CustomerName: customer.Name}}
	data.Board = offerBoard(offerSummaries, tenant.Currency)
	data.Document = offerDocument(offer, tenant.Currency)
	data.Margin = offerMargin(offer, tenant.Currency)
	data.Stats = dashboardTiles(store.Dashboard{}, tenant.Currency)
	data.Revenue = newRevenueChart(nil, tenant.Currency)
	data.Split = newSplitChart(nil, tenant.Currency)
	data.RecentOffers = offerBoard(offerSummaries, tenant.Currency)
	data.RecentRepairs = repairBoard(repairSummaries, tenant.Currency)
	data.Attention = attentionBoard(store.Dashboard{}, tenant.Currency, time.Now())
	for name, tmpl := range cache {
		t.Run(name, func(t *testing.T) {
			if err := tmpl.ExecuteTemplate(io.Discard, "base", data); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestEditTemplatesPrepopulateCurrentValues(t *testing.T) {
	cache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}

	form := forms.New(nil)
	values := map[string]string{
		"name": "Ivan", "company": "Garage Ltd", "email": "ivan@example.com", "phone": "0888123456",
		"address": "Sofia", "notes": "Current notes", "plate": "CA1234AB", "vin": "VIN123",
		"make": "Volvo", "model": "V60", "year": "2020", "mileage": "123456",
		"title": "Oil service", "recorded_at": "2026-03-01", "description": "Changed oil",
	}
	for key, value := range values {
		form.Values.Set(key, value)
	}

	tenant := &domain.Tenant{Name: "Garage", Currency: "EUR"}
	customer := &domain.Customer{ID: "customer", Name: "Customer"}
	car := &domain.Car{ID: "car", CustomerID: customer.ID, Plate: "CA1234AB"}
	offer := &domain.Offer{ID: "offer", DocumentNumber: "OF-2026-000001", CarID: car.ID}
	repair := &domain.Repair{ID: "repair", DocumentNumber: "RP-2026-000001", CarID: car.ID, Status: domain.RepairStatusOpen}
	note := &domain.HistoryNote{ID: "note", CarID: car.ID}

	tests := []struct {
		page string
		data *templateData
		want []string
	}{
		{"customer-form.page.html", &templateData{Form: form, Customer: customer}, []string{`value="Ivan"`, `value="Garage Ltd"`, `value="ivan@example.com"`, `value="0888123456"`, `>Sofia</textarea>`, `>Current notes</textarea>`}},
		{"car-form.page.html", &templateData{Form: form, Customer: customer, Car: car, CarMakeSuggestions: []string{"Volvo", "Volkswagen"}, CarModelSuggestions: []carModelSuggestion{{Make: "Volvo", Model: "V60"}, {Make: "Volkswagen", Model: "Golf"}}}, []string{`value="CA1234AB"`, `value="VIN123"`, `value="Volvo"`, `value="V60"`, `value="2020"`, `value="123456"`, `list="car-makes"`, `list="car-models"`, `<option value="Volkswagen" label="VW">`, `<option value="Golf" data-make="Volkswagen">`}},
		{"history-form.page.html", &templateData{Form: form, Car: car, Note: note}, []string{`value="Oil service"`, `value="2026-03-01"`, `>Changed oil</textarea>`}},
		{"offer-form.page.html", &templateData{Form: form, Tenant: tenant, Customer: customer, Car: car, Offer: offer, Editor: newItemEditor(offerEditorTitle, offerEditorHint, blankOfferRows(1), form)}, []string{`>Current notes</textarea>`, `name="item_cost"`}},
		{"repair-form.page.html", &templateData{Form: form, Tenant: tenant, Customer: customer, Car: car, Repair: repair, Editor: newItemEditor(repairEditorTitle, repairEditorHint, blankOfferRows(1), form)}, []string{`value="123456"`, `>Current notes</textarea>`, `name="item_cost"`}},
	}
	for _, tt := range tests {
		t.Run(tt.page, func(t *testing.T) {
			var output bytes.Buffer
			if err := cache[tt.page].ExecuteTemplate(&output, "base", tt.data); err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("rendered form does not contain %q", want)
				}
			}
		})
	}
}

func TestMergeSuggestionsPreservesPriorityAndRemovesDuplicates(t *testing.T) {
	got := mergeSuggestions(
		[]string{"Volkswagen", "BMW"},
		[]string{" volkswagen ", "Zastava", "", "bmw"},
	)
	want := []string{"Volkswagen", "BMW", "Zastava"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCarCatalogueAndCanonicalMake(t *testing.T) {
	modelsByMake := make(map[string]int)
	for _, suggestion := range commonCarModels {
		modelsByMake[suggestion.Make]++
	}
	for _, makeName := range commonCarMakes {
		if modelsByMake[makeName] == 0 {
			t.Errorf("%s has no model suggestions", makeName)
		}
	}
	if got := canonicalCarMake("vw"); got != "Volkswagen" {
		t.Fatalf("canonicalCarMake(vw) = %q, want Volkswagen", got)
	}
	if got := canonicalCarMake("bmw"); got != "BMW" {
		t.Fatalf("canonicalCarMake(bmw) = %q, want BMW", got)
	}

	merged := mergeModelSuggestions(
		[]carModelSuggestion{{Make: "Volkswagen", Model: "Golf"}},
		[]store.CarModelSuggestion{{Make: "VW", Model: "golf"}, {Make: "VW", Model: "up!"}},
	)
	if len(merged) != 2 || merged[1] != (carModelSuggestion{Make: "Volkswagen", Model: "up!"}) {
		t.Fatalf("merged Volkswagen models: got %v", merged)
	}
}

func TestSidebarHighlightsCurrentSection(t *testing.T) {
	cache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path string
		href string
	}{
		{"/dashboard", "/dashboard"},
		{"/customers/customer-id", "/customers"},
		{"/cars/car-id", "/cars"},
		{"/offers/offer-id/edit", "/offers"},
		{"/repairs/repair-id", "/repairs"},
		{"/garage", "/garage"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			var output bytes.Buffer
			data := &templateData{CurrentPath: tt.path, CurrentUser: "Owner", Form: forms.New(nil)}
			if err := cache["garage.page.html"].ExecuteTemplate(&output, "base", data); err != nil {
				t.Fatal(err)
			}

			html := output.String()
			want := `class="active" href="` + tt.href + `"`
			if !strings.Contains(html, want) {
				t.Errorf("sidebar does not highlight %s", tt.href)
			}
			if count := strings.Count(html, `class="active"`); count != 1 {
				t.Errorf("got %d active sidebar links; want 1", count)
			}
		})
	}
}

func TestPagination(t *testing.T) {
	t.Run("parses safe page numbers", func(t *testing.T) {
		tests := []struct {
			query string
			want  int
		}{
			{"", 1},
			{"?page=2", 2},
			{"?page=0", 1},
			{"?page=-1", 1},
			{"?page=nope", 1},
			{"?page=999999999999999999999999999999", 1},
		}
		for _, tt := range tests {
			r := httptest.NewRequest(http.MethodGet, "/customers"+tt.query, nil)
			if got := requestedPage(r); got != tt.want {
				t.Errorf("requestedPage(%q) = %d; want %d", tt.query, got, tt.want)
			}
		}
	})

	t.Run("builds ranges and preserves filters", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/customers?q=ivan&archived=1&page=2", nil)
		got := newPagination(r, 2, 61)
		if got.FirstItem != 26 || got.LastItem != 50 || got.TotalPages != 3 {
			t.Fatalf("pagination range = %d-%d of %d pages; want 26-50 of 3", got.FirstItem, got.LastItem, got.TotalPages)
		}
		if got.PreviousURL != "/customers?archived=1&q=ivan" {
			t.Errorf("previous URL = %q", got.PreviousURL)
		}
		if got.NextURL != "/customers?archived=1&page=3&q=ivan" {
			t.Errorf("next URL = %q", got.NextURL)
		}
	})

	t.Run("redirects stale pages to the last page", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/offers?status=draft&page=8", nil)
		w := httptest.NewRecorder()
		if !redirectIfPageOutOfRange(w, r, 8, 40) {
			t.Fatal("expected an out-of-range redirect")
		}
		if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/offers?page=2&status=draft" {
			t.Fatalf("got status %d location %q", w.Code, w.Header().Get("Location"))
		}
	})
}

func TestParseCents(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{"12", 1200, true},
		{"12.3", 1230, true},
		{"1 234,56", 123456, true},
		{"0.00", 0, true},
		{"-1", 0, false},
		{"1.234", 0, false},
		{"nope", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := parseCents(tt.input)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("parseCents(%q) = (%d, %v); want (%d, %v)", tt.input, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestReadOfferFormUsesFixedVATAndInclusivePrices(t *testing.T) {
	values := url.Values{
		"tax_rate":         {"0"},
		"item_kind":        {"part"},
		"item_description": {"Масло"},
		"item_quantity":    {"2"},
		"item_price":       {"24,00"},
	}
	r := httptest.NewRequest(http.MethodPost, "/cars/car/offers/new", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	form, offer, ok := (&application{}).readOfferForm(httptest.NewRecorder(), r)
	if !ok || !form.Valid() {
		t.Fatalf("readOfferForm failed: ok=%v errors=%v", ok, form.Errors)
	}
	if offer.TaxRateBps != domain.StandardVATRateBPS {
		t.Fatalf("tax rate = %d; want fixed %d", offer.TaxRateBps, domain.StandardVATRateBPS)
	}
	if len(offer.Items) != 1 || offer.Items[0].UnitPriceCents != 2400 {
		t.Fatalf("items = %+v", offer.Items)
	}
}

func TestReadRepairFormIncludesMileage(t *testing.T) {
	values := url.Values{
		"mileage":          {"123456"},
		"item_kind":        {"labor"},
		"item_description": {"Диагностика"},
		"item_quantity":    {"1"},
		"item_price":       {"50,00"},
	}
	r := httptest.NewRequest(http.MethodPost, "/repairs/repair/edit", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	form, repair, ok := (&application{}).readRepairForm(httptest.NewRecorder(), r)
	if !ok || !form.Valid() {
		t.Fatalf("readRepairForm failed: ok=%v errors=%v", ok, form.Errors)
	}
	if repair.Mileage != 123456 {
		t.Fatalf("mileage = %d; want 123456", repair.Mileage)
	}
}

func TestOfferRowsDoNotAddPlaceholders(t *testing.T) {
	if rows := offerRowsFromForm(forms.New(nil)); len(rows) != 1 {
		t.Fatalf("blank form has %d rows; want 1", len(rows))
	}
	offer := &domain.Offer{Items: []domain.OfferItem{{}, {}}}
	if rows := offerRowsFor(offer); len(rows) != 2 {
		t.Fatalf("two offer items produced %d rows; want 2", len(rows))
	}
}

func TestSafeRedirect(t *testing.T) {
	for _, target := range []string{"/dashboard", "/cars/one?q=a"} {
		if !safeRedirect(target) {
			t.Errorf("expected %q to be safe", target)
		}
	}
	for _, target := range []string{"https://example.com", "//example.com", "dashboard"} {
		if safeRedirect(target) {
			t.Errorf("expected %q to be unsafe", target)
		}
	}
}
