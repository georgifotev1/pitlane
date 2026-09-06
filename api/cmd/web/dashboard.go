package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
)

// statTile is one headline number. A single figure is not a chart: it gets a
// tile with a label, the value, one line of context and, where the comparison
// means something, the change against the month before.
type statTile struct {
	Label   string
	Value   string
	Sub     string
	Delta   string
	Rising  bool
	Falling bool
	URL     string
}

func (app *application) dashboard(w http.ResponseWriter, r *http.Request) {
	tenantID := app.tenantID(r)
	now := time.Now()

	data := app.newTemplateData(r)
	currency := ""
	if data.Tenant != nil {
		currency = data.Tenant.Currency
	}

	var (
		dash          store.Dashboard
		customers     []*domain.Customer
		customerTotal int
		carTotal      int
		offers        []store.OfferSummary
		offerTotal    int
		repairs       []store.RepairSummary
	)
	if err := runConcurrently(r.Context(),
		func(ctx context.Context) error {
			var err error
			dash, err = app.dashboards.Load(ctx, tenantID, now)
			return err
		},
		func(ctx context.Context) error {
			var err error
			customers, customerTotal, err = app.customers.List(ctx, tenantID, store.CustomerListParams{Limit: 5})
			return err
		},
		func(ctx context.Context) error {
			var err error
			_, carTotal, err = app.cars.ListAll(ctx, tenantID, store.CarListParams{Limit: 1})
			return err
		},
		func(ctx context.Context) error {
			var err error
			offers, offerTotal, err = app.offers.ListAll(ctx, tenantID, store.OfferBoardParams{Limit: 5})
			return err
		},
		func(ctx context.Context) error {
			var err error
			repairs, _, err = app.repairs.List(ctx, tenantID, store.RepairListParams{Limit: 5})
			return err
		},
	); err != nil {
		app.serverError(w, r, err)
		return
	}

	data.Customers = customers
	data.Dashboard = dash
	data.Stats = dashboardTiles(dash, currency)
	data.Revenue = newRevenueChart(dash.Months, currency)
	data.Split = newSplitChart(dash.Split, currency)
	data.RecentRepairs = repairBoard(repairs, currency)
	data.RecentOffers = offerBoard(offers, currency)
	data.Attention = attentionBoard(dash, currency, now)
	data.Form.Values.Set("customer_total", stringInt(customerTotal))
	data.Form.Values.Set("car_total", stringInt(carTotal))
	data.Form.Values.Set("offer_total", stringInt(offerTotal))
	app.render(w, r, "dashboard.page.html", data)
}

// dashboardTiles is the answer to "how is the shop doing right now": what came
// in this month, how much of it stayed, what is still on the ramps, and whether
// the quotes going out are being accepted.
func dashboardTiles(dash store.Dashboard, currency string) []statTile {
	current, previous := dash.MonthToDate, dash.PreviousToDate

	revenue := statTile{
		Label: "Оборот този месец",
		Value: pdf.FormatMoney(current.NetRevenueCents, currency),
		Sub:   fmt.Sprintf("без ДДС · %d завършени ремонта", current.Repairs),
		URL:   "/repairs?status=completed",
	}
	applyDelta(&revenue, current.NetRevenueCents, previous.NetRevenueCents)

	profit := statTile{
		Label: "Печалба този месец",
		Value: pdf.FormatMoney(current.NetProfitCents(), currency),
		Sub:   "марж " + formatMargin(current.NetProfitCents(), current.NetRevenueCents) + " · части " + pdf.FormatMoney(current.NetCostCents, currency),
		URL:   "/repairs?status=completed",
	}
	applyDelta(&profit, current.NetProfitCents(), previous.NetProfitCents())

	active := statTile{
		Label: "В работа",
		Value: fmt.Sprintf("%d", dash.ActiveRepairs),
		Sub:   "незавършени ремонта за " + pdf.FormatMoney(dash.ActiveTotalCents, currency),
		URL:   "/repairs",
	}
	if dash.ActiveRepairs == 0 {
		active.Sub = "няма ремонти в процес"
	}

	accepted := statTile{
		Label: "Приети оферти",
		Value: fmt.Sprintf("%d%%", dash.AcceptanceRate()),
		Sub:   fmt.Sprintf("%d от %d за последните 90 дни", dash.AcceptedOffers, dash.DecidedOffers),
		URL:   "/offers",
	}
	if dash.DecidedOffers == 0 {
		accepted.Value = "—"
		accepted.Sub = "още няма приети или отказани оферти"
	}
	if dash.OpenOffers > 0 {
		accepted.Sub += fmt.Sprintf(" · чакат отговор %d за %s", dash.OpenOffers, pdf.FormatMoney(dash.OpenOfferTotalCents, currency))
	}

	return []statTile{revenue, profit, active, accepted}
}

// applyDelta states the change against the same stretch of last month, so a
// dashboard opened on the 3rd compares three days with three days. With nothing
// to compare against - a first month, or one that started from zero - it says
// nothing rather than inventing a percentage of zero.
func applyDelta(tile *statTile, current, previous int64) {
	if previous <= 0 || current == previous {
		return
	}
	change := (current - previous) * 100 / previous
	tile.Rising = change > 0
	tile.Falling = change < 0
	if change < 0 {
		change = -change
	}
	direction := "повече"
	if tile.Falling {
		direction = "по-малко"
	}
	tile.Delta = fmt.Sprintf("%d%% %s спрямо същия период миналия месец", change, direction)
}

// attentionBoard is the panel an owner should read first: quotes nobody has
// answered and jobs that have not moved. Both are money standing still.
func attentionBoard(dash store.Dashboard, currency string, now time.Time) *documentBoard {
	rows := make([]documentRow, 0, len(dash.StaleOffers)+len(dash.StalledRepairs))
	for _, summary := range dash.StaleOffers {
		row := offerDocumentRow(summary)
		row.TypeLabel = "Оферта"
		row.Note = waitingNote(summary.Offer.CreatedAt, now)
		rows = append(rows, row)
	}
	for _, summary := range dash.StalledRepairs {
		row := repairDocumentRow(summary)
		row.TypeLabel = "Ремонт"
		row.Note = waitingNote(summary.Repair.CreatedAt, now)
		rows = append(rows, row)
	}
	return &documentBoard{
		Rows:     rows,
		Currency: currency,
		Empty: emptyState{
			Title: "Нищо не е забравено",
			Text:  "Няма оферти без отговор и ремонти без движение от повече от две седмици.",
		},
	}
}

func (app *application) garageView(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form.Values.Set("name", data.Tenant.Name)
	data.Form.Values.Set("address", data.Tenant.Address)
	data.Form.Values.Set("vat_number", data.Tenant.VATNumber)
	data.Form.Values.Set("currency", data.Tenant.Currency)
	app.render(w, r, "garage.page.html", data)
}

func (app *application) garageUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("name", "currency")
	form.MaxLength("name", 100)
	form.MaxLength("currency", 3)
	data := app.newTemplateData(r)
	data.Form = form
	if !form.Valid() {
		app.render(w, r, "garage.page.html", data)
		return
	}
	data.Tenant.Name = clean(form.Get("name"))
	data.Tenant.Address = clean(form.Get("address"))
	data.Tenant.VATNumber = clean(form.Get("vat_number"))
	data.Tenant.Currency = strings.ToUpper(clean(form.Get("currency")))
	data.Tenant.DefaultTaxRate = domain.StandardVATRateBPS
	if err := app.tenants.Update(r.Context(), data.Tenant); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.sessions.Put(r.Context(), "garageName", data.Tenant.Name)
	app.flash(r, "Настройките на сервиза са запазени.")
	http.Redirect(w, r, "/garage", http.StatusSeeOther)
}
