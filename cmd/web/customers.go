package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/google/uuid"
)

func (app *application) customersList(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	search := clean(r.URL.Query().Get("q"))
	includeArchived := r.URL.Query().Get("archived") == "1"
	customers, total, err := app.customers.List(r.Context(), app.tenantID(r), store.CustomerListParams{Search: search, IncludeArchived: includeArchived, Limit: 500})
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.Customers, data.Total = customers, total
	data.Form.Values.Set("q", search)
	if includeArchived {
		data.Form.Values.Set("archived", "1")
	}
	app.render(w, r, "customers.page.html", data)
}

func (app *application) customerCreateView(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "customer-form.page.html", app.newTemplateData(r))
}

func (app *application) customerCreate(w http.ResponseWriter, r *http.Request) {
	form, ok := app.readCustomerForm(w, r)
	if !ok {
		return
	}
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, "customer-form.page.html", data)
		return
	}
	customer := customerFromForm(form)
	customer.ID, customer.TenantID = uuid.NewString(), app.tenantID(r)
	if err := app.customers.Create(r.Context(), customer); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Клиентът е създаден.")
	http.Redirect(w, r, fmt.Sprintf("/customers/%s", customer.ID), http.StatusSeeOther)
}

func (app *application) customerView(w http.ResponseWriter, r *http.Request) {
	tenantID := app.tenantID(r)
	customerID := r.PathValue("id")
	var customer *domain.Customer
	var cars []*domain.Car
	if err := runConcurrently(r.Context(),
		func(ctx context.Context) error {
			var err error
			customer, err = app.customers.Get(ctx, tenantID, customerID)
			return err
		},
		func(ctx context.Context) error {
			var err error
			cars, _, err = app.cars.List(ctx, tenantID, customerID, store.CarListParams{Limit: 500, IncludeArchived: true})
			return err
		},
	); handleStoreError(app, w, r, err) {
		return
	}
	data := app.newTemplateData(r)
	data.Customer, data.CustomerCars = customer, cars
	app.render(w, r, "customer.page.html", data)
}

func (app *application) customerEditView(w http.ResponseWriter, r *http.Request) {
	customer, err := app.customers.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	data := app.newTemplateData(r)
	data.Customer = customer
	data.Form = customerForm(customer)
	app.render(w, r, "customer-form.page.html", data)
}

func (app *application) customerEdit(w http.ResponseWriter, r *http.Request) {
	customer, err := app.customers.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	form, ok := app.readCustomerForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Customer, data.Form = customer, form
	if !form.Valid() {
		app.render(w, r, "customer-form.page.html", data)
		return
	}
	updated := customerFromForm(form)
	updated.ID, updated.TenantID, updated.ArchivedAt, updated.CreatedAt = customer.ID, customer.TenantID, customer.ArchivedAt, customer.CreatedAt
	if err := app.customers.Update(r.Context(), updated); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Клиентът е обновен.")
	http.Redirect(w, r, "/customers/"+customer.ID, http.StatusSeeOther)
}

func (app *application) customerArchive(w http.ResponseWriter, r *http.Request) {
	err := app.customers.Archive(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	app.flash(r, "Клиентът е архивиран.")
	http.Redirect(w, r, "/customers", http.StatusSeeOther)
}

func (app *application) readCustomerForm(w http.ResponseWriter, r *http.Request) (*forms.Form, bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return nil, false
	}
	form := forms.New(r.PostForm)
	form.Required("name")
	form.MaxLength("name", 150)
	form.MaxLength("email", 254)
	form.Email("email")
	return form, true
}

func customerFromForm(form *forms.Form) *domain.Customer {
	return &domain.Customer{Name: clean(form.Get("name")), Company: clean(form.Get("company")), Email: clean(form.Get("email")), Phone: clean(form.Get("phone")), Address: clean(form.Get("address")), Notes: clean(form.Get("notes"))}
}

func customerForm(c *domain.Customer) *forms.Form {
	form := forms.New(nil)
	form.Values.Set("name", c.Name)
	form.Values.Set("company", c.Company)
	form.Values.Set("email", c.Email)
	form.Values.Set("phone", c.Phone)
	form.Values.Set("address", c.Address)
	form.Values.Set("notes", c.Notes)
	return form
}
