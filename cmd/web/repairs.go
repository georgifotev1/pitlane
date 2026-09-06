package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
)

const maxMileage = 10_000_000

func (app *application) repairsList(w http.ResponseWriter, r *http.Request) {
	status := clean(r.URL.Query().Get("status"))
	if status != "" && !domain.IsValidRepairStatus(status) {
		status = ""
	}
	customerSearch := clean(r.URL.Query().Get("customer"))
	tenantID := app.tenantID(r)
	page := requestedPage(r)
	var repairs []store.RepairSummary
	var stats store.RepairStats
	var total int
	if err := runConcurrently(r.Context(),
		func(ctx context.Context) error {
			var err error
			repairs, total, err = app.repairs.List(ctx, tenantID, store.RepairListParams{
				Status:         status,
				CustomerSearch: customerSearch,
				Limit:          listPageSize,
				Offset:         pageOffset(page),
			})
			return err
		},
		func(ctx context.Context) error {
			var err error
			stats, err = app.repairs.Stats(ctx, tenantID)
			return err
		},
	); err != nil {
		app.serverError(w, r, err)
		return
	}
	if redirectIfPageOutOfRange(w, r, page, total) {
		return
	}
	data := app.newTemplateData(r)
	data.Repairs, data.Total, data.RepairStats = repairs, total, stats
	data.Pagination = newPagination(r, page, total)
	data.Board = repairBoard(repairs, app.currency(data))
	data.Stats = repairStatTiles(stats, app.currency(data))
	data.Form.Values.Set("status", status)
	data.Form.Values.Set("customer", customerSearch)
	app.render(w, r, "repairs.page.html", data)
}

// repairStatTiles gives the repairs board the same headline row as the
// dashboard: how much work is in the shop, what has been billed and what was
// left after the parts.
func repairStatTiles(stats store.RepairStats, currency string) []statTile {
	tiles := []statTile{
		{Label: "Активни", Value: strconv.Itoa(stats.Active), Sub: "отворени и в работа", URL: "/repairs"},
		{Label: "Завършени", Value: strconv.Itoa(stats.Completed), Sub: "отчетени ремонти", URL: "/repairs?status=completed"},
		{Label: "Приход от завършени", Value: pdf.FormatMoney(stats.RevenueCents, currency), Sub: "с ДДС", URL: "/repairs?status=completed"},
	}
	if stats.HasCost {
		tiles = append(tiles, statTile{
			Label: "Печалба след части",
			Value: pdf.FormatMoney(stats.ProfitCents, currency),
			Sub:   "без ДДС · марж " + formatMargin(stats.ProfitCents, netRevenueOf(stats)),
			URL:   "/repairs?status=completed",
		})
	}
	return tiles
}

// netRevenueOf strips VAT from billed revenue at the standard rate so the
// margin is a ratio of two net figures.
func netRevenueOf(stats store.RepairStats) int64 {
	return domain.NetCents(stats.RevenueCents, domain.StandardVATRateBPS)
}

func (app *application) repairView(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.repairBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	data := app.newTemplateData(r)
	setRepairData(data, bundle)
	app.render(w, r, "repair.page.html", data)
}

func (app *application) repairEditView(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.repairBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	if bundle.Repair.Status != domain.RepairStatusOpen {
		http.Redirect(w, r, "/repairs/"+bundle.Repair.ID, http.StatusSeeOther)
		return
	}
	form := forms.New(nil)
	form.Values.Set("notes", bundle.Repair.Notes)
	mileage := bundle.Repair.Mileage
	if mileage == 0 {
		mileage = bundle.Car.Mileage
	}
	form.Values.Set("mileage", strconv.Itoa(mileage))
	data := app.newTemplateData(r)
	setRepairData(data, bundle)
	data.Form = form
	data.Editor = newItemEditor(repairEditorTitle, repairEditorHint, repairRowsFor(bundle.Repair), form)
	app.render(w, r, "repair-form.page.html", data)
}

func (app *application) repairEdit(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.repairBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	form, repair, ok := app.readRepairForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	setRepairData(data, bundle)
	data.Form = form
	data.Editor = newItemEditor(repairEditorTitle, repairEditorHint, offerRowsFromForm(form), form)
	if !form.Valid() {
		app.render(w, r, "repair-form.page.html", data)
		return
	}
	repair.ID = bundle.Repair.ID
	repair.TenantID = bundle.Repair.TenantID
	repair.CarID = bundle.Repair.CarID
	repair.OfferID = bundle.Repair.OfferID
	if err := app.repairs.Update(r.Context(), repair); err != nil {
		if errors.Is(err, store.ErrRepairNotOpen) {
			app.flash(r, "Ремонтът вече не може да бъде редактиран.")
			http.Redirect(w, r, "/repairs/"+repair.ID, http.StatusSeeOther)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Ремонтът е обновен.")
	http.Redirect(w, r, "/repairs/"+repair.ID, http.StatusSeeOther)
}

func (app *application) repairStart(w http.ResponseWriter, r *http.Request) {
	app.changeRepairStatus(w, r, domain.RepairStatusInProgress, "Работата по ремонта е започната.")
}

func (app *application) repairReopen(w http.ResponseWriter, r *http.Request) {
	app.changeRepairStatus(w, r, domain.RepairStatusOpen, "Ремонтът е върнат в отворено състояние.")
}

func (app *application) changeRepairStatus(w http.ResponseWriter, r *http.Request, status domain.RepairStatus, message string) {
	id := r.PathValue("id")
	_, err := app.repairs.SetStatus(r.Context(), app.tenantID(r), id, status)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			app.notFound(w)
		case errors.Is(err, store.ErrInvalidRepairStatusTransition):
			app.flash(r, "Статусът на ремонта вече е променен.")
			http.Redirect(w, r, "/repairs/"+id, http.StatusSeeOther)
		default:
			app.serverError(w, r, err)
		}
		return
	}
	app.flash(r, message)
	http.Redirect(w, r, "/repairs/"+id, http.StatusSeeOther)
}

func (app *application) repairMileageUpdate(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.repairBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("mileage")
	mileage, ok := parseNonNegativeInt(form.Get("mileage"))
	if !ok || mileage > maxMileage {
		form.Errors["mileage"] = "Въведете валиден пробег до 10 000 000 км."
	}
	if !form.Valid() {
		data := app.newTemplateData(r)
		setRepairData(data, bundle)
		data.Form = form
		app.render(w, r, "repair.page.html", data)
		return
	}
	if _, err := app.repairs.UpdateMileage(r.Context(), app.tenantID(r), bundle.Repair.ID, mileage); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Пробегът на ремонта и автомобила е обновен.")
	http.Redirect(w, r, "/repairs/"+bundle.Repair.ID, http.StatusSeeOther)
}

func (app *application) repairComplete(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.repairBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("mileage")
	mileage, ok := parseNonNegativeInt(form.Get("mileage"))
	if !ok || mileage > maxMileage {
		form.Errors["mileage"] = "Въведете валиден пробег до 10 000 000 км."
	}
	if !form.Valid() {
		data := app.newTemplateData(r)
		setRepairData(data, bundle)
		data.Form = form
		app.render(w, r, "repair.page.html", data)
		return
	}
	if _, err := app.repairs.Complete(r.Context(), app.tenantID(r), bundle.Repair.ID, mileage); err != nil {
		if errors.Is(err, store.ErrRepairNotOpen) {
			app.flash(r, "Ремонтът вече е завършен.")
			http.Redirect(w, r, "/repairs/"+bundle.Repair.ID, http.StatusSeeOther)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Ремонтът е завършен и приходът е отчетен.")
	http.Redirect(w, r, "/repairs/"+bundle.Repair.ID, http.StatusSeeOther)
}

func (app *application) readRepairForm(w http.ResponseWriter, r *http.Request) (*forms.Form, *domain.Repair, bool) {
	form, offer, ok := app.readOfferForm(w, r)
	if !ok {
		return nil, nil, false
	}
	mileage, mileageOK := parseNonNegativeInt(form.Get("mileage"))
	if !mileageOK || mileage > maxMileage {
		form.Errors["mileage"] = "Въведете валиден пробег до 10 000 000 км."
	}
	items := make([]domain.RepairItem, 0, len(offer.Items))
	for _, item := range offer.Items {
		items = append(items, domain.RepairItem{
			Kind:           item.Kind,
			Description:    item.Description,
			Quantity:       item.Quantity,
			UnitPriceCents: item.UnitPriceCents,
			CostCents:      item.CostCents,
		})
	}
	return form, &domain.Repair{TaxRateBps: offer.TaxRateBps, Mileage: mileage, Notes: offer.Notes, Items: items}, true
}

func repairRowsFor(repair *domain.Repair) []offerRow {
	n := max(len(repair.Items), 1)
	rows := blankOfferRows(n)
	for i, item := range repair.Items {
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

func setRepairData(data *templateData, bundle *repairBundle) {
	data.Repair = bundle.Repair
	data.Car = bundle.Car
	data.Customer = bundle.Customer
	data.Tenant = bundle.Tenant
	data.Document = repairDocument(bundle.Repair, bundle.Tenant.Currency)
	data.Margin = repairMargin(bundle.Repair, bundle.Tenant.Currency)
}
