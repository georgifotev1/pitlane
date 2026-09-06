package main

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func (app *application) profileData(r *http.Request, nameForm, passwordForm *forms.Form) (*templateData, error) {
	user, err := app.users.GetByID(r.Context(), app.tenantID(r), app.userID(r))
	if err != nil {
		return nil, err
	}
	if nameForm == nil {
		nameForm = forms.New(url.Values{"name": {user.Name}})
	}
	if passwordForm == nil {
		passwordForm = forms.New(nil)
	}
	data := app.newTemplateData(r)
	data.User = user
	data.NameForm = nameForm
	data.PasswordForm = passwordForm
	return data, nil
}

func (app *application) profileView(w http.ResponseWriter, r *http.Request) {
	data, err := app.profileData(r, nil, nil)
	if handleStoreError(app, w, r, err) {
		return
	}
	app.render(w, r, "profile.page.html", data)
}

func (app *application) profileNameUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("name")
	form.MaxLength("name", 100)
	if !form.Valid() {
		data, err := app.profileData(r, form, nil)
		if handleStoreError(app, w, r, err) {
			return
		}
		app.render(w, r, "profile.page.html", data)
		return
	}

	name := clean(form.Get("name"))
	if err := app.users.UpdateName(r.Context(), app.tenantID(r), app.userID(r), name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFound(w)
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.sessions.Put(r.Context(), "userName", name)
	app.flash(r, "Името Ви е променено.")
	http.Redirect(w, r, "/account/profile", http.StatusSeeOther)
}

func (app *application) profilePasswordUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("current_password", "new_password", "confirm_password")
	form.MaxLength("current_password", 72)
	form.MinLength("new_password", 8)
	form.MaxLength("new_password", 72)
	form.MaxLength("confirm_password", 72)
	// bcrypt accepts at most 72 bytes, which may be fewer than 72 Cyrillic
	// characters even though the ordinary form limit is measured in runes.
	if len(form.Get("new_password")) > 72 {
		form.Errors["new_password"] = "Паролата не може да е по-дълга от 72 байта."
	}
	if form.Get("new_password") != "" && form.Get("confirm_password") != "" && form.Get("new_password") != form.Get("confirm_password") {
		form.Errors["confirm_password"] = "Паролите не съвпадат."
	}

	data, err := app.profileData(r, nil, form)
	if handleStoreError(app, w, r, err) {
		return
	}
	if form.Valid() && bcrypt.CompareHashAndPassword([]byte(data.User.PasswordHash), []byte(form.Get("current_password"))) != nil {
		form.Errors["current_password"] = "Текущата парола е неправилна."
	}
	if form.Valid() && form.Get("current_password") == form.Get("new_password") {
		form.Errors["new_password"] = "Новата парола трябва да е различна от текущата."
	}
	if !form.Valid() {
		app.render(w, r, "profile.page.html", data)
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(form.Get("new_password")), 12)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := app.users.UpdatePassword(r.Context(), data.User.TenantID, data.User.ID, string(passwordHash)); err != nil {
		app.serverError(w, r, err)
		return
	}
	// Keep this browser signed in with a fresh session. The password timestamp
	// invalidates any other sessions on their next authenticated request.
	if err := app.startSession(r, data.User); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Паролата Ви е променена.")
	http.Redirect(w, r, "/account/profile", http.StatusSeeOther)
}
