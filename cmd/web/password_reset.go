package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gfotev/pitlane/internal/domain"
	"github.com/gfotev/pitlane/internal/forms"
	"github.com/gfotev/pitlane/internal/mailer"
	"github.com/gfotev/pitlane/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func newAuthToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = base64.RawURLEncoding.EncodeToString(b)
	return plain, hashAuthToken(plain), nil
}

func hashAuthToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func (app *application) forgotPasswordView(w http.ResponseWriter, r *http.Request) {
	data := &templateData{
		CurrentPath:    r.URL.Path,
		Form:           forms.New(nil),
		ResetRequested: app.sessions.PopBool(r.Context(), "resetRequested"),
	}
	app.render(w, r, "forgot-password.page.html", data)
}

// Always return the same outcome to prevent account discovery.
func (app *application) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("email")
	form.MaxLength("email", 254)
	form.Email("email")
	if !form.Valid() {
		app.render(w, r, "forgot-password.page.html", &templateData{CurrentPath: r.URL.Path, Form: form})
		return
	}

	// Equalize work for known and unknown addresses.
	_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte("password-reset-timing"))
	email := strings.ToLower(clean(form.Get("email")))
	user, err := app.users.GetByEmail(r.Context(), email)
	if err == nil {
		app.sendPasswordReset(r, user)
	} else if !errors.Is(err, store.ErrNotFound) {
		app.logger.Error("password reset lookup failed", "err", err)
	}

	app.sessions.Put(r.Context(), "resetRequested", true)
	http.Redirect(w, r, "/account/forgot-password", http.StatusSeeOther)
}

func (app *application) sendPasswordReset(r *http.Request, user *domain.User) {
	token, tokenHash, err := newAuthToken()
	if err != nil {
		app.logger.Error("create password reset token", "err", err)
		return
	}
	if err := app.resets.RequestReset(r.Context(), user, tokenHash, time.Now().Add(domain.PasswordResetTokenTTL)); err != nil {
		if errors.Is(err, store.ErrResetThrottled) {
			app.logger.Warn("password reset throttled", "userID", user.ID)
			return
		}
		app.logger.Error("store password reset token", "err", err)
		return
	}

	resetURL := strings.TrimRight(app.baseURL, "/") + "/account/reset-password?token=" + url.QueryEscape(token)
	subject, html, text, err := mailer.PasswordResetEmail(mailer.PasswordResetEmailData{UserName: user.Name, ResetURL: resetURL})
	if err != nil {
		app.logger.Error("render password reset email", "err", err)
		return
	}
	if err := app.mailer.Send(r.Context(), mailer.Message{To: user.Email, Subject: subject, HTML: html, Text: text}); err != nil {
		app.logger.Error("send password reset email", "userID", user.ID, "err", err)
	}
}

func (app *application) resetPasswordView(w http.ResponseWriter, r *http.Request) {
	form := forms.New(url.Values{"token": {clean(r.URL.Query().Get("token"))}})
	app.render(w, r, "reset-password.page.html", &templateData{CurrentPath: r.URL.Path, Form: form, InvalidToken: form.Get("token") == ""})
}

func (app *application) resetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Невалидна заявка.", http.StatusBadRequest)
		return
	}
	form := forms.New(r.PostForm)
	form.Required("token", "password", "confirm")
	form.MinLength("password", 8)
	form.MaxLength("password", 72)
	if form.Get("password") != form.Get("confirm") {
		form.Errors["confirm"] = "Паролите не съвпадат."
	}
	if !form.Valid() {
		app.render(w, r, "reset-password.page.html", &templateData{CurrentPath: r.URL.Path, Form: form})
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(form.Get("password")), 12)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if _, err := app.resets.Consume(r.Context(), hashAuthToken(clean(form.Get("token"))), string(passwordHash)); err != nil {
		if errors.Is(err, store.ErrInvalidToken) {
			form.Errors["generic"] = "Линкът е невалиден или е изтекъл. Заявете нов линк."
			app.render(w, r, "reset-password.page.html", &templateData{CurrentPath: r.URL.Path, Form: form, InvalidToken: true})
			return
		}
		app.serverError(w, r, err)
		return
	}

	if err := app.sessions.Destroy(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}
	if err := app.sessions.RenewToken(r.Context()); err != nil {
		app.serverError(w, r, err)
		return
	}
	app.sessions.Put(r.Context(), "flash", "Паролата Ви е променена. Влезте с новата парола.")
	http.Redirect(w, r, "/account/login", http.StatusSeeOther)
}
