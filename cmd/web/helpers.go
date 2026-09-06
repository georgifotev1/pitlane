package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/store"
	"golang.org/x/sync/errgroup"
)

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	app.logger.Error("request failed", "method", r.Method, "url", r.URL.RequestURI(), "err", err)
	http.Error(w, "Възникна вътрешна грешка.", http.StatusInternalServerError)
}

func (app *application) notFound(w http.ResponseWriter) {
	http.Error(w, "Страницата не е намерена.", http.StatusNotFound)
}

func (app *application) tenantID(r *http.Request) string {
	return app.sessions.GetString(r.Context(), "tenantID")
}
func (app *application) userID(r *http.Request) string {
	return app.sessions.GetString(r.Context(), "userID")
}

const maxConcurrentStoreCalls = 3

// runConcurrently runs independent store calls concurrently while preventing
// one request from occupying too much of the database connection pool.
func runConcurrently(ctx context.Context, tasks ...func(context.Context) error) error {
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(maxConcurrentStoreCalls)
	for _, task := range tasks {
		group.Go(func() error {
			return task(ctx)
		})
	}
	return group.Wait()
}

func (app *application) flash(r *http.Request, message string) {
	app.sessions.Put(r.Context(), "flash", message)
}

func clean(value string) string { return strings.TrimSpace(value) }

const listPageSize = 25

// requestedPage treats missing, malformed and overflowing page values as the
// first page. This keeps list offsets bounded before they reach PostgreSQL.
func requestedPage(r *http.Request) int {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	maxInt := int(^uint(0) >> 1)
	if err != nil || page < 1 || page-1 > maxInt/listPageSize {
		return 1
	}
	return page
}

func paginationPageCount(total int) int {
	if total < 1 {
		return 1
	}
	return (total-1)/listPageSize + 1
}

func pageOffset(page int) int { return (page - 1) * listPageSize }

// redirectIfPageOutOfRange avoids rendering an empty table when rows disappear
// or a user follows a stale page link. Search and filter parameters survive.
func redirectIfPageOutOfRange(w http.ResponseWriter, r *http.Request, page, total int) bool {
	lastPage := paginationPageCount(total)
	if page <= lastPage {
		return false
	}
	http.Redirect(w, r, paginationURL(r, lastPage), http.StatusSeeOther)
	return true
}

func newPagination(r *http.Request, page, total int) *pagination {
	pages := paginationPageCount(total)
	p := &pagination{
		CurrentPage: page,
		TotalPages:  pages,
		TotalItems:  total,
		HasPrevious: page > 1,
		HasNext:     page < pages,
		Show:        pages > 1,
	}
	if total > 0 {
		offset := pageOffset(page)
		p.FirstItem = offset + 1
		p.LastItem = total
		if total-offset > listPageSize {
			p.LastItem = offset + listPageSize
		}
	}
	if p.HasPrevious {
		p.PreviousURL = paginationURL(r, page-1)
	}
	if p.HasNext {
		p.NextURL = paginationURL(r, page+1)
	}
	return p
}

func paginationURL(r *http.Request, page int) string {
	query := r.URL.Query()
	if page <= 1 {
		query.Del("page")
	} else {
		query.Set("page", strconv.Itoa(page))
	}
	if encoded := query.Encode(); encoded != "" {
		return r.URL.Path + "?" + encoded
	}
	return r.URL.Path
}

// currency is the tenant's currency, or the empty string on a page rendered
// before a tenant is loaded - pdf.FormatMoney reads that as the euro.
func (app *application) currency(data *templateData) string {
	if data.Tenant == nil {
		return ""
	}
	return data.Tenant.Currency
}

func parseNonNegativeInt(value string) (int, bool) {
	if clean(value) == "" {
		return 0, true
	}
	n, err := strconv.Atoi(clean(value))
	return n, err == nil && n >= 0
}

func parseCents(value string) (int64, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, " ", ""))
	value = strings.ReplaceAll(value, ",", ".")
	if value == "" {
		return 0, false
	}
	if strings.HasPrefix(value, "-") || strings.Count(value, ".") > 1 {
		return 0, false
	}
	parts := strings.SplitN(value, ".", 2)
	if parts[0] == "" {
		parts[0] = "0"
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, false
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 2 {
		return 0, false
	}
	fraction += strings.Repeat("0", 2-len(fraction))
	frac := int64(0)
	if fraction != "" {
		frac, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, false
		}
	}
	if whole > (1<<63-1-frac)/100 {
		return 0, false
	}
	return whole*100 + frac, true
}

func centsInput(cents int64) string { return fmt.Sprintf("%d.%02d", cents/100, cents%100) }

// optionalCentsInput leaves a zero cost blank rather than pre-filling 0.00, so
// an empty supplier price stays visibly empty and unentered cost is never
// mistaken for a part that was free.
func optionalCentsInput(cents int64) string {
	if cents == 0 {
		return ""
	}
	return centsInput(cents)
}
func stringInt(n int) string { return strconv.Itoa(n) }

func offerRowsFromForm(form *forms.Form) []offerRow {
	if form == nil {
		return blankOfferRows(1)
	}
	kinds := form.Values["item_kind"]
	descriptions := form.Values["item_description"]
	quantities := form.Values["item_quantity"]
	prices := form.Values["item_price"]
	costs := form.Values["item_cost"]
	n := max(len(kinds), len(descriptions), len(quantities), len(prices), len(costs), 1)
	rows := make([]offerRow, n)
	for i := range rows {
		rows[i] = offerRow{Kind: "part", Quantity: "1"}
		if i < len(kinds) && kinds[i] != "" {
			rows[i].Kind = kinds[i]
		}
		if i < len(descriptions) {
			rows[i].Description = descriptions[i]
		}
		if i < len(quantities) {
			rows[i].Quantity = quantities[i]
		}
		if i < len(prices) {
			rows[i].UnitPrice = prices[i]
		}
		if i < len(costs) {
			rows[i].Cost = costs[i]
		}
	}
	return rows
}

func blankOfferRows(n int) []offerRow {
	rows := make([]offerRow, n)
	for i := range rows {
		rows[i] = offerRow{Kind: "part", Quantity: "1"}
	}
	return rows
}

func offerRowsFor(o *domain.Offer) []offerRow {
	n := max(len(o.Items), 1)
	rows := blankOfferRows(n)
	for i, item := range o.Items {
		rows[i] = offerRow{
			Kind:        string(item.Kind),
			Description: item.Description,
			Quantity:    strconv.Itoa(item.Quantity),
			UnitPrice:   centsInput(item.UnitPriceCents),
			Cost:        optionalCentsInput(item.CostCents),
		}
	}
	return rows
}

func (app *application) offerBundle(r *http.Request, id string) (*offerBundle, error) {
	tenantID := app.tenantID(r)
	offer, err := app.offers.Get(r.Context(), tenantID, id)
	if err != nil {
		return nil, err
	}
	var car *domain.Car
	var tenant *domain.Tenant
	if err := runConcurrently(r.Context(),
		func(ctx context.Context) error {
			var err error
			car, err = app.cars.Get(ctx, tenantID, offer.CarID)
			return err
		},
		func(ctx context.Context) error {
			var err error
			tenant, err = app.tenants.GetByID(ctx, tenantID)
			return err
		},
	); err != nil {
		return nil, err
	}
	customer, err := app.customers.Get(r.Context(), tenantID, car.CustomerID)
	if err != nil {
		return nil, err
	}
	return &offerBundle{Offer: offer, Car: car, Customer: customer, Tenant: tenant}, nil
}

func (app *application) repairBundle(r *http.Request, id string) (*repairBundle, error) {
	tenantID := app.tenantID(r)
	repair, err := app.repairs.Get(r.Context(), tenantID, id)
	if err != nil {
		return nil, err
	}
	var car *domain.Car
	var tenant *domain.Tenant
	if err := runConcurrently(r.Context(),
		func(ctx context.Context) error {
			var err error
			car, err = app.cars.Get(ctx, tenantID, repair.CarID)
			return err
		},
		func(ctx context.Context) error {
			var err error
			tenant, err = app.tenants.GetByID(ctx, tenantID)
			return err
		},
	); err != nil {
		return nil, err
	}
	customer, err := app.customers.Get(r.Context(), tenantID, car.CustomerID)
	if err != nil {
		return nil, err
	}
	return &repairBundle{Repair: repair, Car: car, Customer: customer, Tenant: tenant}, nil
}

func handleStoreError(app *application, w http.ResponseWriter, r *http.Request, err error) bool {
	if errors.Is(err, store.ErrNotFound) {
		app.notFound(w)
		return true
	}
	if err != nil {
		app.serverError(w, r, err)
		return true
	}
	return false
}
