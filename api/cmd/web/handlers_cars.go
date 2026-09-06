package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/google/uuid"
)

func (app *application) carsList(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	search := clean(r.URL.Query().Get("q"))
	cars, total, err := app.cars.ListAll(r.Context(), app.tenantID(r), store.CarListParams{Search: search, IncludeArchived: r.URL.Query().Get("archived") == "1", Limit: 500})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.Cars, data.Total = cars, total
	data.Form.Values.Set("q", search)
	if r.URL.Query().Get("archived") == "1" {
		data.Form.Values.Set("archived", "1")
	}
	app.render(w, r, "cars.page.html", data)
}

func (app *application) carCreateView(w http.ResponseWriter, r *http.Request) {
	customer, err := app.customers.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	data := app.newTemplateData(r)
	data.Customer = customer
	app.render(w, r, "car-form.page.html", data)
}

func (app *application) carCreate(w http.ResponseWriter, r *http.Request) {
	customer, err := app.customers.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	form, car, ok := app.readCarForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Customer, data.Form = customer, form
	if !form.Valid() {
		app.render(w, r, "car-form.page.html", data)
		return
	}
	car.ID, car.TenantID, car.CustomerID = uuid.NewString(), app.tenantID(r), customer.ID
	if err := app.cars.Create(r.Context(), car); err != nil {
		if errors.Is(err, store.ErrDuplicatePlate) {
			form.Errors["plate"] = "Този регистрационен номер вече е регистриран."
			app.render(w, r, "car-form.page.html", data)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Автомобилът е добавен.")
	http.Redirect(w, r, "/cars/"+car.ID, http.StatusSeeOther)
}

func (app *application) carView(w http.ResponseWriter, r *http.Request) {
	tenantID := app.tenantID(r)
	car, err := app.cars.Get(r.Context(), tenantID, r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	var customer *domain.Customer
	var history []store.HistoryEntry
	var offers []*domain.Offer
	if err := runConcurrently(r.Context(),
		func(ctx context.Context) error {
			var err error
			customer, err = app.customers.Get(ctx, tenantID, car.CustomerID)
			return err
		},
		func(ctx context.Context) error {
			var err error
			history, err = app.history.ListByCar(ctx, tenantID, car.ID)
			return err
		},
		func(ctx context.Context) error {
			var err error
			offers, _, err = app.offers.List(ctx, tenantID, car.ID, store.OfferListParams{Limit: 100})
			return err
		},
	); err != nil {
		app.serverError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Car, data.Customer, data.History, data.CarOffers = car, customer, history, offers
	app.render(w, r, "car.page.html", data)
}

func (app *application) carEditView(w http.ResponseWriter, r *http.Request) {
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
	data.Car, data.Customer, data.Form = car, customer, carForm(car)
	app.render(w, r, "car-form.page.html", data)
}

func (app *application) carEdit(w http.ResponseWriter, r *http.Request) {
	current, err := app.cars.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	customer, err := app.customers.Get(r.Context(), app.tenantID(r), current.CustomerID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	form, car, ok := app.readCarForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Car, data.Customer, data.Form = current, customer, form
	if !form.Valid() {
		app.render(w, r, "car-form.page.html", data)
		return
	}
	car.ID, car.TenantID, car.CustomerID = current.ID, current.TenantID, current.CustomerID
	if err := app.cars.Update(r.Context(), car); err != nil {
		if errors.Is(err, store.ErrDuplicatePlate) {
			form.Errors["plate"] = "Този регистрационен номер вече е регистриран."
			app.render(w, r, "car-form.page.html", data)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Автомобилът е обновен.")
	http.Redirect(w, r, "/cars/"+car.ID, http.StatusSeeOther)
}

func (app *application) carArchive(w http.ResponseWriter, r *http.Request) {
	err := app.cars.Archive(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	app.flash(r, "Автомобилът е архивиран.")
	http.Redirect(w, r, "/cars", http.StatusSeeOther)
}

func (app *application) readCarForm(w http.ResponseWriter, r *http.Request) (*forms.Form, *domain.Car, bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return nil, nil, false
	}
	form := forms.New(r.PostForm)
	form.Required("plate")
	form.MaxLength("plate", 20)
	form.MaxLength("vin", 50)
	year, yearOK := parseNonNegativeInt(form.Get("year"))
	mileage, mileageOK := parseNonNegativeInt(form.Get("mileage"))
	if !yearOK || year > 2100 {
		form.Errors["year"] = "Въведете валидна година."
	}
	if !mileageOK {
		form.Errors["mileage"] = "Въведете валиден пробег."
	}
	car := &domain.Car{Plate: strings.ToUpper(clean(form.Get("plate"))), VIN: strings.ToUpper(clean(form.Get("vin"))), Make: clean(form.Get("make")), Model: clean(form.Get("model")), Year: year, Mileage: mileage}
	return form, car, true
}

func carForm(car *domain.Car) *forms.Form {
	form := forms.New(nil)
	form.Values.Set("plate", car.Plate)
	form.Values.Set("vin", car.VIN)
	form.Values.Set("make", car.Make)
	form.Values.Set("model", car.Model)
	if car.Year != 0 {
		form.Values.Set("year", strconv.Itoa(car.Year))
	}
	if car.Mileage != 0 {
		form.Values.Set("mileage", strconv.Itoa(car.Mileage))
	}
	return form
}
