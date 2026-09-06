package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/store"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var dummyPasswordHash = []byte("$2a$12$O.nnYN8XkTvW7HZe3oCpOe/biwIy9eWE..QoDn0hl/XJNzFjgC.Y2")

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		app.notFound(w)
		return
	}
	if app.sessions.Exists(r.Context(), "userID") {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/account/login", http.StatusSeeOther)
}

func (app *application) health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}

func (app *application) signupView(w http.ResponseWriter, r *http.Request) {
	app.render(w, r, "signup.page.html", &templateData{CurrentPath: r.URL.Path, Form: forms.New(nil)})
}

func (app *application) signup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("garage_name", "name", "email", "password")
	form.MaxLength("garage_name", 100)
	form.MaxLength("name", 100)
	form.MaxLength("email", 254)
	form.Email("email")
	form.MinLength("password", 8)
	form.MaxLength("password", 72)
	if !form.Valid() {
		app.render(w, r, "signup.page.html", &templateData{CurrentPath: r.URL.Path, Form: form})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(form.Get("password")), 12)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	tenantID, userID := uuid.NewString(), uuid.NewString()
	tenant := &domain.Tenant{ID: tenantID, Name: clean(form.Get("garage_name")), Currency: "EUR", Locale: "bg", DefaultTaxRate: domain.StandardVATRateBPS, Settings: map[string]any{}}
	owner := &domain.User{ID: userID, TenantID: tenantID, Email: strings.ToLower(clean(form.Get("email"))), PasswordHash: string(hash), Role: domain.RoleOwner, Name: clean(form.Get("name"))}
	if err := app.tenants.CreateWithOwner(r.Context(), tenant, owner); err != nil {
		if errors.Is(err, store.ErrDuplicateEmail) {
			form.Errors["email"] = "Вече съществува профил с този имейл."
			app.render(w, r, "signup.page.html", &templateData{CurrentPath: r.URL.Path, Form: form})
			return
		}
		app.serverError(w, r, err)
		return
	}
	app.sendWelcomeEmail(r, owner, tenant.Name)
	if err := app.startSession(r, owner); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.flash(r, "Добре дошли в Pitlane. Вашият сервиз е готов.")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (app *application) loginView(w http.ResponseWriter, r *http.Request) {
	data := &templateData{CurrentPath: r.URL.Path, Form: forms.New(nil), Flash: app.sessions.PopString(r.Context(), "flash")}
	app.render(w, r, "login.page.html", data)
}

func (app *application) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("email", "password")
	form.Email("email")
	if !form.Valid() {
		app.render(w, r, "login.page.html", &templateData{CurrentPath: r.URL.Path, Form: form})
		return
	}
	user, err := app.users.GetByEmail(r.Context(), strings.ToLower(clean(form.Get("email"))))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(form.Get("password")))
			form.Errors["generic"] = "Имейлът или паролата са неправилни."
			app.render(w, r, "login.page.html", &templateData{CurrentPath: r.URL.Path, Form: form})
			return
		}
		app.serverError(w, r, err)
		return
	}
	passwordErr := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(form.Get("password")))
	if passwordErr != nil || user.Role != domain.RoleOwner {
		form.Errors["generic"] = "Имейлът или паролата са неправилни."
		app.render(w, r, "login.page.html", &templateData{CurrentPath: r.URL.Path, Form: form})
		return
	}
	if err := app.startSession(r, user); err != nil {
		app.serverError(w, r, err)
		return
	}
	redirect := app.sessions.PopString(r.Context(), "redirectPath")
	if redirect == "" || !safeRedirect(redirect) {
		redirect = "/dashboard"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (app *application) startSession(r *http.Request, user *domain.User) error {
	if err := app.sessions.RenewToken(r.Context()); err != nil {
		return err
	}
	app.sessions.Put(r.Context(), "userID", user.ID)
	app.sessions.Put(r.Context(), "tenantID", user.TenantID)
	app.sessions.Put(r.Context(), "userName", user.Name)
	app.sessions.Put(r.Context(), "authenticatedAt", time.Now().UnixNano())
	return nil
}

func (app *application) sendWelcomeEmail(r *http.Request, user *domain.User, garageName string) {
	loginURL := strings.TrimRight(app.baseURL, "/") + "/account/login"
	subject, html, text, err := mailer.WelcomeEmail(mailer.WelcomeEmailData{
		UserName: user.Name, GarageName: garageName, LoginURL: loginURL,
	})
	if err != nil {
		app.logger.Error("render welcome email", "userID", user.ID, "err", err)
		return
	}
	if err := app.mailer.Send(r.Context(), mailer.Message{To: user.Email, Subject: subject, HTML: html, Text: text}); err != nil {
		app.logger.Error("send welcome email", "userID", user.ID, "err", err)
	}
}

func (app *application) logout(w http.ResponseWriter, r *http.Request) {
	if err := app.sessions.Destroy(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := app.sessions.RenewToken(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.sessions.Put(r.Context(), "flash", "Излязохте от профила си.")
	http.Redirect(w, r, "/account/login", http.StatusSeeOther)
}

func safeRedirect(target string) bool {
	u, err := url.Parse(target)
	return err == nil && u.IsAbs() == false && strings.HasPrefix(u.Path, "/") && !strings.HasPrefix(u.Path, "//")
}
