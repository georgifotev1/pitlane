package main

import (
	"net/http"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
)

func (app *application) historyCreateView(w http.ResponseWriter, r *http.Request) {
	car, err := app.cars.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	data := app.newTemplateData(r)
	data.Car = car
	data.Form.Values.Set("recorded_at", time.Now().Format("2006-01-02"))
	app.render(w, r, "history-form.page.html", data)
}

func (app *application) historyCreate(w http.ResponseWriter, r *http.Request) {
	car, err := app.cars.Get(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	form, recordedAt, ok := app.readHistoryForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Car, data.Form = car, form
	if !form.Valid() {
		app.render(w, r, "history-form.page.html", data)
		return
	}
	note := &domain.HistoryNote{TenantID: app.tenantID(r), CarID: car.ID, Title: clean(form.Get("title")), Description: clean(form.Get("description")), RecordedAt: recordedAt}
	if err := app.history.CreateNote(r.Context(), note); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Записът е добавен.")
	http.Redirect(w, r, "/cars/"+car.ID, http.StatusSeeOther)
}

func (app *application) historyEditView(w http.ResponseWriter, r *http.Request) {
	note, err := app.history.GetNote(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	car, err := app.cars.Get(r.Context(), app.tenantID(r), note.CarID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	form := forms.New(nil)
	form.Values.Set("title", note.Title)
	form.Values.Set("description", note.Description)
	form.Values.Set("recorded_at", note.RecordedAt.Format("2006-01-02"))
	data := app.newTemplateData(r)
	data.Note, data.Car, data.Form = note, car, form
	app.render(w, r, "history-form.page.html", data)
}

func (app *application) historyEdit(w http.ResponseWriter, r *http.Request) {
	note, err := app.history.GetNote(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	car, err := app.cars.Get(r.Context(), app.tenantID(r), note.CarID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	form, recordedAt, ok := app.readHistoryForm(w, r)
	if !ok {
		return
	}
	data := app.newTemplateData(r)
	data.Note, data.Car, data.Form = note, car, form
	if !form.Valid() {
		app.render(w, r, "history-form.page.html", data)
		return
	}
	note.Title, note.Description, note.RecordedAt = clean(form.Get("title")), clean(form.Get("description")), recordedAt
	if err := app.history.UpdateNote(r.Context(), note); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Записът е обновен.")
	http.Redirect(w, r, "/cars/"+car.ID, http.StatusSeeOther)
}

func (app *application) historyDelete(w http.ResponseWriter, r *http.Request) {
	note, err := app.history.GetNote(r.Context(), app.tenantID(r), r.PathValue("id"))
	if handleStoreError(app, w, r, err) {
		return
	}
	if err := app.history.DeleteNote(r.Context(), app.tenantID(r), note.ID); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Записът е изтрит.")
	http.Redirect(w, r, "/cars/"+note.CarID, http.StatusSeeOther)
}

func (app *application) readHistoryForm(w http.ResponseWriter, r *http.Request) (*forms.Form, time.Time, bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return nil, time.Time{}, false
	}
	form := forms.New(r.PostForm)
	form.Required("title", "recorded_at")
	form.MaxLength("title", 150)
	recordedAt, err := time.Parse("2006-01-02", form.Get("recorded_at"))
	if err != nil {
		form.Errors["recorded_at"] = "Въведете валидна дата."
	}
	return form, recordedAt, true
}
