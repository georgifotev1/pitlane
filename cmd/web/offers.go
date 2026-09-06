package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/pdf"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/google/uuid"
)

const (
	offerEditorTitle  = "Редове"
	offerEditorHint   = "Цените към клиента са с включено ДДС. Доставната цена е това, което плащате на доставчика - тя остава само за вас."
	repairEditorTitle = "Извършвана работа"
	repairEditorHint  = "Коригирайте частите и труда преди да започнете ремонта. Доставната цена не се вижда от клиента."
)

func (app *application) offersList(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	page := requestedPage(r)
	status := clean(r.URL.Query().Get("status"))
	customerSearch := clean(r.URL.Query().Get("customer"))
	offers, total, err := app.offers.ListAll(r.Context(), app.tenantID(r), store.OfferBoardParams{
		Status:         status,
		CustomerSearch: customerSearch,
		Limit:          listPageSize,
		Offset:         pageOffset(page),
	})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if redirectIfPageOutOfRange(w, r, page, total) {
		return
	}
	data.Offers, data.Total = offers, total
	data.Pagination = newPagination(r, page, total)
	data.Board = offerBoard(offers, app.currency(data))
	data.Form.Values.Set("status", status)
	data.Form.Values.Set("customer", customerSearch)
	app.render(w, r, "offers.page.html", data)
}

func (app *application) offerCreateView(w http.ResponseWriter, r *http.Request) {
	car, err := app.cars.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	customer, err := app.customers.Get(r.Context(), app.tenantID(r), car.CustomerID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Car, data.Customer = car, customer
	data.Editor = newItemEditor(offerEditorTitle, offerEditorHint, blankOfferRows(1), nil)
	app.render(w, r, "offer-form.page.html", data)
}

func (app *application) offerCreate(w http.ResponseWriter, r *http.Request) {
	car, err := app.cars.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	customer, err := app.customers.Get(r.Context(), app.tenantID(r), car.CustomerID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	form, offer, ok := app.readOfferForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Car, data.Customer, data.Form = car, customer, form
	data.Editor = newItemEditor(offerEditorTitle, offerEditorHint, offerRowsFromForm(form), form)
	if !form.Valid() {
		app.render(w, r, "offer-form.page.html", data)
		return
	}
	offer.ID, offer.TenantID, offer.CarID = uuid.NewString(), app.tenantID(r), car.ID
	if err := app.offers.Create(r.Context(), offer); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Офертата е създадена.")
	http.Redirect(w, r, "/offers/"+offer.ID, http.StatusSeeOther)
}

func (app *application) offerView(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.offerBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	data := app.newTemplateData(r)
	data.Offer, data.Car, data.Customer, data.Tenant = bundle.Offer, bundle.Car, bundle.Customer, bundle.Tenant
	data.Document = offerDocument(bundle.Offer, bundle.Tenant.Currency)
	data.Margin = offerMargin(bundle.Offer, bundle.Tenant.Currency)
	if bundle.Offer.Status == domain.OfferStatusAccepted {
		data.Repair, err = app.repairs.GetByOffer(r.Context(), app.tenantID(r), bundle.Offer.ID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
	}
	app.render(w, r, "offer.page.html", data)
}

func (app *application) offerAccept(w http.ResponseWriter, r *http.Request) {
	repair, err := app.repairs.CreateFromOffer(r.Context(), app.tenantID(r), r.PathValue("id"))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			app.notFound(w)
		case errors.Is(err, store.ErrOfferNotAcceptable):
			app.flash(r, "Офертата вече не може да бъде приета.")
			http.Redirect(w, r, "/offers/"+r.PathValue("id"), http.StatusSeeOther)
		default:
			app.serverError(w, r, err)
		}
		return
	}
	app.flash(r, "Офертата е приета и ремонтът е създаден.")
	http.Redirect(w, r, "/repairs/"+repair.ID, http.StatusSeeOther)
}

func (app *application) offerEditView(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.offerBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	if bundle.Offer.Status != domain.OfferStatusDraft {
		http.Redirect(w, r, "/offers/"+bundle.Offer.ID, http.StatusSeeOther)
		return
	}
	form := forms.New(nil)
	form.Values.Set("notes", bundle.Offer.Notes)
	data := app.newTemplateData(r)
	data.Offer, data.Car, data.Customer, data.Form = bundle.Offer, bundle.Car, bundle.Customer, form
	data.Editor = newItemEditor(offerEditorTitle, offerEditorHint, offerRowsFor(bundle.Offer), form)
	app.render(w, r, "offer-form.page.html", data)
}

func (app *application) offerEdit(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.offerBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	form, updated, ok := app.readOfferForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Offer, data.Car, data.Customer, data.Form = bundle.Offer, bundle.Car, bundle.Customer, form
	data.Editor = newItemEditor(offerEditorTitle, offerEditorHint, offerRowsFromForm(form), form)
	if !form.Valid() {
		app.render(w, r, "offer-form.page.html", data)
		return
	}
	updated.ID, updated.TenantID, updated.CarID = bundle.Offer.ID, bundle.Offer.TenantID, bundle.Offer.CarID
	if err := app.offers.Update(r.Context(), updated); err != nil {
		if errors.Is(err, store.ErrOfferNotDraft) {
			http.Redirect(w, r, "/offers/"+updated.ID, http.StatusSeeOther)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Офертата е обновена.")
	http.Redirect(w, r, "/offers/"+updated.ID, http.StatusSeeOther)
}

func (app *application) offerPrint(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.offerBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	data := app.newTemplateData(r)
	data.Offer, data.Car, data.Customer, data.Tenant = bundle.Offer, bundle.Car, bundle.Customer, bundle.Tenant
	// The print sheet gets the customer-facing document only: no margin view.
	data.Document = offerDocument(bundle.Offer, bundle.Tenant.Currency)
	app.render(w, r, "offer-print.page.html", data)
}

func (app *application) offerPDF(w http.ResponseWriter, r *http.Request) {
	bundle, err := app.offerBundle(r, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	doc, err := app.pdf.RenderOffer(pdf.OfferData{Tenant: bundle.Tenant, Customer: bundle.Customer, Car: bundle.Car, Offer: bundle.Offer})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, "offer-"+bundle.Offer.DocumentNumber+".pdf"))
	w.Header().Set("Content-Length", strconv.Itoa(len(doc)))
	_, _ = w.Write(doc)
}

func (app *application) readOfferForm(w http.ResponseWriter, r *http.Request) (*forms.Form, *domain.Offer, bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return nil, nil, false
	}
	form := forms.New(r.PostForm)
	kinds, descriptions := form.Values["item_kind"], form.Values["item_description"]
	quantities, prices := form.Values["item_quantity"], form.Values["item_price"]
	costs := form.Values["item_cost"]
	n := max(len(kinds), len(descriptions), len(quantities), len(prices), len(costs))
	items := make([]domain.OfferItem, 0, n)
	for i := 0; i < n; i++ {
		kind, description, quantityText, priceText := valueAt(kinds, i), clean(valueAt(descriptions, i)), valueAt(quantities, i), valueAt(prices, i)
		if description == "" && clean(priceText) == "" {
			continue
		}
		quantity, qerr := strconv.Atoi(clean(quantityText))
		price, priceOK := parseCents(priceText)
		if description == "" || qerr != nil || quantity < 1 || !priceOK {
			form.Errors["items"] = "Попълнете всеки използван ред с описание, положително количество и валидна цена."
			continue
		}
		if !domain.IsValidOfferItemKind(kind) {
			form.Errors["items"] = "Изберете валиден вид на реда."
			continue
		}
		// The supplier price is optional - labour has none - so an empty field
		// is a legitimate zero, but a filled-in one must still be a number.
		cost := int64(0)
		if costText := clean(valueAt(costs, i)); costText != "" {
			parsed, costOK := parseCents(costText)
			if !costOK {
				form.Errors["items"] = "Доставната цена трябва да е валидна сума или да остане празна."
				continue
			}
			cost = parsed
		}
		items = append(items, domain.OfferItem{
			Kind:           domain.OfferItemKind(kind),
			Description:    description,
			Quantity:       quantity,
			UnitPriceCents: price,
			CostCents:      cost,
		})
	}
	if len(items) == 0 {
		form.Errors["items"] = "Добавете поне един ред към офертата."
	}
	offer := &domain.Offer{TaxRateBps: domain.StandardVATRateBPS, Notes: clean(form.Get("notes")), Items: items}
	return form, offer, true
}

func valueAt(values []string, i int) string {
	if i < len(values) {
		return values[i]
	}
	return ""
}
